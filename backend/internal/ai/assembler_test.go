package ai

import (
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

	got := AssembleTourist(payload, translations, flight)

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

	got := AssembleTourist(payload, nil, flight)
	if got.IntendedStayDays != 0 {
		t.Errorf("one-way intended_stay_days = %d, want 0", got.IntendedStayDays)
	}
	if got.DepartureFlight != "" {
		t.Errorf("one-way departure_flight = %q, want empty", got.DepartureFlight)
	}
}
