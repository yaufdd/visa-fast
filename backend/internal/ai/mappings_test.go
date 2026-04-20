package ai

import "testing"

func TestGenderMap(t *testing.T) {
	cases := map[string]string{
		"Мужской": "Male",
		"мужской": "Male",
		"Женский": "Female",
		"":        "",
		"Unknown": "",
	}
	for in, want := range cases {
		if got := MapGender(in); got != want {
			t.Errorf("MapGender(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaritalMap(t *testing.T) {
	cases := map[string]string{
		"Холост/не замужем": "Single",
		"Женат/замужем":     "Married",
		"Вдовец/вдова":      "Widowed",
		"Разведен(а)":       "Divorced",
		"":                  "",
	}
	for in, want := range cases {
		if got := MapMaritalStatus(in); got != want {
			t.Errorf("MapMaritalStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPassportTypeMap(t *testing.T) {
	cases := map[string]string{
		"Обычный":         "Ordinary",
		"Дипломатический": "Diplomatic",
		"Служебный":       "Official",
		"":                "Ordinary", // default fallback
	}
	for in, want := range cases {
		if got := MapPassportType(in); got != want {
			t.Errorf("MapPassportType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestYesNoMap(t *testing.T) {
	cases := map[string]string{
		"Нет": "No",
		"Да":  "Yes",
		"":    "No",
	}
	for in, want := range cases {
		if got := MapYesNo(in); got != want {
			t.Errorf("MapYesNo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountryISO(t *testing.T) {
	cases := map[string]string{
		"Россия":    "RUS",
		"РФ":        "RUS",
		"Казахстан": "KAZ",
		"Беларусь":  "BLR",
		"Украина":   "UKR",
		"Unknown":   "",
	}
	for in, want := range cases {
		if got := CountryISO(in); got != want {
			t.Errorf("CountryISO(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRadioButtonCodes(t *testing.T) {
	if got := GenderRB("Male"); got != "0" {
		t.Errorf("GenderRB(Male) = %q, want 0", got)
	}
	if got := GenderRB("Female"); got != "1" {
		t.Errorf("GenderRB(Female) = %q, want 1", got)
	}
	if got := MaritalRB("Married"); got != "1" {
		t.Errorf("MaritalRB(Married) = %q, want 1", got)
	}
	if got := PassportTypeRB("Ordinary"); got != "2" {
		t.Errorf("PassportTypeRB(Ordinary) = %q, want 2", got)
	}
}
