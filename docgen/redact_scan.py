#!/usr/bin/env python3
"""
Redact the passenger/guest name from a ticket or hotel-voucher scan BEFORE
it is uploaded to the Anthropic Files API.

Flow:
  redact_scan.py --in=/tmp/raw.jpg --out=/tmp/redacted.png

Output (stdout) on success:
  {"regions_redacted": N, "labels_found": ["passenger", ...]}

Output (stderr) on failure:
  {"error": "..."}
  non-zero exit code

Why this exists:
  ticket_parser.go and voucher_parser.go pass the full scan image to
  Claude Opus. Tickets and vouchers visibly contain the passenger's /
  guest's full name — PII under 152-ФЗ.  Redaction strips the name
  LOCALLY before anything leaves the server. No AI involved.

Policy:
  - If we cannot find ANY known label, we FAIL LOUDLY rather than ship
    an unredacted image. Silent failure would defeat the purpose.
  - The handler in Go turns that failure into a visible error for the
    manager so they know to redact manually or enter fields by hand.
"""
import argparse
import json
import sys
from pathlib import Path

import cv2
import numpy as np
import pytesseract
from PIL import Image
from pdf2image import convert_from_path

# Known label tokens. Matched case-insensitively. Each entry is one token —
# we also detect multi-word labels like "имя пассажира" by sequence search.
LABEL_TOKENS_SINGLE = {
    # English
    "passenger", "passengers", "name", "paxname", "guest", "guests",
    # Russian
    "пассажир", "пассажира", "гость", "гостя", "фио",
}

# Multi-word labels — matched as consecutive tokens. Case-insensitive.
LABEL_PHRASES = [
    ("passenger", "name"),
    ("guest", "name"),
    ("имя", "пассажира"),
    ("фамилия", "и", "имя"),
    ("имя", "и", "фамилия"),
]

# How many words after the label we blacken as "the name value".
NAME_TOKENS_AFTER_LABEL = 5


def _die(msg: str, code: int = 1) -> None:
    print(json.dumps({"error": msg}), file=sys.stderr)
    sys.exit(code)


def _load_pages_rgb(path: str) -> list[Image.Image]:
    """Return a list of PIL RGB images — one per page for PDF, single-element
    for JPG/PNG. Multi-page tickets/vouchers get fully processed; redacting
    only page 1 would still leak the name on later pages."""
    lower = path.lower()
    if lower.endswith(".pdf"):
        pages = convert_from_path(path, dpi=250)  # all pages
        if not pages:
            raise ValueError("empty PDF")
        return [p.convert("RGB") for p in pages]
    return [Image.open(path).convert("RGB")]


def _ocr_words(img: Image.Image) -> dict:
    """Return pytesseract.image_to_data in DICT form (positions + words)."""
    # rus+eng catches bilingual scans; PSM 6 = single uniform block of text.
    return pytesseract.image_to_data(
        img, lang="rus+eng", config="--psm 6",
        output_type=pytesseract.Output.DICT,
    )


def _normalize(tok: str) -> str:
    return tok.strip(".,:;/\\|()[]{}\"'<>").lower()


def _find_redaction_regions(data: dict) -> tuple[list[tuple[int, int, int, int]], list[str]]:
    """Scan OCR words, return the list of bounding boxes to blacken plus the
    labels we hit. Each box covers a single word; multiple adjacent word
    boxes merge upstream via cv2.rectangle painting.

    For each label match we also include the next NAME_TOKENS_AFTER_LABEL
    words on the same text line (same `line_num`).
    """
    words = data.get("text", [])
    n = len(words)
    if n == 0:
        return [], []
    lefts = data["left"]
    tops = data["top"]
    widths = data["width"]
    heights = data["height"]
    line_nums = data["line_num"]
    block_nums = data["block_num"]

    normalized = [_normalize(w) for w in words]
    regions: list[tuple[int, int, int, int]] = []
    labels_found: list[str] = []

    def _push(i: int) -> None:
        regions.append((lefts[i], tops[i], lefts[i] + widths[i], tops[i] + heights[i]))

    for i in range(n):
        tok = normalized[i]
        if not tok:
            continue

        # Multi-word phrases first — they may not also be singletons in LABEL_TOKENS_SINGLE.
        matched_phrase = False
        for phrase in LABEL_PHRASES:
            plen = len(phrase)
            if i + plen > n:
                continue
            window = tuple(normalized[i + k] for k in range(plen))
            if window == phrase:
                labels_found.append(" ".join(phrase))
                # Blacken the label words themselves.
                for k in range(plen):
                    _push(i + k)
                # And up to NAME_TOKENS_AFTER_LABEL words on the same line.
                line = line_nums[i]
                block = block_nums[i]
                taken = 0
                j = i + plen
                while j < n and taken < NAME_TOKENS_AFTER_LABEL:
                    if line_nums[j] == line and block_nums[j] == block:
                        if normalized[j]:
                            _push(j)
                            taken += 1
                    elif normalized[j]:
                        # Moved to next line — stop expanding to avoid over-redacting.
                        break
                    j += 1
                matched_phrase = True
                break
        if matched_phrase:
            continue

        if tok in LABEL_TOKENS_SINGLE:
            labels_found.append(tok)
            _push(i)
            line = line_nums[i]
            block = block_nums[i]
            taken = 0
            j = i + 1
            while j < n and taken < NAME_TOKENS_AFTER_LABEL:
                if line_nums[j] == line and block_nums[j] == block:
                    if normalized[j]:
                        _push(j)
                        taken += 1
                elif normalized[j]:
                    break
                j += 1

    return regions, labels_found


def _paint_black(img: Image.Image, regions: list[tuple[int, int, int, int]]) -> Image.Image:
    """Return a copy of `img` with every region drawn as a solid black rectangle."""
    arr = np.array(img)
    for (x1, y1, x2, y2) in regions:
        # 2-pixel padding each side — keeps ascenders/descenders fully covered.
        x1p = max(0, x1 - 2)
        y1p = max(0, y1 - 2)
        x2p = min(arr.shape[1], x2 + 2)
        y2p = min(arr.shape[0], y2 + 2)
        arr[y1p:y2p, x1p:x2p] = 0
    return Image.fromarray(arr)


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(add_help=False)
    p.add_argument("--in", dest="in_path", required=True)
    p.add_argument("--out", dest="out_path", required=True)
    try:
        args = p.parse_args(argv[1:])
    except SystemExit:
        _die("usage: redact_scan.py --in=<path> --out=<path>")

    if not Path(args.in_path).is_file():
        _die(f"input file not found: {args.in_path}", code=2)

    try:
        pages = _load_pages_rgb(args.in_path)
    except Exception as exc:
        _die(f"load image failed: {exc}", code=3)

    total_regions = 0
    all_labels: list[str] = []
    redacted_pages: list[Image.Image] = []
    pages_without_labels: list[int] = []

    for idx, page in enumerate(pages):
        try:
            data = _ocr_words(page)
        except Exception as exc:
            _die(f"ocr failed on page {idx + 1}: {exc}", code=4)
        regions, labels = _find_redaction_regions(data)
        total_regions += len(regions)
        all_labels.extend(labels)
        if regions:
            redacted_pages.append(_paint_black(page, regions))
        else:
            redacted_pages.append(page)
            pages_without_labels.append(idx + 1)

    # Policy: every page must have at least one label. If any page lacks a
    # recognizable name marker, we cannot guarantee the name isn't visible
    # on that page → fail loud. Single-page scans fall back to the same rule.
    if total_regions == 0:
        _die(
            "no name labels found — refusing to ship an un-redacted scan to AI. "
            "Manager should redact manually or enter fields by hand.",
            code=5,
        )
    if pages_without_labels:
        _die(
            "pages without any detectable name label: "
            + ", ".join(str(p) for p in pages_without_labels)
            + " — refusing to ship a partially-redacted scan.",
            code=5,
        )

    try:
        if args.out_path.lower().endswith(".pdf") and len(redacted_pages) > 1:
            first, rest = redacted_pages[0], redacted_pages[1:]
            first.save(args.out_path, "PDF", save_all=True, append_images=rest)
        elif len(redacted_pages) > 1:
            # Multi-page but PNG output requested — save as multi-page PDF
            # for fidelity. Caller must accept either output format.
            first, rest = redacted_pages[0], redacted_pages[1:]
            alt_path = args.out_path.rsplit(".", 1)[0] + ".pdf"
            first.save(alt_path, "PDF", save_all=True, append_images=rest)
            # Report the actual path so Go reads the right file.
            args.out_path = alt_path
        else:
            redacted_pages[0].save(args.out_path, "PNG")
    except Exception as exc:
        _die(f"save redacted image failed: {exc}", code=6)

    print(json.dumps({
        "regions_redacted": total_regions,
        "labels_found": sorted(set(all_labels)),
        "pages_processed": len(redacted_pages),
        "output_path": args.out_path,
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
