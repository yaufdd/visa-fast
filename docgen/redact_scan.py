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
    ("имя", "гостя"),
    ("фамилия", "и", "имя"),
    ("имя", "и", "фамилия"),
]

# Trailing labels — the word appears AFTER the guest name on the same line.
# Hotel vouchers commonly list passengers as "SURNAME FIRSTNAME   Adult",
# with NO label before the name. We trigger on these anchors and redact the
# words that precede them on the same line.
#
# To avoid false positives in running prose (e.g. "... the infants will be
# converted to children ..."), a trailing label only fires when the two
# immediately preceding non-empty tokens are BOTH all-uppercase alphabetic
# — which is how passenger names appear on these vouchers.
TRAILING_LABEL_TOKENS = {
    "adult", "adults",
    "child", "children",
    "infant", "infants",
    # Russian in case a localized voucher appears
    "взрослый", "взрослые", "ребёнок", "ребенок", "дети",
}

# How many words after the label we blacken as "the name value".
NAME_TOKENS_AFTER_LABEL = 5

# How many words before a trailing label we treat as the name.
NAME_TOKENS_BEFORE_LABEL = 4


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


def _strip_punct(tok: str) -> str:
    """Strip punctuation but preserve case — needed for the uppercase check
    used by trailing-label detection."""
    return tok.strip(".,:;/\\|()[]{}\"'<>")


def _looks_like_name_token(tok: str) -> bool:
    """A name-ish token on a voucher: all alphabetic (latin or cyrillic),
    length >= 2, all uppercase. Matches e.g. 'ISAEV', 'ИСАЕВ'."""
    if len(tok) < 2:
        return False
    if not tok.isalpha():
        return False
    return tok.isupper()


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
            continue

        # Trailing label — word appears AFTER the name on the same line.
        # Common on hotel vouchers ("ISAEV ANDREY   Adult"). Because OCR
        # often garbles one of the name tokens (e.g. misreads the surname
        # as a mix of Cyrillic+Latin+digits), we can't require multiple
        # clean all-caps tokens. Instead:
        #   * require at least ONE clean all-caps alphabetic name token
        #     (length >= 2) on the same line before the label,
        #   * refuse to fire if any mixed-case alphabetic word (length >= 3)
        #     appears on the line — that signals prose, not a name field.
        # If both hold, blacken every token on the line before the label so
        # that OCR-garbled parts of the name (like "|5АЕмА") are covered too.
        if tok in TRAILING_LABEL_TOKENS:
            line = line_nums[i]
            block = block_nums[i]
            prev: list[tuple[int, str]] = []  # [(token_index, original_text)]
            has_name_token = False
            has_prose_token = False
            j = i - 1
            while j >= 0 and len(prev) < NAME_TOKENS_BEFORE_LABEL:
                if line_nums[j] != line or block_nums[j] != block:
                    break
                if not normalized[j]:
                    j -= 1
                    continue
                raw = _strip_punct(words[j])
                prev.append((j, raw))
                if _looks_like_name_token(raw):
                    has_name_token = True
                elif raw.isalpha() and len(raw) >= 3 and not raw.isupper():
                    has_prose_token = True
                j -= 1

            if has_name_token and not has_prose_token:
                labels_found.append(tok)
                for idx, _ in prev:
                    _push(idx)

    return regions, labels_found


# Common ALL-CAPS words that are NOT person names — used by the per-page
# "is this page safe without any detected label" sanity check. Anything NOT in
# this set that appears as 2+ consecutive all-caps alphabetic tokens on one
# line is treated as a suspected name → fail loud.
SAFE_CAPS_WORDS = {
    # Document/section headers
    "HOTEL", "VOUCHER", "INVOICE", "RECEIPT", "BOOKING", "CONFIRMATION",
    "RESERVATION", "TICKET", "ORDER",
    # Common structural / passenger-category words
    "ADULT", "ADULTS", "CHILD", "CHILDREN", "INFANT", "INFANTS",
    "GUEST", "GUESTS", "PASSENGER", "PASSENGERS",
    "NAME", "ROOM", "MEAL", "RATE", "NIGHT", "NIGHTS",
    "CHECK", "IN", "OUT", "ID", "NO", "NR",
    # Country / airport codes that happen to be 3 letters all caps — we
    # intentionally do NOT list these; 3-letter codes without surrounding
    # name words won't trigger alone (need 2+ consecutive).
}


def _page_has_suspected_name(data: dict) -> bool:
    """Return True if this page looks like it contains a person name even
    though no redaction label was matched.

    Heuristic for a name on a voucher line:
      * 2+ consecutive all-caps alphabetic tokens of length >= 3
      * at least one of them is NOT in SAFE_CAPS_WORDS
      * and no mixed-case alphabetic word precedes them on the line
        (that would mean prose — e.g. "MORI Building DIGITAL ART MUSEUM").
    """
    words = data.get("text", [])
    if not words:
        return False
    line_nums = data["line_num"]
    block_nums = data["block_num"]
    n = len(words)

    def _prev_on_line_is_prose(start: int) -> bool:
        """True if any earlier non-empty token on the same line/block is an
        alphabetic word in non-uppercase (i.e. mixed/title/lowercase)."""
        line = line_nums[start]
        block = block_nums[start]
        k = start - 1
        while k >= 0:
            if line_nums[k] != line or block_nums[k] != block:
                return False
            raw = _strip_punct(words[k])
            if raw and raw.isalpha() and not raw.isupper():
                return True
            k -= 1
        return False

    i = 0
    while i < n:
        raw = _strip_punct(words[i])
        if _looks_like_name_token(raw) and len(raw) >= 3:
            line = line_nums[i]
            block = block_nums[i]
            seq: list[str] = [raw]
            j = i + 1
            while j < n:
                if line_nums[j] != line or block_nums[j] != block:
                    break
                next_raw = _strip_punct(words[j])
                if not next_raw:
                    j += 1
                    continue
                if _looks_like_name_token(next_raw) and len(next_raw) >= 3:
                    seq.append(next_raw)
                    j += 1
                else:
                    break
            if (
                len(seq) >= 2
                and any(tok not in SAFE_CAPS_WORDS for tok in seq)
                and not _prev_on_line_is_prose(i)
            ):
                return True
            i = j
        else:
            i += 1
    return False


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
    unsafe_pages: list[int] = []

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
            continue

        # No label on this page. That's OK for pages that legitimately have
        # no guest name (e.g. terms/remarks on a voucher). But if the page
        # contains something that LOOKS like a person name (2+ consecutive
        # all-caps tokens, not all from the safe headings list) we cannot
        # ship it to AI — fail loud.
        if _page_has_suspected_name(data):
            unsafe_pages.append(idx + 1)
        redacted_pages.append(page)

    # Policy: at least one page in the document must have matched a label,
    # and no page may contain an UN-redacted suspected name.
    if total_regions == 0:
        _die(
            "no name labels found — refusing to ship an un-redacted scan to AI. "
            "Manager should redact manually or enter fields by hand.",
            code=5,
        )
    if unsafe_pages:
        _die(
            "pages with a possible unredacted name: "
            + ", ".join(str(p) for p in unsafe_pages)
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
