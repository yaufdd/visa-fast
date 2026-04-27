#!/usr/bin/env python3
"""
One-off tool to test Yandex Vision OCR on a real PDF/image.
Not part of the project — delete when done evaluating.

Usage:
  export YANDEX_IAM_TOKEN="t1.9eu..."        # OR set YANDEX_API_KEY
  export YANDEX_FOLDER_ID="b1gv..."
  python3 test_yandex_ocr.py path/to/file.pdf

Multi-page PDFs are split locally with pypdf and each page is sent
separately to the sync OCR endpoint (which is 1 page max). Install
pypdf once:  pip3 install pypdf
"""

import base64
import io
import json
import mimetypes
import os
import sys
import urllib.error
import urllib.request


OCR_ENDPOINT = "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"


def guess_mime(path: str) -> str:
    mt, _ = mimetypes.guess_type(path)
    if mt:
        return mt
    if path.lower().endswith(".pdf"):
        return "application/pdf"
    return "image/jpeg"


def split_pdf(path: str) -> list[bytes]:
    """Return a list of single-page PDF byte blobs, one per page."""
    try:
        from pypdf import PdfReader, PdfWriter
    except ImportError:
        sys.stderr.write("pypdf not installed. Run: pip3 install pypdf\n")
        sys.exit(2)

    reader = PdfReader(path)
    pages: list[bytes] = []
    for page in reader.pages:
        writer = PdfWriter()
        writer.add_page(page)
        buf = io.BytesIO()
        writer.write(buf)
        pages.append(buf.getvalue())
    return pages


def ocr_call(content_b64: str, mime: str, auth_header: str, folder_id: str) -> dict:
    payload = json.dumps({
        "mimeType": mime,
        "languageCodes": ["*"],
        "model": "page",
        "content": content_b64,
    }).encode("utf-8")

    req = urllib.request.Request(
        OCR_ENDPOINT,
        data=payload,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "Authorization": auth_header,
            "x-folder-id": folder_id,
        },
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.loads(resp.read())


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: python3 test_yandex_ocr.py path/to/file.pdf", file=sys.stderr)
        return 2

    path = sys.argv[1]
    if not os.path.isfile(path):
        print(f"File not found: {path}", file=sys.stderr)
        return 2

    iam_token = os.environ.get("YANDEX_IAM_TOKEN", "").strip()
    api_key = os.environ.get("YANDEX_API_KEY", "").strip()
    folder_id = os.environ.get("YANDEX_FOLDER_ID", "").strip()
    if not folder_id:
        print("Set YANDEX_FOLDER_ID env var first.", file=sys.stderr)
        return 2
    if not iam_token and not api_key:
        print("Set YANDEX_IAM_TOKEN (recommended) or YANDEX_API_KEY first.", file=sys.stderr)
        return 2

    if iam_token:
        auth_header = f"Bearer {iam_token}"
        auth_kind = "IAM token"
    else:
        auth_header = f"Api-Key {api_key}"
        auth_kind = "API key"

    mime = guess_mime(path)
    is_pdf = mime == "application/pdf"

    # Build pages: list of (mime, raw_bytes) for each page to OCR.
    if is_pdf:
        page_blobs = split_pdf(path)
        pages = [("application/pdf", blob) for blob in page_blobs]
    else:
        with open(path, "rb") as f:
            pages = [(mime, f.read())]

    print(f"→ Sending {path}")
    print(f"  auth: {auth_kind}")
    print(f"  pages to OCR: {len(pages)}")
    print()

    full_text_parts: list[str] = []
    raw_responses: list[dict] = []

    try:
        for i, (page_mime, page_bytes) in enumerate(pages, start=1):
            content_b64 = base64.b64encode(page_bytes).decode("ascii")
            kb = len(content_b64) // 1024
            print(f"  page {i}/{len(pages)} — {kb} KB base64 — calling OCR...")

            data = ocr_call(content_b64, page_mime, auth_header, folder_id)
            raw_responses.append(data)

            text_ann = (data.get("result") or {}).get("textAnnotation") or {}
            full_text = text_ann.get("fullText") or ""
            full_text_parts.append(f"--- page {i} ---\n{full_text}")

            blocks = text_ann.get("blocks") or []
            print(f"    {len(blocks)} blocks, {len(full_text)} chars")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        print(f"\nHTTP {e.code}: {body}", file=sys.stderr)
        return 1
    except urllib.error.URLError as e:
        print(f"\nNetwork error: {e.reason}", file=sys.stderr)
        return 1

    combined = "\n\n".join(full_text_parts)

    print()
    print("=" * 70)
    print("RECOGNIZED TEXT (all pages)")
    print("=" * 70)
    print(combined or "[empty — no text recognized]")

    out_path = os.path.splitext(path)[0] + ".ocr.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump({"pages": raw_responses}, f, ensure_ascii=False, indent=2)
    print(f"\nFull JSON response saved to: {out_path}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
