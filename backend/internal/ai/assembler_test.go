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

func TestAssembleTourist_HomeAddressUsesCleanedMap(t *testing.T) {
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
	// HomeAddress is the cleaned Russian → ICAO transliteration. We check
	// the prefix that the canonical form produces ("g. Moskva, ul. Lenina")
	// rather than the full string so transliteration tweaks elsewhere
	// don't break this test.
	if !strings.Contains(strings.ToLower(got.HomeAddress), "moskva") ||
		!strings.Contains(strings.ToLower(got.HomeAddress), "lenina") {
		t.Errorf("HomeAddress = %q, expected transliteration of cleaned form", got.HomeAddress)
	}
}

func TestAssembleTourist_HomeAddressFallbackWhenNoCleanedMap(t *testing.T) {
	payload := map[string]any{
		"name_cyr":         "Иванов Петр",
		"gender_ru":        "Мужской",
		"passport_type_ru": "Обычный",
		"birth_date":       "10.06.1990",
		"home_address_ru":  "москва ул ленина д5",
	}
	// nil cleaned map → falls back to raw value, then transliterated.
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
