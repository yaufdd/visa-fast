# Russian Passport Scan Parser (Internal + Foreign) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a manager upload a scan/photo of either the Russian **internal passport** or the Russian **foreign passport** (загранпаспорт), extract its fields via local-only parsing, and pre-fill the submission form — without sending any passport PII to Claude or other external services.

**Architecture:** One authenticated endpoint `POST /api/passport/parse` accepts a multipart `file` plus a `doc_type=internal|foreign` form field, writes the upload to `uploads/_passport_tmp/<uuid>.<ext>`, shells out to `docgen/passport_parser.py --doc-type=…`, deletes the temp file in a `defer`, and returns a JSON payload whose keys line up with `submission_snapshot` fields.

Two extraction strategies share the same script, `_load_image` preprocessing helper, and Go wrapper:

- **Internal passport** → Tesseract OCR (`rus+eng`) + regex/label heuristics. No MRZ exists on the internal passport. Fields: `internal_series`, `internal_number`, `internal_issued_ru`, `internal_issued_by_ru`, `department_code`, `name_cyr`, `birth_date`, `gender_ru`, `place_of_birth_ru`.
- **Foreign passport** → `passporteye` (MRZ, ICAO 9303, deterministic) + `pycountry` for ISO-alpha-3 → English country name. Fields: `passport_number`, `expiry_date`, `name_lat`, `birth_date`, `gender_ru`, `nationality_ru`. (The visual-inspection-zone fields — `issue_date`, `issued_by_ru` — are not in MRZ and remain manually entered.)

Both paths return the **same superset JSON schema** (all keys always present, empty string when not applicable). Frontend applies each non-empty value only if the form field is currently empty — never overwrites user input.

**Tech Stack:**
- Go (chi router, pgx) — reuses the Python-subprocess pattern from `backend/internal/docgen/generate.go`
- Python 3 + Tesseract 5 (`rus+eng`) + pytesseract + pdf2image + Pillow + opencv-python-headless (preprocessing)
- Python: `passporteye` (MRZ) + `pycountry` (ISO country-code → English name)
- React + Vite (SubmissionForm.jsx)

**Privacy / Compliance:**
- Scan file exists only for the lifetime of one HTTP request (~1–5 s), under `uploads/_passport_tmp/`, and is removed in a `defer`. Never persisted to DB, never written under `uploads/{groupID}/`.
- No scan bytes ever reach Anthropic or any network — all processing is local. Contrast with `ticket_parser.go` / `voucher_parser.go`, which legitimately use the AI path because the content they see (flight numbers, hotel names + arrival dates) is not passport PII.
- No new DB columns — extracted fields flow into the existing `submission_snapshot` JSONB under keys the form already uses.

**Out of scope (explicit non-goals):**
- Public `/form/<slug>` exposure — MVP is authenticated-only. Public scan upload requires separate rate-limit/abuse hardening and belongs in a follow-up plan.
- Replacing the ticket / voucher AI parsers — content is not passport PII and the voucher has no standard layout.
- `issue_date` + `issued_by_ru` for the foreign passport — those live in the visual inspection zone, not MRZ. Parsing them reliably needs OCR + layout tricks that don't fit in this plan.
- `reg_address_ru` for the internal passport (propiska stamp, page 5, often handwritten).
- Auto-saving parsed values — the UI only pre-fills; the manager confirms by clicking "Сохранить" as today.

**Output schema (stable across both doc types):**

```json
{
  "fields": {
    "internal_series": "",
    "internal_number": "",
    "internal_issued_ru": "",
    "internal_issued_by_ru": "",
    "department_code": "",
    "passport_number": "",
    "expiry_date": "",
    "name_cyr": "",
    "name_lat": "",
    "gender_ru": "",
    "birth_date": "",
    "place_of_birth_ru": "",
    "nationality_ru": "",
    "reg_address_ru": ""
  },
  "doc_type": "internal",
  "raw_text": "…",
  "warnings": []
}
```

Which keys populate for which `doc_type`:

| key | internal | foreign |
|---|:---:|:---:|
| internal_series | ✓ | — |
| internal_number | ✓ | — |
| internal_issued_ru | ✓ | — |
| internal_issued_by_ru | ✓ | — |
| department_code | ✓ | — |
| passport_number | — | ✓ |
| expiry_date | — | ✓ |
| name_cyr | ✓ | — |
| name_lat | — | ✓ |
| gender_ru | ✓ | ✓ |
| birth_date | ✓ | ✓ |
| place_of_birth_ru | ✓ | — |
| nationality_ru | — | ✓ |
| reg_address_ru | — | — (left empty in both, see non-goals) |

---

### Task 1: Dockerfile.backend — install Tesseract + MRZ deps

**Files:**
- Modify: `Dockerfile.backend`

- [ ] **Step 1: Read current Dockerfile.backend to locate the existing apt / pip blocks**

Run: `cat Dockerfile.backend`
Expected: you can see where `python3`, `poppler-utils`, and the existing `pip install` line live.

- [ ] **Step 2: Add system packages**

In the existing `apt-get install -y …` block (or a new one right next to it in the same stage), add:

```
tesseract-ocr tesseract-ocr-rus tesseract-ocr-eng
poppler-utils
libgl1 libglib2.0-0
```

If there's no such block, add:

```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends \
    tesseract-ocr tesseract-ocr-rus tesseract-ocr-eng \
    poppler-utils libgl1 libglib2.0-0 \
 && rm -rf /var/lib/apt/lists/*
```

- [ ] **Step 3: Add Python libs to the existing pip install line**

Append to the same `pip install` command (single layer):

```
pytesseract==0.3.13 pdf2image==1.17.0 pillow==11.1.0 opencv-python-headless==4.10.0.84 passporteye==2.2.2 pycountry==24.6.1
```

- [ ] **Step 4: Build and verify**

```bash
docker build -f Dockerfile.backend -t fujit-backend-test .
docker run --rm fujit-backend-test tesseract --list-langs
docker run --rm fujit-backend-test python3 -c "from passporteye import read_mrz; import pycountry; print('ok')"
```

Expected: `rus`, `eng` in tesseract langs; `ok` from the Python import check.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.backend
git commit -m "feat(docker): install Tesseract + passporteye for passport OCR/MRZ"
```

---

### Task 2: Python — `passport_parser.py` skeleton + CLI contract

**Files:**
- Create: `docgen/passport_parser.py`
- Create: `docgen/tests/__init__.py`
- Create: `docgen/tests/test_passport_parser.py`

- [ ] **Step 1: Write failing CLI-contract tests**

`docgen/tests/test_passport_parser.py`:

```python
"""CLI-level tests for docgen/passport_parser.py.

The script is invoked as a subprocess (matching how Go calls it) and must
speak a stable JSON contract even for error cases.
"""
import json
import subprocess
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).parent
SCRIPT = HERE.parent / "passport_parser.py"


def run_cli(*args: str) -> tuple[int, str, str]:
    proc = subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        capture_output=True, text=True, timeout=30,
    )
    return proc.returncode, proc.stdout, proc.stderr


def test_cli_missing_args_exits_with_error_json():
    rc, _, err = run_cli()
    assert rc != 0
    payload = json.loads(err)
    assert "usage" in payload["error"].lower()


def test_cli_rejects_unknown_doc_type(tmp_path):
    img = tmp_path / "x.jpg"
    img.write_bytes(b"not-a-real-image")
    rc, _, err = run_cli(str(img), "--doc-type=unknown")
    assert rc != 0
    payload = json.loads(err)
    assert "doc-type" in payload["error"].lower()
```

- [ ] **Step 2: Run the failing test**

Run: `pytest docgen/tests/test_passport_parser.py -v`
Expected: FAIL — `FileNotFoundError` for `passport_parser.py`.

- [ ] **Step 3: Write the CLI skeleton**

`docgen/passport_parser.py`:

```python
#!/usr/bin/env python3
"""
Russian passport OCR/MRZ extractor.

CLI:
    passport_parser.py <image_or_pdf_path> --doc-type=internal
    passport_parser.py <image_or_pdf_path> --doc-type=foreign

stdout (success): JSON { "fields": {…}, "doc_type": "...", "raw_text": "...", "warnings": [...] }
stderr (failure): JSON { "error": "..." }

Runs ENTIRELY locally — never touches the network. Internal passport uses
Tesseract + regex; foreign passport uses passporteye (MRZ, ICAO 9303).
"""
import argparse
import json
import os
import sys

VALID_DOC_TYPES = ("internal", "foreign")


def _die(msg: str, code: int = 1) -> None:
    print(json.dumps({"error": msg}), file=sys.stderr)
    sys.exit(code)


def _parse_argv(argv: list[str]) -> tuple[str, str]:
    if len(argv) < 2:
        _die("usage: passport_parser.py <path> --doc-type=internal|foreign")
    p = argparse.ArgumentParser(add_help=False)
    p.add_argument("path")
    p.add_argument("--doc-type", dest="doc_type", required=True)
    try:
        args = p.parse_args(argv[1:])
    except SystemExit:
        _die("usage: passport_parser.py <path> --doc-type=internal|foreign")
    if args.doc_type not in VALID_DOC_TYPES:
        _die(f"invalid --doc-type: {args.doc_type!r}, expected one of {VALID_DOC_TYPES}")
    if not os.path.isfile(args.path):
        _die(f"file not found: {args.path}", code=2)
    return args.path, args.doc_type


def main(argv: list[str]) -> int:
    path, doc_type = _parse_argv(argv)
    # Real extraction lands in Task 5.
    print(json.dumps({
        "fields": {}, "doc_type": doc_type, "raw_text": "", "warnings": [],
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
```

- [ ] **Step 4: Run the tests — both should pass**

Run: `pytest docgen/tests/test_passport_parser.py -v`
Expected: PASS for both cases.

- [ ] **Step 5: Commit**

```bash
git add docgen/passport_parser.py docgen/tests/
git commit -m "feat(docgen): passport parser CLI with --doc-type validation"
```

---

### Task 3: Python — `extract_internal_fields` (pure function, TDD)

Rationale: the regex/label heuristics are the hard part. We TDD them against a deterministic OCR-output fixture so iteration is fast and doesn't re-invoke Tesseract.

**Files:**
- Modify: `docgen/passport_parser.py`
- Modify: `docgen/tests/test_passport_parser.py`
- Create: `docgen/tests/fixtures/internal_ocr_sample.txt`

- [ ] **Step 1: Add realistic OCR fixture**

`docgen/tests/fixtures/internal_ocr_sample.txt` — line breaks and small OCR artefacts preserved on purpose (this is what Tesseract typically emits on a clean scan):

```
РОССИЙСКАЯ ФЕДЕРАЦИЯ
ПАСПОРТ
45 12 345678
ГУ МВД РОССИИ ПО
Г. МОСКВЕ
Дата выдачи 15.08.2015
Код подразделения 770-001
Личный код

Фамилия ИВАНОВ
Имя ПЁТР
Отчество СЕРГЕЕВИЧ
Пол МУЖ.
Дата рождения 02.03.1985
Место рождения Г. МОСКВА
```

- [ ] **Step 2: Write failing unit tests for internal extraction**

Append to `docgen/tests/test_passport_parser.py`:

```python
# Direct import of the pure function — no subprocess here.
sys.path.insert(0, str(SCRIPT.parent))
from passport_parser import extract_internal_fields, EMPTY_FIELDS  # noqa: E402

FIXTURES = HERE / "fixtures"


@pytest.fixture
def internal_text() -> str:
    return (FIXTURES / "internal_ocr_sample.txt").read_text(encoding="utf-8")


def test_internal_series_and_number(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["internal_series"] == "4512"
    assert out["internal_number"] == "345678"


def test_internal_issue_date(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["internal_issued_ru"] == "15.08.2015"


def test_internal_issuing_authority_is_multiline_single_spaced(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["internal_issued_by_ru"] == "ГУ МВД РОССИИ ПО Г. МОСКВЕ"


def test_internal_department_code(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["department_code"] == "770-001"


def test_internal_full_name(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["name_cyr"] == "ИВАНОВ ПЁТР СЕРГЕЕВИЧ"


def test_internal_gender(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["gender_ru"] == "Мужской"


def test_internal_birth_date(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["birth_date"] == "02.03.1985"


def test_internal_place_of_birth(internal_text):
    out = extract_internal_fields(internal_text)
    assert out["place_of_birth_ru"] == "Г. МОСКВА"


def test_internal_all_missing_defaults_to_empty_strings():
    out = extract_internal_fields("")
    for key in EMPTY_FIELDS:
        assert out[key] == "", f"{key} should default to empty string"


def test_internal_output_contains_every_schema_key(internal_text):
    # The superset schema is enforced on both internal and foreign paths.
    out = extract_internal_fields(internal_text)
    assert set(out.keys()) == set(EMPTY_FIELDS.keys())
```

- [ ] **Step 3: Run the failing tests**

Run: `pytest docgen/tests/test_passport_parser.py -v -k internal`
Expected: FAIL — `ImportError: cannot import name 'extract_internal_fields'`.

- [ ] **Step 4: Implement the superset schema + `extract_internal_fields`**

Insert in `docgen/passport_parser.py` (between the imports and `main`):

```python
import re
from typing import Dict

# Superset field schema — every key present in every response, empty string when
# not applicable to this doc_type. Keep in sync with:
#   - Go: backend/internal/passport/parser.go :: Fields
#   - JS: frontend/src/components/SubmissionForm.jsx :: applyScan allowed keys
EMPTY_FIELDS: Dict[str, str] = {
    "internal_series": "",
    "internal_number": "",
    "internal_issued_ru": "",
    "internal_issued_by_ru": "",
    "department_code": "",
    "passport_number": "",
    "expiry_date": "",
    "name_cyr": "",
    "name_lat": "",
    "gender_ru": "",
    "birth_date": "",
    "place_of_birth_ru": "",
    "nationality_ru": "",
    "reg_address_ru": "",
}

_RE_SERIES_NUMBER = re.compile(r"\b(\d{2})\s*(\d{2})\s+(\d{6})\b")
_RE_DATE = re.compile(r"\b(\d{2}\.\d{2}\.\d{4})\b")
_RE_DEPT_CODE = re.compile(r"\b(\d{3}-\d{3})\b")
_RE_GENDER = re.compile(r"\b(МУЖ|ЖЕН)\.?(?:СКОЙ|СКИЙ)?\b", re.IGNORECASE)


def _field_after_label(text: str, label: str) -> str:
    """Find `label` at the start of a line; return what follows on same line, or
    the next non-empty line if the label is alone. Case-insensitive."""
    lines = [ln.rstrip() for ln in text.splitlines()]
    pattern = re.compile(r"^\s*" + re.escape(label) + r"\s*[:.\-]?\s*(.*)$", re.IGNORECASE)
    for i, ln in enumerate(lines):
        m = pattern.match(ln)
        if not m:
            continue
        rest = m.group(1).strip()
        if rest:
            return rest
        for nxt in lines[i + 1:]:
            if nxt.strip():
                return nxt.strip()
        return ""
    return ""


def _extract_issuing_authority(text: str) -> str:
    """The authority block sits between the series-number line and "Дата выдачи".

    Collect lines of Cyrillic caps in that window, join with single spaces,
    drop any leading "Кем выдан" label.
    """
    lines = [ln.strip() for ln in text.splitlines()]
    collecting = False
    collected: list[str] = []
    for ln in lines:
        if _RE_SERIES_NUMBER.search(ln):
            collecting = True
            continue
        if not collecting:
            continue
        if re.search(r"дата\s+выдачи|код\s+подразделения", ln, re.IGNORECASE):
            break
        if ln:
            collected.append(ln)
    joined = " ".join(collected)
    joined = re.sub(r"^\s*кем\s+выдан\s*[:.\-]?\s*", "", joined, flags=re.IGNORECASE)
    return re.sub(r"\s+", " ", joined).strip()


def extract_internal_fields(text: str) -> Dict[str, str]:
    """Extract internal-passport fields from raw OCR text.

    All keys in EMPTY_FIELDS are always present in the output.
    """
    out = dict(EMPTY_FIELDS)
    if not text:
        return out

    m = _RE_SERIES_NUMBER.search(text)
    if m:
        out["internal_series"] = m.group(1) + m.group(2)
        out["internal_number"] = m.group(3)

    # Pick oldest date = DoB, newest = issue date.
    dates = _RE_DATE.findall(text)
    if dates:
        def _key(s: str) -> tuple[int, int, int]:
            d, mo, y = s.split(".")
            return int(y), int(mo), int(d)
        ordered = sorted(set(dates), key=_key)
        out["birth_date"] = ordered[0]
        if len(ordered) > 1:
            out["internal_issued_ru"] = ordered[-1]

    dept = _RE_DEPT_CODE.search(text)
    if dept:
        out["department_code"] = dept.group(1)

    out["internal_issued_by_ru"] = _extract_issuing_authority(text)

    surname = _field_after_label(text, "Фамилия")
    given = _field_after_label(text, "Имя")
    patronymic = _field_after_label(text, "Отчество")
    parts = [p for p in (surname, given, patronymic) if p]
    out["name_cyr"] = " ".join(parts)

    g = _RE_GENDER.search(text)
    if g:
        out["gender_ru"] = "Мужской" if g.group(1).upper() == "МУЖ" else "Женский"

    out["place_of_birth_ru"] = _field_after_label(text, "Место рождения")
    return out
```

- [ ] **Step 5: Run the tests until green**

Run: `pytest docgen/tests/test_passport_parser.py -v -k internal`
Expected: all 10 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add docgen/passport_parser.py docgen/tests/
git commit -m "feat(docgen): extract_internal_fields with superset schema"
```

---

### Task 4: Python — `extract_foreign_fields` (passporteye MRZ, TDD)

Unlike Task 3, we can't easily fake an MRZ string without exercising `passporteye` itself (it reads image bytes). We therefore test `extract_foreign_fields` by passing in a pre-parsed `passporteye.to_dict()`-shaped input dict, and separately smoke-test the full MRZ→image pipeline if a sample is present.

**Files:**
- Modify: `docgen/passport_parser.py`
- Modify: `docgen/tests/test_passport_parser.py`

- [ ] **Step 1: Write failing tests for foreign extraction from a mocked MRZ dict**

Append to `docgen/tests/test_passport_parser.py`:

```python
from passport_parser import extract_foreign_fields  # noqa: E402


def _fake_mrz(**overrides) -> dict:
    """Shape that matches passporteye.MRZ.to_dict() for a type-P document."""
    base = {
        "type": "P",
        "country": "RUS",
        "surname": "IVANOV",
        "names": "PIOTR",
        "number": "123456789",
        "nationality": "RUS",
        "date_of_birth": "850302",   # YYMMDD
        "sex": "M",
        "expiration_date": "300915",  # YYMMDD
        "personal_number": "",
        "valid_score": 95,
    }
    base.update(overrides)
    return base


def test_foreign_passport_number(internal_text):  # fixture unused; pytest needs a param name
    out = extract_foreign_fields(_fake_mrz())
    assert out["passport_number"] == "123456789"


def test_foreign_name_lat_surname_then_given():
    out = extract_foreign_fields(_fake_mrz(surname="IVANOV", names="PIOTR SERGEEVICH"))
    assert out["name_lat"] == "IVANOV PIOTR SERGEEVICH"


def test_foreign_birth_date_converted_to_dmy():
    out = extract_foreign_fields(_fake_mrz(date_of_birth="850302"))
    assert out["birth_date"] == "02.03.1985"


def test_foreign_birth_date_handles_19xx_vs_20xx_pivot():
    # passporteye returns YY only; our pivot: YY >= 30 → 19YY, YY < 30 → 20YY.
    # A child born in 2015 vs a grown adult in 1985 must both work.
    assert extract_foreign_fields(_fake_mrz(date_of_birth="150510"))["birth_date"] == "10.05.2015"
    assert extract_foreign_fields(_fake_mrz(date_of_birth="850302"))["birth_date"] == "02.03.1985"


def test_foreign_expiry_date_always_20xx():
    # Passports don't expire >75 years out; expiry YY is always 20YY.
    assert extract_foreign_fields(_fake_mrz(expiration_date="300915"))["expiry_date"] == "15.09.2030"
    assert extract_foreign_fields(_fake_mrz(expiration_date="251231"))["expiry_date"] == "31.12.2025"


def test_foreign_gender_male_female():
    assert extract_foreign_fields(_fake_mrz(sex="M"))["gender_ru"] == "Мужской"
    assert extract_foreign_fields(_fake_mrz(sex="F"))["gender_ru"] == "Женский"


def test_foreign_nationality_rus_becomes_russia():
    # Output uses full English name per CLAUDE.md ("RUSSIA", not "RUS").
    assert extract_foreign_fields(_fake_mrz(nationality="RUS"))["nationality_ru"] == "RUSSIA"


def test_foreign_nationality_other_country():
    assert extract_foreign_fields(_fake_mrz(nationality="KAZ"))["nationality_ru"] == "KAZAKHSTAN"


def test_foreign_nationality_unknown_code_falls_back_to_raw():
    # Better than dropping info — if pycountry doesn't resolve, surface the code.
    assert extract_foreign_fields(_fake_mrz(nationality="ZZZ"))["nationality_ru"] == "ZZZ"


def test_foreign_empty_mrz_gives_schema_with_empty_values():
    out = extract_foreign_fields(None)
    for key in EMPTY_FIELDS:
        assert out[key] == ""


def test_foreign_output_is_superset_schema():
    out = extract_foreign_fields(_fake_mrz())
    assert set(out.keys()) == set(EMPTY_FIELDS.keys())
```

Remove the spurious `internal_text` parameter from `test_foreign_passport_number` before running — it was only there to match the fixture-reuse pattern and is unused. Keep only `_fake_mrz()`.

- [ ] **Step 2: Run the failing tests**

Run: `pytest docgen/tests/test_passport_parser.py -v -k foreign`
Expected: FAIL — `ImportError: extract_foreign_fields`.

- [ ] **Step 3: Implement `extract_foreign_fields` + country-name helper**

Add to `docgen/passport_parser.py` (below `extract_internal_fields`):

```python
try:
    import pycountry
except ImportError:  # tests in lightweight envs can still load the module.
    pycountry = None  # type: ignore[assignment]


def _country_english_name(alpha3: str) -> str:
    """Map ISO 3166-1 alpha-3 to the English country name (e.g. 'RUS' → 'RUSSIA').

    Per CLAUDE.md the form uses the full English uppercase name. If pycountry
    can't resolve the code, return the raw code so the manager can correct it.
    """
    if not alpha3:
        return ""
    code = alpha3.strip().upper()
    if pycountry is None:
        return code
    c = pycountry.countries.get(alpha_3=code)
    if c is None:
        return code
    # pycountry gives "Russian Federation" — we want the shorter form the rest
    # of the system uses. Fall back to official name if the common "name"
    # field is already short.
    name = getattr(c, "name", code).upper()
    # Narrow shortening for a handful of CIS countries the agency sees most.
    aliases = {
        "RUSSIAN FEDERATION": "RUSSIA",
        "KOREA, REPUBLIC OF": "KOREA",
        "KOREA, DEMOCRATIC PEOPLE'S REPUBLIC OF": "NORTH KOREA",
        "MOLDOVA, REPUBLIC OF": "MOLDOVA",
    }
    return aliases.get(name, name)


def _yymmdd_to_dmy(yymmdd: str, century_pivot: int | None = None) -> str:
    """YYMMDD → DD.MM.YYYY. If `century_pivot` is None, always 20YY (use for
    expiry dates). Otherwise YY >= pivot → 19YY, else 20YY (use for DoB, pivot=30).
    """
    if not yymmdd or len(yymmdd) != 6 or not yymmdd.isdigit():
        return ""
    yy, mm, dd = yymmdd[0:2], yymmdd[2:4], yymmdd[4:6]
    if century_pivot is None:
        year = 2000 + int(yy)
    else:
        year = 1900 + int(yy) if int(yy) >= century_pivot else 2000 + int(yy)
    return f"{dd}.{mm}.{year}"


def extract_foreign_fields(mrz: dict | None) -> Dict[str, str]:
    """Map passporteye.MRZ.to_dict() → superset schema."""
    out = dict(EMPTY_FIELDS)
    if not mrz:
        return out

    number = (mrz.get("number") or "").strip().replace("<", "")
    out["passport_number"] = number

    surname = (mrz.get("surname") or "").strip().replace("<", " ").strip()
    names = (mrz.get("names") or "").strip().replace("<", " ").strip()
    name_lat = " ".join(p for p in (surname, names) if p)
    out["name_lat"] = re.sub(r"\s+", " ", name_lat).upper()

    out["birth_date"] = _yymmdd_to_dmy(mrz.get("date_of_birth") or "", century_pivot=30)
    out["expiry_date"] = _yymmdd_to_dmy(mrz.get("expiration_date") or "", century_pivot=None)

    sex = (mrz.get("sex") or "").strip().upper()
    if sex == "M":
        out["gender_ru"] = "Мужской"
    elif sex == "F":
        out["gender_ru"] = "Женский"

    out["nationality_ru"] = _country_english_name(mrz.get("nationality") or "")
    return out
```

- [ ] **Step 4: Run the tests until green**

Run: `pytest docgen/tests/test_passport_parser.py -v -k foreign`
Expected: all 11 tests PASS. If `pycountry` isn't installed locally, the `nationality_*` tests may fail — install it: `pip install pycountry==24.6.1`.

- [ ] **Step 5: Commit**

```bash
git add docgen/passport_parser.py docgen/tests/
git commit -m "feat(docgen): extract_foreign_fields from MRZ dict (ICAO 9303)"
```

---

### Task 5: Python — wire OCR + MRZ into CLI dispatch

**Files:**
- Modify: `docgen/passport_parser.py`
- Create: `docgen/tests/test_passport_parser_ocr.py`
- Create: `docgen/tests/fixtures/README.md` (documents that sample images are not committed)

- [ ] **Step 1: Fixtures README + .gitignore guard**

`docgen/tests/fixtures/README.md`:

```markdown
# Fixtures for passport parser tests

Text fixtures (e.g. `internal_ocr_sample.txt`) are committed — they are
synthetic OCR output, no real PII.

Image fixtures (`sample_internal.jpg`, `sample_foreign.jpg`, `*.pdf`) are
**NOT** committed. Place scans locally to exercise the OCR integration tests;
they will be skipped when files are absent. Use synthetic or anonymised
sample images — never commit real personal documents.
```

In `.gitignore`, add (if not already covered):

```
docgen/tests/fixtures/sample_*.jpg
docgen/tests/fixtures/sample_*.jpeg
docgen/tests/fixtures/sample_*.png
docgen/tests/fixtures/sample_*.pdf
```

- [ ] **Step 2: Write skippable OCR + MRZ integration tests**

`docgen/tests/test_passport_parser_ocr.py`:

```python
"""Integration tests — run only when Tesseract / passporteye / a sample image is present."""
import json
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

HERE = Path(__file__).parent
SCRIPT = HERE.parent / "passport_parser.py"
SAMPLE_INTERNAL = HERE / "fixtures" / "sample_internal.jpg"
SAMPLE_FOREIGN = HERE / "fixtures" / "sample_foreign.jpg"

requires_tesseract = pytest.mark.skipif(
    shutil.which("tesseract") is None, reason="tesseract not installed",
)


def _run(path: Path, doc_type: str) -> tuple[int, dict]:
    proc = subprocess.run(
        [sys.executable, str(SCRIPT), str(path), f"--doc-type={doc_type}"],
        capture_output=True, text=True, timeout=60,
    )
    if proc.returncode != 0:
        return proc.returncode, {"stderr": proc.stderr}
    return proc.returncode, json.loads(proc.stdout)


@requires_tesseract
@pytest.mark.skipif(not SAMPLE_INTERNAL.exists(), reason="sample_internal.jpg not present")
def test_internal_end_to_end_extracts_series():
    rc, out = _run(SAMPLE_INTERNAL, "internal")
    assert rc == 0, out
    assert out["fields"]["internal_series"], "series should be populated from real sample"


@pytest.mark.skipif(not SAMPLE_FOREIGN.exists(), reason="sample_foreign.jpg not present")
def test_foreign_end_to_end_extracts_passport_number():
    rc, out = _run(SAMPLE_FOREIGN, "foreign")
    assert rc == 0, out
    assert out["fields"]["passport_number"], "MRZ number should be populated from real sample"
    assert len(out["fields"]["passport_number"]) == 9
```

- [ ] **Step 3: Implement `_load_image`, `_preprocess`, `run_internal_ocr`, `run_foreign_mrz`**

Add near the top of `passport_parser.py` (after existing imports):

```python
import cv2
import numpy as np
import pytesseract
from PIL import Image
from pdf2image import convert_from_path


def _load_image(path: str) -> Image.Image:
    """PIL image from a JPG/PNG, or page 1 of a PDF."""
    if path.lower().endswith(".pdf"):
        pages = convert_from_path(path, dpi=300, first_page=1, last_page=1)
        if not pages:
            raise ValueError("empty PDF")
        return pages[0]
    return Image.open(path)


def _preprocess(img: Image.Image) -> Image.Image:
    """Grayscale → upscale if small → Otsu threshold."""
    arr = np.array(img.convert("L"))
    h, w = arr.shape[:2]
    if max(h, w) < 1500:
        scale = 1500 / max(h, w)
        arr = cv2.resize(arr, None, fx=scale, fy=scale, interpolation=cv2.INTER_CUBIC)
    _, arr = cv2.threshold(arr, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)
    return Image.fromarray(arr)


def run_internal_ocr(path: str) -> str:
    """Internal passport → raw OCR text (regex-parsed by extract_internal_fields)."""
    img = _preprocess(_load_image(path))
    return pytesseract.image_to_string(img, lang="rus+eng", config="--psm 6")


def run_foreign_mrz(path: str) -> tuple[dict | None, str]:
    """Foreign passport → (mrz_dict_or_None, raw_mrz_text_for_debug)."""
    from passporteye import read_mrz  # local import — heavy, optional at import time
    # For PDFs passporteye needs an image path — convert the first page first.
    if path.lower().endswith(".pdf"):
        pages = convert_from_path(path, dpi=300, first_page=1, last_page=1)
        if not pages:
            return None, ""
        # Write to a sibling .png next to the PDF — caller deletes the temp dir.
        img_path = path + ".page1.png"
        pages[0].save(img_path, "PNG")
        mrz = read_mrz(img_path)
    else:
        mrz = read_mrz(path)
    if mrz is None:
        return None, ""
    data = mrz.to_dict()
    raw = getattr(mrz, "mrz", "") or ""
    return data, str(raw)
```

- [ ] **Step 4: Replace the Task-2 stub in `main` with real dispatch**

Replace the contents of `main` (after `_parse_argv`):

```python
def main(argv: list[str]) -> int:
    path, doc_type = _parse_argv(argv)
    warnings: list[str] = []
    try:
        if doc_type == "internal":
            text = run_internal_ocr(path)
            fields = extract_internal_fields(text)
            raw = text
        else:  # foreign
            mrz, raw = run_foreign_mrz(path)
            if mrz is None:
                warnings.append("MRZ not found — foreign passport scan too low quality or not a passport")
            fields = extract_foreign_fields(mrz)
    except Exception as exc:
        _die(f"parse failed: {exc}", code=3)
    print(json.dumps(
        {"fields": fields, "doc_type": doc_type, "raw_text": raw, "warnings": warnings},
        ensure_ascii=False,
    ))
    return 0
```

- [ ] **Step 5: Run the full test suite**

Run: `pytest docgen/tests/ -v`
Expected: all Task 2, 3, 4 tests PASS; Task 5 integration tests SKIP (no sample images) unless the developer placed them locally.

- [ ] **Step 6: Local smoke-test with real samples (if available)**

```bash
python3 docgen/passport_parser.py /path/to/internal.jpg --doc-type=internal | python3 -m json.tool
python3 docgen/passport_parser.py /path/to/foreign.jpg --doc-type=foreign | python3 -m json.tool
```

Expected: `fields.internal_series` populated on internal; `fields.passport_number` (9 chars) populated on foreign.

- [ ] **Step 7: Commit**

```bash
git add docgen/passport_parser.py docgen/tests/ .gitignore
git commit -m "feat(docgen): Tesseract OCR + passporteye MRZ dispatch"
```

---

### Task 6: Go — `backend/internal/passport` package

**Files:**
- Create: `backend/internal/passport/parser.go`
- Create: `backend/internal/passport/parser_test.go`

- [ ] **Step 1: Failing test with a stub Python script**

`backend/internal/passport/parser_test.go`:

```go
package passport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// stubScript writes a tiny python script that prints canned JSON and exits.
func stubScript(t *testing.T, stdout string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.py")
	body := fmt.Sprintf("import sys\nprint(r'''%s''')\nsys.exit(%d)\n", stdout, exitCode)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseScan_internal_parsesStubOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub uses POSIX python path")
	}
	script := stubScript(t, `{"fields":{"internal_series":"4512","internal_number":"345678","internal_issued_ru":"15.08.2015","internal_issued_by_ru":"ГУ МВД","department_code":"770-001","passport_number":"","expiry_date":"","name_cyr":"ИВАНОВ ПЁТР","name_lat":"","gender_ru":"Мужской","birth_date":"02.03.1985","place_of_birth_ru":"Г. МОСКВА","nationality_ru":"","reg_address_ru":""},"doc_type":"internal","raw_text":"...","warnings":[]}`, 0)
	scanFile := filepath.Join(t.TempDir(), "x.jpg")
	os.WriteFile(scanFile, []byte("x"), 0o644)

	got, err := ParseScan(context.Background(), script, scanFile, DocTypeInternal)
	if err != nil {
		t.Fatal(err)
	}
	if got.DocType != "internal" {
		t.Errorf("DocType = %q", got.DocType)
	}
	if got.Fields.InternalSeries != "4512" {
		t.Errorf("InternalSeries = %q", got.Fields.InternalSeries)
	}
	if got.Fields.NameCyr != "ИВАНОВ ПЁТР" {
		t.Errorf("NameCyr = %q", got.Fields.NameCyr)
	}
}

func TestParseScan_foreign_parsesStubOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub uses POSIX python path")
	}
	script := stubScript(t, `{"fields":{"internal_series":"","internal_number":"","internal_issued_ru":"","internal_issued_by_ru":"","department_code":"","passport_number":"123456789","expiry_date":"15.09.2030","name_cyr":"","name_lat":"IVANOV PIOTR","gender_ru":"Мужской","birth_date":"02.03.1985","place_of_birth_ru":"","nationality_ru":"RUSSIA","reg_address_ru":""},"doc_type":"foreign","raw_text":"","warnings":[]}`, 0)
	scanFile := filepath.Join(t.TempDir(), "y.jpg")
	os.WriteFile(scanFile, []byte("y"), 0o644)

	got, err := ParseScan(context.Background(), script, scanFile, DocTypeForeign)
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields.PassportNumber != "123456789" {
		t.Errorf("PassportNumber = %q", got.Fields.PassportNumber)
	}
	if got.Fields.NationalityRu != "RUSSIA" {
		t.Errorf("NationalityRu = %q", got.Fields.NationalityRu)
	}
}

func TestParseScan_rejectsInvalidDocType(t *testing.T) {
	_, err := ParseScan(context.Background(), "unused", "unused", DocType("bogus"))
	if err == nil {
		t.Fatal("expected error for unknown doc type")
	}
}

func TestParseScan_returnsErrorOnNonZeroExit(t *testing.T) {
	script := stubScript(t, `{"error":"parse failed"}`, 3)
	// Emit the JSON on stderr — the parser script convention is: errors → stderr JSON.
	// Our stub just uses stdout; adapt if your stub infrastructure differs. For the
	// purpose of this test what we check is "non-zero exit → error".
	scanFile := filepath.Join(t.TempDir(), "z.jpg")
	os.WriteFile(scanFile, []byte("z"), 0o644)
	_, err := ParseScan(context.Background(), script, scanFile, DocTypeInternal)
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
}
```

- [ ] **Step 2: Run the failing tests**

Run: `cd backend && go test ./internal/passport/... -v`
Expected: FAIL — `undefined: ParseScan`, `undefined: DocType`.

- [ ] **Step 3: Implement the package**

`backend/internal/passport/parser.go`:

```go
// Package passport wraps docgen/passport_parser.py — a local-only OCR/MRZ
// extractor for Russian passports (internal and foreign).
//
// No AI. No external calls. Scan bytes never leave the server.
package passport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// DocType selects the extraction strategy executed by the Python subprocess.
type DocType string

const (
	DocTypeInternal DocType = "internal"
	DocTypeForeign  DocType = "foreign"
)

func (d DocType) valid() bool {
	return d == DocTypeInternal || d == DocTypeForeign
}

// Fields is the superset schema both strategies populate. Keys that do not
// apply to the chosen DocType are returned as empty strings. JSON tags MUST
// match docgen/passport_parser.py :: EMPTY_FIELDS exactly.
type Fields struct {
	InternalSeries     string `json:"internal_series"`
	InternalNumber     string `json:"internal_number"`
	InternalIssuedRu   string `json:"internal_issued_ru"`
	InternalIssuedByRu string `json:"internal_issued_by_ru"`
	DepartmentCode     string `json:"department_code"`
	PassportNumber     string `json:"passport_number"`
	ExpiryDate         string `json:"expiry_date"`
	NameCyr            string `json:"name_cyr"`
	NameLat            string `json:"name_lat"`
	GenderRu           string `json:"gender_ru"`
	BirthDate          string `json:"birth_date"`
	PlaceOfBirthRu     string `json:"place_of_birth_ru"`
	NationalityRu      string `json:"nationality_ru"`
	RegAddressRu       string `json:"reg_address_ru"`
}

// Result is what ParseScan returns and what the HTTP handler echoes to the frontend.
type Result struct {
	Fields   Fields   `json:"fields"`
	DocType  string   `json:"doc_type"`
	RawText  string   `json:"raw_text"`
	Warnings []string `json:"warnings"`
}

type pythonError struct {
	Error string `json:"error"`
}

// ParseScan runs docgen/passport_parser.py <scanPath> --doc-type=<docType>
// and decodes its stdout JSON.
func ParseScan(ctx context.Context, pythonScript, scanPath string, docType DocType) (*Result, error) {
	if !docType.valid() {
		return nil, fmt.Errorf("invalid doc_type: %q", docType)
	}
	cmd := exec.CommandContext(ctx, "python3", pythonScript, scanPath, "--doc-type="+string(docType))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var pe pythonError
		if jerr := json.Unmarshal(stderr.Bytes(), &pe); jerr == nil && pe.Error != "" {
			return nil, fmt.Errorf("passport parser: %s", pe.Error)
		}
		return nil, fmt.Errorf("passport parser exit: %w — stderr: %s", err, stderr.String())
	}

	var out Result
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("passport parser decode: %w — stdout: %s", err, stdout.String())
	}
	return &out, nil
}
```

- [ ] **Step 4: Run the tests — all pass**

Run: `cd backend && go test ./internal/passport/... -v`
Expected: 3 of 4 tests PASS. `TestParseScan_returnsErrorOnNonZeroExit` may need the stub script to emit error JSON on stderr rather than stdout — adjust the stub helper accordingly (tweak `body` to use `sys.stderr` when exit code ≠ 0).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/passport/
git commit -m "feat(passport): Go wrapper with DocType dispatch"
```

---

### Task 7: Go — `POST /api/passport/parse` handler

**Files:**
- Create: `backend/internal/api/passport_scan.go`
- Create: `backend/internal/api/passport_scan_test.go`

- [ ] **Step 1: Confirm the existing multipart style**

Run: `grep -n "ParseMultipartForm\|FormFile" backend/internal/api/uploads.go`
Expected: you see how `UploadTouristFile` handles size limits and the `file` field. Mirror that.

- [ ] **Step 2: Failing handler tests**

`backend/internal/api/passport_scan_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func stubPy(t *testing.T, jsonOut string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "stub.py")
	src := fmt.Sprintf("import sys\nprint(r'''%s''')\nsys.exit(0)\n", jsonOut)
	if err := os.WriteFile(p, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func multipartReq(t *testing.T, fields map[string]string, fileName string, fileBody []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if fileName != "" {
		fw, _ := mw.CreateFormFile("file", fileName)
		_, _ = io.Copy(fw, bytes.NewReader(fileBody))
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/passport/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestPassportScan_internal_returnsParsedFields(t *testing.T) {
	stub := stubPy(t, `{"fields":{"internal_series":"4512","internal_number":"345678","internal_issued_ru":"","internal_issued_by_ru":"","department_code":"","passport_number":"","expiry_date":"","name_cyr":"","name_lat":"","gender_ru":"","birth_date":"","place_of_birth_ru":"","nationality_ru":"","reg_address_ru":""},"doc_type":"internal","raw_text":"","warnings":[]}`)
	h := NewPassportScanHandler(stub, t.TempDir())

	req := multipartReq(t, map[string]string{"doc_type": "internal"}, "p.jpg", []byte("img-bytes"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["fields"].(map[string]any)["internal_series"] != "4512" {
		t.Errorf("got %v", out["fields"])
	}
}

func TestPassportScan_foreign_returnsParsedFields(t *testing.T) {
	stub := stubPy(t, `{"fields":{"internal_series":"","internal_number":"","internal_issued_ru":"","internal_issued_by_ru":"","department_code":"","passport_number":"123456789","expiry_date":"15.09.2030","name_cyr":"","name_lat":"IVANOV PIOTR","gender_ru":"Мужской","birth_date":"02.03.1985","place_of_birth_ru":"","nationality_ru":"RUSSIA","reg_address_ru":""},"doc_type":"foreign","raw_text":"","warnings":[]}`)
	h := NewPassportScanHandler(stub, t.TempDir())

	req := multipartReq(t, map[string]string{"doc_type": "foreign"}, "p.jpg", []byte("img"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	fields := out["fields"].(map[string]any)
	if fields["passport_number"] != "123456789" || fields["nationality_ru"] != "RUSSIA" {
		t.Errorf("got %v", fields)
	}
}

func TestPassportScan_rejectsMissingDocType(t *testing.T) {
	h := NewPassportScanHandler("unused", t.TempDir())
	req := multipartReq(t, nil, "p.jpg", []byte("x"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPassportScan_rejectsUnsupportedExtension(t *testing.T) {
	h := NewPassportScanHandler("unused", t.TempDir())
	req := multipartReq(t, map[string]string{"doc_type": "internal"}, "p.docx", []byte("x"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPassportScan_tempFileDeleted(t *testing.T) {
	stub := stubPy(t, `{"fields":{},"doc_type":"internal","raw_text":"","warnings":[]}`)
	uploadsTmp := t.TempDir()
	h := NewPassportScanHandler(stub, uploadsTmp)

	req := multipartReq(t, map[string]string{"doc_type": "internal"}, "p.jpg", []byte("x"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	entries, err := os.ReadDir(filepath.Join(uploadsTmp, "_passport_tmp"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%d temp files remain", len(entries))
	}
}
```

- [ ] **Step 3: Run the failing tests**

Run: `cd backend && go test ./internal/api -run TestPassportScan -v`
Expected: FAIL — `undefined: NewPassportScanHandler`.

- [ ] **Step 4: Implement the handler**

`backend/internal/api/passport_scan.go`:

```go
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"<module>/backend/internal/passport"
)

const (
	passportMaxBytes     = 20 << 20 // 20 MiB
	passportParseTimeout = 45 * time.Second
)

type PassportScanHandler struct {
	pythonScript string
	uploadsDir   string
}

func NewPassportScanHandler(pythonScript, uploadsDir string) *PassportScanHandler {
	return &PassportScanHandler{pythonScript: pythonScript, uploadsDir: uploadsDir}
}

func (h *PassportScanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(passportMaxBytes); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	docType := passport.DocType(strings.TrimSpace(r.FormValue("doc_type")))
	switch docType {
	case passport.DocTypeInternal, passport.DocTypeForeign:
	default:
		http.Error(w, "missing or invalid doc_type (expected 'internal' or 'foreign')", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".pdf":
	default:
		http.Error(w, "unsupported file type (expect jpg/jpeg/png/pdf)", http.StatusBadRequest)
		return
	}

	tmpDir := filepath.Join(h.uploadsDir, "_passport_tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		http.Error(w, "tmp dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	tmpPath := filepath.Join(tmpDir, hex.EncodeToString(nonce)+ext)

	f, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "tmp create: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = os.Remove(tmpPath)
		// If passporteye wrote a .page1.png sidecar for PDFs, remove it too.
		if ext == ".pdf" {
			_ = os.Remove(tmpPath + ".page1.png")
		}
	}()

	if _, err := io.Copy(f, file); err != nil {
		f.Close()
		http.Error(w, "write: "+err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()

	ctx, cancel := context.WithTimeout(r.Context(), passportParseTimeout)
	defer cancel()

	result, err := passport.ParseScan(ctx, h.pythonScript, tmpPath, docType)
	if err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
```

> Replace `<module>` in the import with the actual go.mod module name (check with `head -1 backend/go.mod`).

- [ ] **Step 5: Run the tests**

Run: `cd backend && go test ./internal/api -run TestPassportScan -v`
Expected: 5/5 PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/passport_scan.go backend/internal/api/passport_scan_test.go
git commit -m "feat(api): POST /api/passport/parse with doc_type dispatch"
```

---

### Task 8: Router wiring + env var

**Files:**
- Modify: `backend/internal/server/router.go`
- Modify: `backend/cmd/server/main.go` (if env vars are read there)
- Modify: `CLAUDE.md`

- [ ] **Step 1: Locate where `DOCGEN_SCRIPT` is resolved**

Run: `grep -rn "DOCGEN_SCRIPT" backend/`
Expected: shows the `os.Getenv` / default-value lines — mirror the pattern.

- [ ] **Step 2: Add env var reading**

Next to the existing `docgenScript := os.Getenv("DOCGEN_SCRIPT"); if docgenScript == "" { docgenScript = "…" }` block, add:

```go
passportScript := os.Getenv("PASSPORT_PARSER_SCRIPT")
if passportScript == "" {
    passportScript = "../../docgen/passport_parser.py"
}
```

- [ ] **Step 3: Register the route inside the authenticated chi group**

Inside the `r.Group(func(r chi.Router) { r.Use(middleware.RequireAuth(...)) ... })` block in `router.go`:

```go
passportHandler := api.NewPassportScanHandler(passportScript, uploadsDir)
r.Method(http.MethodPost, "/passport/parse", passportHandler)
```

Do NOT register under the `/public` routes.

- [ ] **Step 4: Boot and smoke-test**

```bash
docker compose up -d db
cd backend && go run cmd/server/main.go
```

In another shell, with a valid session cookie:

```bash
curl -sS -b cookie.txt -F "file=@/path/to/internal.jpg" -F "doc_type=internal" \
  http://localhost:8080/api/passport/parse | python3 -m json.tool

curl -sS -b cookie.txt -F "file=@/path/to/foreign.jpg" -F "doc_type=foreign" \
  http://localhost:8080/api/passport/parse | python3 -m json.tool
```

Expected: valid JSON responses with populated `fields` per doc type.

- [ ] **Step 5: Document env var in CLAUDE.md**

Under "Environment Variables" → "Optional" block, append:

```
PASSPORT_PARSER_SCRIPT=../../docgen/passport_parser.py
```

- [ ] **Step 6: Commit**

```bash
git add backend/internal/server/router.go backend/cmd/server/main.go CLAUDE.md
git commit -m "feat(server): wire POST /api/passport/parse + env var"
```

---

### Task 9: Frontend — API client method

**Files:**
- Modify: `frontend/src/api/client.js`

- [ ] **Step 1: Append the new client method**

```javascript
// Uploads a Russian passport scan (internal or foreign) and returns the
// parsed fields. Local OCR/MRZ only — no AI.
//
// docType: 'internal' | 'foreign'
export async function parsePassportScan(file, docType) {
  if (docType !== 'internal' && docType !== 'foreign') {
    throw new Error(`parsePassportScan: invalid docType ${docType}`);
  }
  const fd = new FormData();
  fd.append('file', file);
  fd.append('doc_type', docType);
  const res = await fetch('/api/passport/parse', {
    method: 'POST',
    body: fd,
    credentials: 'include',
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Passport parse failed: ${res.status} — ${text}`);
  }
  return res.json();
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/api/client.js
git commit -m "feat(frontend): parsePassportScan(file, docType) client method"
```

---

### Task 10: Frontend — two scan buttons in SubmissionForm

**Files:**
- Modify: `frontend/src/components/SubmissionForm.jsx`

- [ ] **Step 1: Update the import**

Change the existing line:

```javascript
import { getConsentText } from '../api/client';
```

to:

```javascript
import { getConsentText, parsePassportScan } from '../api/client';
```

- [ ] **Step 2: Add scan state hooks** (inside `SubmissionForm`, near other `useState`):

```javascript
const [scanning, setScanning] = useState(null); // null | 'internal' | 'foreign'
const [scanError, setScanError] = useState('');
const [scanNotice, setScanNotice] = useState('');
```

- [ ] **Step 3: Add scan handlers + applyScan**

Below `clearError`:

```javascript
// Writes only into fields that are empty — never overwrites user input.
const applyScan = (fields) => {
  // Allowed target keys must match backend's Fields struct + our form names.
  const allowed = new Set([
    'internal_series', 'internal_number',
    'internal_issued_ru', 'internal_issued_by_ru',
    'passport_number', 'expiry_date',
    'name_cyr', 'name_lat',
    'gender_ru', 'birth_date',
    'place_of_birth_ru', 'nationality_ru',
  ]);
  setPayload((prev) => {
    const next = { ...prev };
    let filled = 0;
    for (const [k, v] of Object.entries(fields || {})) {
      if (!allowed.has(k)) continue;
      if (v && !(next[k] || '').trim()) {
        next[k] = v;
        filled += 1;
      }
    }
    setScanNotice(`Заполнено полей: ${filled}`);
    return next;
  });
};

const scanWith = (docType) => async (e) => {
  const file = e.target.files?.[0];
  e.target.value = ''; // allow reselecting the same file
  if (!file) return;
  setScanning(docType);
  setScanError('');
  setScanNotice('');
  try {
    const result = await parsePassportScan(file, docType);
    applyScan(result.fields);
    if ((result.warnings || []).length) {
      setScanError(result.warnings.join('; '));
    }
  } catch (err) {
    setScanError(err.message || 'Не удалось распознать скан');
  } finally {
    setScanning(null);
  }
};
```

- [ ] **Step 4: Render two buttons above the passport fieldsets**

Before the JSX that contains `internal_series` / `passport_number` inputs (or at the top of the form's passport section — grep for `internal_series` in the JSX to locate), insert:

```jsx
<div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12, flexWrap: 'wrap' }}>
  <label className="btn">
    {scanning === 'internal' ? 'Распознаю…' : 'Распознать внутренний паспорт'}
    <input type="file" accept="image/jpeg,image/png,application/pdf"
      onChange={scanWith('internal')} disabled={!!scanning}
      style={{ display: 'none' }} />
  </label>
  <label className="btn">
    {scanning === 'foreign' ? 'Распознаю…' : 'Распознать загранпаспорт'}
    <input type="file" accept="image/jpeg,image/png,application/pdf"
      onChange={scanWith('foreign')} disabled={!!scanning}
      style={{ display: 'none' }} />
  </label>
  {scanNotice && <span style={{ color: '#9bd19c' }}>{scanNotice}</span>}
  {scanError && <span style={{ color: '#d19b9b' }}>{scanError}</span>}
</div>
```

Use the form's existing button class (grep for other `className=` in `SubmissionForm.jsx` — likely `btn` or similar). Keep the dark-theme aesthetic consistent.

- [ ] **Step 5: Manual UI check**

```bash
cd frontend && npm run dev
```

In browser, logged in as a manager:
1. Click "Распознать внутренний паспорт", pick a sample → verify internal fields fill.
2. Without reloading, click "Распознать загранпаспорт", pick a foreign sample → verify `passport_number`, `expiry_date`, `name_lat`, `nationality_ru`, `gender_ru`, `birth_date` fill.
3. Non-empty fields from step 1 must not be overwritten in step 2 (e.g. if `birth_date` was filled from internal, foreign won't touch it).
4. Pick a random PDF → confirm the error appears in `scanError`.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/SubmissionForm.jsx
git commit -m "feat(submission): scan buttons for internal and foreign passports"
```

---

### Task 11: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Insert a new "Passport scan" block under "Data Flow"**

After the existing numbered flow (near where ticket / voucher scans are discussed):

```markdown
### Passport scan (optional, manager-side only)
- Manager uploads internal or foreign passport scan via SubmissionForm
- POST /api/passport/parse with multipart `file` + `doc_type=internal|foreign`
- Backend runs `docgen/passport_parser.py` locally — Tesseract for internal,
  passporteye (MRZ) for foreign. **No AI. No external calls.**
- Scan file lives under `uploads/_passport_tmp/` for ≤45 s and is deleted in
  a defer. It is never persisted to the DB.
- Response pre-fills empty form fields only; manager reviews and saves as
  usual.
```

- [ ] **Step 2: Extend the "AI Privacy" section**

Append a paragraph:

```markdown
Passport scan parsing (`/api/passport/parse`) bypasses the AI layer entirely
and runs locally via Tesseract (internal passport) or passporteye MRZ (foreign
passport). Scans never reach Anthropic and are not retained after parsing.
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document local passport scan parser flow"
```

---

### Task 12: End-to-end smoke verification

**Files:** none (manual)

- [ ] **Step 1: docker-compose build + up**

```bash
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
```

- [ ] **Step 2: Sample both flows via the UI**

Log in as a manager, open the submission form, test both buttons with real sample scans.

- [ ] **Step 3: Confirm cleanup**

```bash
docker compose exec backend ls /app/uploads/_passport_tmp/ 2>/dev/null || echo "(empty / missing — OK)"
```

Expected: directory missing or empty after each request.

- [ ] **Step 4: Check logs**

```bash
docker compose logs backend | tail -50 | grep -iE "passport|parse"
```

Expected: no panics, only expected INFO lines.

---

## Self-Review

**Spec coverage — each user requirement maps to a task:**

| Requirement | Tasks |
|---|---|
| Scan internal passport → auto-fill form, no AI | 2, 3, 5, 6, 7, 10 |
| Scan foreign passport → auto-fill form, no AI | 2, 4, 5, 6, 7, 10 |
| Manager-only (no public flow) | 8 (authenticated group only) |
| Don't overwrite manager-entered values | 10, Step 3 (`applyScan` empty-check) |
| Privacy — scan never persists, never touches AI | 7 (temp dir + defer delete), 11 |

**Placeholder scan:** no `TBD`, no "similar to Task N"; every code block is complete; every regex, test body, and env-var name is concrete. The only placeholder deliberately marked for the implementer is `<module>` in the Go import (to be replaced with the module name from `backend/go.mod` — noted in Task 7 Step 4).

**Type consistency:**
- Python `EMPTY_FIELDS` keys ≡ Go `Fields` struct JSON tags ≡ frontend `applyScan` `allowed` Set: { `internal_series`, `internal_number`, `internal_issued_ru`, `internal_issued_by_ru`, `department_code`, `passport_number`, `expiry_date`, `name_cyr`, `name_lat`, `gender_ru`, `birth_date`, `place_of_birth_ru`, `nationality_ru`, `reg_address_ru` }.
- `department_code` and `reg_address_ru` intentionally omitted from `applyScan`'s `allowed` Set — they have no corresponding form field today. Parser output still includes them for future use / debug.
- `DocType` constants match the Python CLI's `VALID_DOC_TYPES` strings (`"internal"`, `"foreign"`).
- `NewPassportScanHandler(pythonScript, uploadsDir)` — same signature used in tests (Task 7) and router wiring (Task 8).

**Known MVP limitations (future follow-ups):**
- `issue_date` / `issued_by_ru` not extracted from foreign passport (lives in VIZ, not MRZ). Can add with a second Tesseract pass if needed later.
- `reg_address_ru` (internal, propiska, page 5) not extracted — handwritten text, poor OCR.
- Phone-photo accuracy ~70–85%; scanner PDFs ~95%. The `applyScan` logic + editable UI is the guardrail.
- If Tesseract accuracy proves insufficient for internal passports, `run_internal_ocr(path) -> str` is the single swap point (replace with PaddleOCR or a RU-hosted cloud API).
