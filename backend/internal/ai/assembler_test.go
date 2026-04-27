package ai

import (
	"strings"
	"testing"
)

func TestAssembleTourist_MaidenNameTransliterated(t *testing.T) {
	payload := map[string]any{
		"name_cyr":              "Иванова Анна",
		"name_lat":              "IVANOVA ANNA",
		"maiden_name_ru":        "Петрова",
		"gender_ru":             "Женский",
		"marital_status_ru":     "Замужем",
		"passport_type_ru":      "Обычный",
		"criminal_record_ru":    "Нет",
		"been_to_japan_ru":      "Нет",
		"birth_date":            "15.03.1980",
		"nationality_ru":        "Россия",
		"former_nationality_ru": "",
		"place_of_birth_ru":     "Москва",
	}
	translations := map[string]string{
		"Москва": "Moscow",
	}
	flight := map[string]any{
		"arrival":   map[string]any{"flight_number": "SU 262", "date": "25.04.2026", "time": "12:45", "airport": "TOKYO NARITA"},
		"departure": map[string]any{"flight_number": "SU 263", "date": "05.05.2026", "time": "14:20", "airport": "TOKYO NARITA"},
	}

	got := AssembleTourist(payload, translations, nil, flight)

	if got.MaidenName != "PETROVA" {
		t.Errorf("maiden_name = %q, want PETROVA", got.MaidenName)
	}
	if got.Gender != "Female" {
		t.Errorf("gender = %q", got.Gender)
	}
	if got.GenderRB != "1" {
		t.Errorf("gender_rb = %q", got.GenderRB)
	}
	if got.MaritalStatus != "Married" {
		t.Errorf("marital_status = %q", got.MaritalStatus)
	}
	if got.PlaceOfBirth != "Moscow" {
		t.Errorf("place_of_birth = %q", got.PlaceOfBirth)
	}
	if got.NationalityISO != "RUS" {
		t.Errorf("nationality_iso = %q", got.NationalityISO)
	}
	if got.IntendedStayDays != 11 {
		t.Errorf("intended_stay_days = %d, want 11", got.IntendedStayDays)
	}
	if got.ArrivalFlight != "SU 262" {
		t.Errorf("arrival_flight = %q", got.ArrivalFlight)
	}
}

func TestAssembleTourist_HomeAddressUsesTranslation(t *testing.T) {
	// Primary path: translate batch produced an English address →
	// анкета field is the full English, NOT the latinised Russian.
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"home_address_ru":  "Москва, ул. Митинская, д. 12, кв. 49",
	}
	translations := map[string]string{
		"Москва, ул. Митинская, д. 12, кв. 49": "Moscow, Mitinskaya St. 12, Apt. 49",
	}
	// cleanedDoverenost may also contain this raw key (it's used for the
	// доверенность Russian output) — the анкета must still pick the
	// English version from translations.
	cleaned := map[string]string{
		"Москва, ул. Митинская, д. 12, кв. 49": "г. Москва, ул. Митинская, д. 12, кв. 49",
	}
	got := AssembleTourist(payload, translations, cleaned, nil)
	if got.HomeAddress != "Moscow, Mitinskaya St. 12, Apt. 49" {
		t.Errorf("HomeAddress = %q, want English translation %q",
			got.HomeAddress, "Moscow, Mitinskaya St. 12, Apt. 49")
	}
}

func TestAssembleTourist_HomeAddressFallsBackToCleanedICAO(t *testing.T) {
	// Translation missing → fall back to ICAO transliteration of the
	// cleaned (Russian) form. Used when the translator failed for this
	// particular string but doverenost-clean still ran.
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"home_address_ru":  "москва ул ленина д5 кв12",
	}
	cleaned := map[string]string{
		"москва ул ленина д5 кв12": "г. Москва, ул. Ленина, д. 5, кв. 12",
	}
	got := AssembleTourist(payload, nil, cleaned, nil)
	if !strings.Contains(strings.ToLower(got.HomeAddress), "moskva") ||
		!strings.Contains(strings.ToLower(got.HomeAddress), "lenina") {
		t.Errorf("HomeAddress = %q, expected ICAO of cleaned form", got.HomeAddress)
	}
}

func TestAssembleTourist_HomeAddressEmptyTranslationFallsBackToCleaned(t *testing.T) {
	// Translation present but empty string → still treat as missing and
	// fall back. Guards against translator returning "" for a row.
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"home_address_ru":  "москва ул ленина д5",
	}
	translations := map[string]string{
		"москва ул ленина д5": "", // translator returned empty
	}
	cleaned := map[string]string{
		"москва ул ленина д5": "г. Москва, ул. Ленина, д. 5",
	}
	got := AssembleTourist(payload, translations, cleaned, nil)
	if got.HomeAddress == "" {
		t.Errorf("HomeAddress empty — should fall back to cleaned + ICAO")
	}
	if strings.Contains(got.HomeAddress, " St.") || strings.Contains(got.HomeAddress, "Apt.") {
		t.Errorf("HomeAddress = %q, expected ICAO fallback (no English address parts)", got.HomeAddress)
	}
}

func TestAssembleTourist_HomeAddressFallbackWhenNoMaps(t *testing.T) {
	// Last-resort: no translations, no cleaned map → raw Russian → ICAO.
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"home_address_ru":  "москва ул ленина д5",
	}
	got := AssembleTourist(payload, nil, nil, nil)
	if got.HomeAddress == "" {
		t.Errorf("HomeAddress empty — should fall back to raw + ICAO")
	}
}

func TestAssembleDoverenost_UsesCleanedMap(t *testing.T) {
	tourists := []Pass2Tourist{
		{NameCyr: "Иванов Петр Иванович", BirthDate: "10.06.1990", Gender: "Male"},
	}
	payloads := []map[string]any{
		{
			"internal_series":       "4500",
			"internal_number":       "123456",
			"internal_issued_ru":    "01.02.2015",
			"internal_issued_by_ru": "оуфмс россии по г москве",
			"reg_address_ru":        "москва ул ленина д5",
		},
	}
	cleaned := map[string]string{
		"оуфмс россии по г москве": "ОУФМС России по г. Москве",
		"москва ул ленина д5":      "г. Москва, ул. Ленина, д. 5",
	}
	out := AssembleDoverenost(tourists, payloads, cleaned, "")
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].IssuedBy != "ОУФМС России по г. Москве" {
		t.Errorf("IssuedBy = %q, want canonical form from cleaned map", out[0].IssuedBy)
	}
	if out[0].RegAddress != "г. Москва, ул. Ленина, д. 5" {
		t.Errorf("RegAddress = %q, want canonical form from cleaned map", out[0].RegAddress)
	}
}

func TestAssembleDoverenost_IgnoresTranslationsMap(t *testing.T) {
	// Regression for two-version model: even when the translate batch has
	// an English version of the same raw address, the доверенность output
	// must use the doverenost-clean Russian form (the translations map is
	// not even passed to AssembleDoverenost — but documenting via test).
	tourists := []Pass2Tourist{
		{NameCyr: "Иванов Петр", BirthDate: "10.06.1990", Gender: "Male"},
	}
	payloads := []map[string]any{
		{
			"internal_issued_by_ru": "оуфмс россии по г москве",
			"reg_address_ru":        "москва ул ленина д5",
		},
	}
	cleaned := map[string]string{
		"оуфмс россии по г москве": "ОУФМС России по г. Москве",
		"москва ул ленина д5":      "г. Москва, ул. Ленина, д. 5",
	}
	out := AssembleDoverenost(tourists, payloads, cleaned, "")
	// Russian-formatted, not English.
	if out[0].IssuedBy != "ОУФМС России по г. Москве" {
		t.Errorf("IssuedBy = %q, want canonical Russian", out[0].IssuedBy)
	}
	if out[0].RegAddress != "г. Москва, ул. Ленина, д. 5" {
		t.Errorf("RegAddress = %q, want canonical Russian", out[0].RegAddress)
	}
	// Sanity: not an English transliteration / translation.
	for _, s := range []string{out[0].IssuedBy, out[0].RegAddress} {
		if strings.Contains(s, " St.") || strings.Contains(s, "Apt.") || strings.Contains(s, "Moscow") {
			t.Errorf("доверенность field looks English: %q", s)
		}
	}
}

func TestAssembleDoverenost_FallbackToRawWhenMissing(t *testing.T) {
	tourists := []Pass2Tourist{
		{NameCyr: "Иванов Петр", BirthDate: "10.06.1990", Gender: "Male"},
	}
	payloads := []map[string]any{
		{
			"internal_issued_by_ru": "оуфмс по г москве",
			"reg_address_ru":        "москва ул ленина",
		},
	}
	// nil map → raw value flows through unchanged.
	out := AssembleDoverenost(tourists, payloads, nil, "")
	if out[0].IssuedBy != "оуфмс по г москве" {
		t.Errorf("IssuedBy = %q, want raw fallback", out[0].IssuedBy)
	}
	if out[0].RegAddress != "москва ул ленина" {
		t.Errorf("RegAddress = %q, want raw fallback", out[0].RegAddress)
	}
}

func TestAssembleTourist_OccupationCategories(t *testing.T) {
	// The Работа section in the form maps a small set of Russian markers
	// to fixed English values without going through the translator.
	cases := []struct {
		ru   string
		want string
	}{
		{"ИП", "INDIVIDUAL ENTREPRENEUR"},
		{"ип", "INDIVIDUAL ENTREPRENEUR"}, // case-insensitive
		{"Пенсионер", "PENSIONER"},
		{"Домохозяйка", "HOUSEWIFE"},
		{"Безработный", "UNEMPLOYED"},
		{"Студент", "STUDENT"},
		{"Школьник", "STUDENT"}, // visa form doesn't distinguish
	}
	for _, c := range cases {
		t.Run(c.ru, func(t *testing.T) {
			payload := map[string]any{
				"name_cyr":         "Иванов Петр",
				"gender_ru":        "Мужской",
				"passport_type_ru": "Обычный",
				"birth_date":       "10.06.1990",
				"occupation_ru":    c.ru,
			}
			// Translator entry would normally win, but the switch must take
			// precedence for these categories.
			translations := map[string]string{
				c.ru: "WRONG_TRANSLATION",
			}
			got := AssembleTourist(payload, translations, nil, nil)
			if got.Occupation != c.want {
				t.Errorf("occupation_ru=%q → Occupation=%q, want %q", c.ru, got.Occupation, c.want)
			}
		})
	}
}

func TestAssembleTourist_EmployerPhonePassthrough(t *testing.T) {
	// Regression net for a recurring bug: the form collects
	// employer_phone ("Телефон организации"), but if we don't carry it
	// through Pass2Tourist → docgen, the анкета PDF silently falls back
	// to the template's baked-in default (the infamous "...03" number).
	//
	// The phone is taken raw from the submission_snapshot — no
	// translation, no auto-fill, no formatting. Whatever the user typed
	// (or the OccupationStep auto-filled) lands here verbatim.
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"employer_ru":      "ООО Ромашка",
		"employer_phone":   "+7 999 123 4567",
	}
	// Translator must NOT touch employer_phone — sanity-check that even
	// a misconfigured translations map can't corrupt the raw value.
	translations := map[string]string{
		"+7 999 123 4567": "WRONG_TRANSLATION",
	}
	got := AssembleTourist(payload, translations, nil, nil)
	if got.EmployerPhone != "+7 999 123 4567" {
		t.Errorf("EmployerPhone = %q, want raw passthrough %q",
			got.EmployerPhone, "+7 999 123 4567")
	}

	// And confirm an empty source stays empty (no spurious fallback).
	got2 := AssembleTourist(map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
	}, nil, nil, nil)
	if got2.EmployerPhone != "" {
		t.Errorf("EmployerPhone with empty payload = %q, want \"\"", got2.EmployerPhone)
	}
}

func TestAssembleTourist_OneWay(t *testing.T) {
	payload := map[string]any{
		"name_cyr":         "Сидоров Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
	}
	flight := map[string]any{
		"arrival":   map[string]any{"flight_number": "SU 262", "date": "25.04.2026", "time": "12:45", "airport": "TOKYO NARITA"},
		"departure": map[string]any{}, // empty — one-way
	}

	got := AssembleTourist(payload, nil, nil, flight)
	if got.IntendedStayDays != 0 {
		t.Errorf("one-way intended_stay_days = %d, want 0", got.IntendedStayDays)
	}
	if got.DepartureFlight != "" {
		t.Errorf("one-way departure_flight = %q, want empty", got.DepartureFlight)
	}
}
