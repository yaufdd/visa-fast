package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// passportInternalOCRFixture is the OCR'd text we observed when manually
// running Yandex Vision on a real Russian internal passport (БАМБА ЭРИК).
// We embed it inline rather than committing the scan itself — only the
// derived text is needed to drive the parser tests, and keeping it inline
// lets the test be hermetic.
const passportInternalOCRFixture = `РОССИЙСКАЯ ФЕДЕРАЦИЯ
ПАСПОРТ ГРАЖДАНИНА
РОССИЙСКОЙ ФЕДЕРАЦИИ

Серия 4523 № 172344

ГУ МВД РОССИИ ПО Г.МОСКВЕ
Дата выдачи 12.05.2018
Код подразделения 770-091

Личный код

Фамилия БАМБА
Имя ЭРИК
Отчество СЕРГЕЕВИЧ
Пол МУЖ.
Дата рождения 06.10.1996
Место рождения ГОР. МОСКВА

PNRUSBAMBA<<ERIK<SERGEEVICH<<<<<<<<<<<<<<<<<
4523172344RUS9610065M<<<<<<<<<<<<<<<<<<2

--- PAGE BREAK ---

МЕСТО ЖИТЕЛЬСТВА
Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49
ОУФМС РОССИИ ПО Г. МОСКВЕ
Дата регистрации 18.05.2018`

// passportForeignOCRFixture is a synthetic foreign-passport OCR fragment.
// We don't have a real-scan capture committed; the fixture is plausible
// (matches the format of typical Russian travel-passport data pages plus
// MRZ) and good enough to drive the prompt+decode happy path.
const passportForeignOCRFixture = `RUSSIAN FEDERATION
PASSPORT / ПАСПОРТ

Type/Тип P
Country/Страна RUS
Passport No./Номер паспорта 751234567

Surname/Фамилия БАМБА / BAMBA
Given Names/Имена ЭРИК СЕРГЕЕВИЧ / ERIK SERGEEVICH
Nationality/Гражданство RUS
Date of Birth/Дата рождения 06.10.1996
Sex/Пол M / МУЖ
Place of Birth/Место рождения МОСКВА
Date of Issue/Дата выдачи 15.03.2022
Authority/Орган, выдавший документ ФМС 77810
Date of Expiry/Дата окончания срока 15.03.2032

P<RUSBAMBA<<ERIK<SERGEEVICH<<<<<<<<<<<<<<<<<
7512345674RUS9610065M3203158<<<<<<<<<<<<<<00`

func TestParsePassportScan_InternalHappyPath(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			// Yandex Vision returns one string per page — our fixture already
			// contains an embedded "--- PAGE BREAK ---" so split on it to
			// emulate two-page output authentically.
			parts := strings.Split(passportInternalOCRFixture, "\n--- PAGE BREAK ---\n")
			return parts, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `{
				"type": "ignored",
				"series": "4523",
				"number": "172344",
				"last_name": "БАМБА",
				"first_name": "ЭРИК",
				"patronymic": "СЕРГЕЕВИЧ",
				"gender": "МУЖ",
				"birth_date": "1996-10-06",
				"place_of_birth": "ГОР. МОСКВА",
				"issue_date": "2018-05-12",
				"expiry_date": "",
				"issuing_authority": "ГУ МВД РОССИИ ПО Г.МОСКВЕ",
				"department_code": "770-091",
				"reg_address": "Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49",
				"name_latin": ""
			}`, nil
		},
	}

	out, err := ParsePassportScan(context.Background(), ocr, tr, []byte("scan-bytes"), "image/jpeg", PassportInternal)
	if err != nil {
		t.Fatalf("ParsePassportScan: %v", err)
	}

	// Type is set from the caller's intent — even though the model returned
	// "ignored", the parser must override with PassportInternal.
	if out.Type != PassportInternal {
		t.Errorf("Type = %q, want %q", out.Type, PassportInternal)
	}
	if out.Series != "4523" {
		t.Errorf("Series = %q, want 4523", out.Series)
	}
	if out.Number != "172344" {
		t.Errorf("Number = %q, want 172344", out.Number)
	}
	if out.LastName != "БАМБА" {
		t.Errorf("LastName = %q, want БАМБА", out.LastName)
	}
	if out.FirstName != "ЭРИК" {
		t.Errorf("FirstName = %q, want ЭРИК", out.FirstName)
	}
	if out.Patronymic != "СЕРГЕЕВИЧ" {
		t.Errorf("Patronymic = %q, want СЕРГЕЕВИЧ", out.Patronymic)
	}
	if out.Gender != "МУЖ" {
		t.Errorf("Gender = %q, want МУЖ", out.Gender)
	}
	if out.BirthDate != "1996-10-06" {
		t.Errorf("BirthDate = %q, want 1996-10-06", out.BirthDate)
	}
	if out.PlaceOfBirth != "ГОР. МОСКВА" {
		t.Errorf("PlaceOfBirth = %q, want ГОР. МОСКВА", out.PlaceOfBirth)
	}
	if out.IssueDate != "2018-05-12" {
		t.Errorf("IssueDate = %q, want 2018-05-12", out.IssueDate)
	}
	if out.IssuingAuthor != "ГУ МВД РОССИИ ПО Г.МОСКВЕ" {
		t.Errorf("IssuingAuthor = %q, want ГУ МВД РОССИИ ПО Г.МОСКВЕ", out.IssuingAuthor)
	}
	if out.DepartmentCode != "770-091" {
		t.Errorf("DepartmentCode = %q, want 770-091", out.DepartmentCode)
	}
	if out.RegAddress != "Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49" {
		t.Errorf("RegAddress = %q, want Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49", out.RegAddress)
	}
	if out.ExpiryDate != "" {
		t.Errorf("ExpiryDate = %q, want empty (internal passport)", out.ExpiryDate)
	}
	if out.NameLatin != "" {
		t.Errorf("NameLatin = %q, want empty (internal passport)", out.NameLatin)
	}

	if ocr.calls != 1 {
		t.Errorf("ocr.calls = %d, want 1", ocr.calls)
	}
	if tr.calls != 1 {
		t.Errorf("tr.calls = %d, want 1", tr.calls)
	}
	// OCR receives the raw scan bytes + mime verbatim — Yandex residency means
	// no local redaction step; the parser must not mutate the upload payload.
	if string(ocr.requests[0].content) != "scan-bytes" {
		t.Errorf("ocr content = %q, want scan-bytes", ocr.requests[0].content)
	}
	if ocr.requests[0].mime != "image/jpeg" {
		t.Errorf("ocr mime = %q, want image/jpeg", ocr.requests[0].mime)
	}

	// Sanity-check the assembled User payload — both pages of our fixture
	// must be present and joined by the documented page-break marker.
	req := tr.requests[0]
	if !strings.Contains(req.User, "БАМБА") {
		t.Errorf("User payload missing first-page content")
	}
	if !strings.Contains(req.User, "МИТИНСКАЯ") {
		t.Errorf("User payload missing second-page content")
	}
	if !strings.Contains(req.User, "PAGE BREAK") {
		t.Errorf("User payload missing PAGE BREAK marker between pages")
	}
	if !req.JSONOutput {
		t.Errorf("JSONOutput = false, want true — extractor must use json responseFormat")
	}
	if req.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0 — extractor must be deterministic", req.Temperature)
	}
}

func TestParsePassportScan_ForeignHappyPath(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{passportForeignOCRFixture}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `{
				"series": "",
				"number": "751234567",
				"last_name": "БАМБА",
				"first_name": "ЭРИК",
				"patronymic": "СЕРГЕЕВИЧ",
				"gender": "МУЖ",
				"birth_date": "1996-10-06",
				"place_of_birth": "МОСКВА",
				"issue_date": "2022-03-15",
				"expiry_date": "2032-03-15",
				"issuing_authority": "ФМС 77810",
				"department_code": "77810",
				"reg_address": "",
				"name_latin": "BAMBA ERIK SERGEEVICH"
			}`, nil
		},
	}

	out, err := ParsePassportScan(context.Background(), ocr, tr, []byte("scan-bytes"), "application/pdf", PassportForeign)
	if err != nil {
		t.Fatalf("ParsePassportScan: %v", err)
	}

	if out.Type != PassportForeign {
		t.Errorf("Type = %q, want %q", out.Type, PassportForeign)
	}
	if out.Series != "" {
		t.Errorf("Series = %q, want empty (foreign passport)", out.Series)
	}
	if out.Number != "751234567" {
		t.Errorf("Number = %q, want 751234567", out.Number)
	}
	if out.NameLatin != "BAMBA ERIK SERGEEVICH" {
		t.Errorf("NameLatin = %q, want BAMBA ERIK SERGEEVICH", out.NameLatin)
	}
	if out.ExpiryDate != "2032-03-15" {
		t.Errorf("ExpiryDate = %q, want 2032-03-15", out.ExpiryDate)
	}
	if out.IssueDate != "2022-03-15" {
		t.Errorf("IssueDate = %q, want 2022-03-15", out.IssueDate)
	}
	if out.IssuingAuthor != "ФМС 77810" {
		t.Errorf("IssuingAuthor = %q, want ФМС 77810", out.IssuingAuthor)
	}
	if out.RegAddress != "" {
		t.Errorf("RegAddress = %q, want empty (foreign passport)", out.RegAddress)
	}
}

func TestParsePassportScan_OCRError(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return nil, errors.New("vision-503")
		},
	}
	tr := &recordingTranslator{}

	_, err := ParsePassportScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg", PassportInternal)
	if err == nil {
		t.Fatal("expected error on OCR failure, got nil")
	}
	if !strings.Contains(err.Error(), "passport ocr") {
		t.Errorf("error = %q, want substring 'passport ocr'", err)
	}
	if !strings.Contains(err.Error(), "vision-503") {
		t.Errorf("error = %q, want substring 'vision-503'", err)
	}
	// GPT must NOT be called when OCR fails — the pipeline is short-circuit.
	if tr.calls != 0 {
		t.Errorf("tr.calls on OCR failure = %d, want 0", tr.calls)
	}
}

func TestParsePassportScan_GPTError(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"page text"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return "", errors.New("gpt-429")
		},
	}

	_, err := ParsePassportScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg", PassportInternal)
	if err == nil {
		t.Fatal("expected error on GPT failure, got nil")
	}
	if !strings.Contains(err.Error(), "passport gpt") {
		t.Errorf("error = %q, want substring 'passport gpt'", err)
	}
	if !strings.Contains(err.Error(), "gpt-429") {
		t.Errorf("error = %q, want substring 'gpt-429'", err)
	}
}

func TestParsePassportScan_InvalidJSON(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"page text"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return "this is not json at all", nil
		},
	}

	_, err := ParsePassportScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg", PassportInternal)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want substring 'decode'", err)
	}
}

func TestParsePassportScan_UnknownType(t *testing.T) {
	// Unknown type must fail BEFORE any OCR / GPT call — guards against
	// burning Yandex quota on a malformed caller request.
	ocr := &fakeOCR{}
	tr := &recordingTranslator{}

	_, err := ParsePassportScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg", PassportType(""))
	if err == nil {
		t.Fatal("expected error on unknown type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("error = %q, want substring 'unknown type'", err)
	}
	if ocr.calls != 0 {
		t.Errorf("ocr.calls on unknown type = %d, want 0 (must short-circuit)", ocr.calls)
	}
	if tr.calls != 0 {
		t.Errorf("tr.calls on unknown type = %d, want 0 (must short-circuit)", tr.calls)
	}
}

func TestParsePassportScan_PromptVariesByType(t *testing.T) {
	// Two parser invocations with different PassportType values must produce
	// distinct system prompts — guards against accidental wiring where a
	// future refactor sends the wrong prompt for one of the variants.
	mkOCR := func() *fakeOCR {
		return &fakeOCR{
			respond: func(_ []byte, _ string) ([]string, error) {
				return []string{"page text"}, nil
			},
		}
	}
	mkTr := func() *recordingTranslator {
		return &recordingTranslator{
			respond: func(_ yandex.ChatRequest) (string, error) {
				return `{}`, nil
			},
		}
	}

	internalTr := mkTr()
	if _, err := ParsePassportScan(context.Background(), mkOCR(), internalTr, []byte("x"), "image/jpeg", PassportInternal); err != nil {
		t.Fatalf("ParsePassportScan(internal): %v", err)
	}
	foreignTr := mkTr()
	if _, err := ParsePassportScan(context.Background(), mkOCR(), foreignTr, []byte("x"), "image/jpeg", PassportForeign); err != nil {
		t.Fatalf("ParsePassportScan(foreign): %v", err)
	}

	if internalTr.calls != 1 || foreignTr.calls != 1 {
		t.Fatalf("translator call counts: internal=%d foreign=%d, want 1 each", internalTr.calls, foreignTr.calls)
	}

	internalSystem := internalTr.requests[0].System
	foreignSystem := foreignTr.requests[0].System

	if internalSystem == foreignSystem {
		t.Fatal("internal and foreign system prompts are identical — must differ")
	}

	// Spot-check distinguishing tokens unique to each prompt.
	if !strings.Contains(internalSystem, "internal civil passport") {
		t.Errorf("internal prompt missing 'internal civil passport' phrase")
	}
	if !strings.Contains(internalSystem, "department_code") {
		t.Errorf("internal prompt missing department_code field reference")
	}
	if !strings.Contains(internalSystem, "reg_address") {
		t.Errorf("internal prompt missing reg_address field reference")
	}

	if !strings.Contains(foreignSystem, "foreign (travel) passport") {
		t.Errorf("foreign prompt missing 'foreign (travel) passport' phrase")
	}
	if !strings.Contains(foreignSystem, "name_latin") {
		t.Errorf("foreign prompt missing name_latin field reference")
	}
	if !strings.Contains(foreignSystem, "expiry_date") {
		t.Errorf("foreign prompt missing expiry_date field reference")
	}

	// Both must request deterministic JSON output.
	for label, req := range map[string]yandex.ChatRequest{
		"internal": internalTr.requests[0],
		"foreign":  foreignTr.requests[0],
	} {
		if !req.JSONOutput {
			t.Errorf("%s: JSONOutput = false, want true", label)
		}
		if req.Temperature != 0 {
			t.Errorf("%s: Temperature = %v, want 0", label, req.Temperature)
		}
	}
}

func TestParsePassportScan_NilClients(t *testing.T) {
	// Defensive: the handler-side wiring should always pass real clients,
	// but a nil interface should fail fast with a clear error rather than
	// a nil-pointer panic.
	_, err := ParsePassportScan(context.Background(), nil, &recordingTranslator{}, []byte("x"), "image/jpeg", PassportInternal)
	if err == nil || !strings.Contains(err.Error(), "ocr") {
		t.Errorf("nil-ocr error = %v, want substring 'ocr'", err)
	}

	_, err = ParsePassportScan(context.Background(), &fakeOCR{}, nil, []byte("x"), "image/jpeg", PassportInternal)
	if err == nil || !strings.Contains(err.Error(), "translator") {
		t.Errorf("nil-translator error = %v, want substring 'translator'", err)
	}
}
