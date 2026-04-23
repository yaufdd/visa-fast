package format

import "testing"

func TestFormatRussianField_shortAbbreviationsGetDotAndLowercase(t *testing.T) {
	cases := map[string]string{
		"д4 кв23":  "д. 4, кв. 23",
		"Д.4 КВ.23": "д. 4, кв. 23",
		"г москва":  "г. Москва",
		"ул ленина": "ул. Ленина",
	}
	for in, want := range cases {
		if got := FormatRussianField(in); got != want {
			t.Errorf("FormatRussianField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatRussianField_fullAddress(t *testing.T) {
	cases := map[string]string{
		"г москва ул ленина д 5 кв 12":         "г. Москва, ул. Ленина, д. 5, кв. 12",
		"Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49": "г. Москва, ул. Митинская, д. 12, кв. 49",
		"г. Санкт-Петербург, ул. Пушкина, д. 3": "г. Санкт-Петербург, ул. Пушкина, д. 3",
	}
	for in, want := range cases {
		if got := FormatRussianField(in); got != want {
			t.Errorf("FormatRussianField(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
	}
}

func TestFormatRussianField_issuingAuthorities(t *testing.T) {
	cases := map[string]string{
		"уфмс россии по г москве":    "УФМС России по г. Москве",
		"ГУ МВД РОССИИ ПО Г. МОСКВЕ": "ГУ МВД России по г. Москве",
		"ГУ МВД России по г.Москве":  "ГУ МВД России по г. Москве",
	}
	for in, want := range cases {
		if got := FormatRussianField(in); got != want {
			t.Errorf("FormatRussianField(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
	}
}

func TestFormatRussianField_hyphenatedAbbreviations(t *testing.T) {
	in := "г москва р-н тверской пр-т мира д 5"
	want := "г. Москва, р-н. Тверской, пр-т. Мира, д. 5"
	if got := FormatRussianField(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRussianField_hyphenatedProperNoun(t *testing.T) {
	if got := FormatRussianField("сиракава-го"); got != "Сиракава-Го" {
		t.Errorf("got %q", got)
	}
	if got := FormatRussianField("иванов-петров петр иванович"); got != "Иванов-Петров Петр Иванович" {
		t.Errorf("got %q", got)
	}
}

func TestFormatRussianField_digitsPreservedExactly(t *testing.T) {
	cases := map[string]string{
		"770-001":                   "770-001",
		"код 77011":                 "Код 77011",
		"мвд 77810":                 "МВД 77810",
		"отделение № 3 д 5 кв 12-а": "Отделение № 3, д. 5, кв. 12-а",
	}
	for in, want := range cases {
		if got := FormatRussianField(in); got != want {
			t.Errorf("FormatRussianField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatRussianField_functionWordsStayLowercase(t *testing.T) {
	in := "УФМС РОССИИ ПО Г. МОСКВЕ В РАЙОНЕ КРЫЛАТСКОЕ"
	want := "УФМС России по г. Москве в Районе Крылатское"
	if got := FormatRussianField(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRussianField_empty(t *testing.T) {
	if got := FormatRussianField(""); got != "" {
		t.Errorf("empty → %q", got)
	}
	if got := FormatRussianField("   "); got != "" {
		t.Errorf("whitespace → %q", got)
	}
	if got := FormatRussianField(","); got != "" {
		t.Errorf("comma-only → %q", got)
	}
}

func TestFormatRussianField_aliasesMatchMainFn(t *testing.T) {
	in := "д4 кв23"
	if FormatAddress(in) != FormatRussianField(in) {
		t.Error("FormatAddress should alias FormatRussianField")
	}
	if FormatIssuingAuthority(in) != FormatRussianField(in) {
		t.Error("FormatIssuingAuthority should alias FormatRussianField")
	}
}

func TestFormatRussianField_idempotent(t *testing.T) {
	// Formatting an already-formatted string should produce the same output.
	in := "г. Москва, ул. Ленина, д. 5, кв. 12"
	once := FormatRussianField(in)
	twice := FormatRussianField(once)
	if once != twice {
		t.Errorf("not idempotent:\n  once:  %q\n  twice: %q", once, twice)
	}
}
