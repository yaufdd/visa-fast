package ai

import "testing"

func TestGenitiveCase(t *testing.T) {
	cases := []struct {
		name      string
		surname   string
		firstName string
		gender    string // "Male" / "Female"
		wantName  string
		wantTag   bool // if true, expect "[ПРОВЕРЬТЕ ПАДЕЖ]" suffix
	}{
		{"male consonant surname + consonant first", "Кузнецов", "Александр", "Male",
			"Кузнецова Александра", false},
		{"male -й first name", "Иванов", "Андрей", "Male",
			"Иванова Андрея", false},
		{"female -а surname, consonant first", "Кузнецова", "Анна", "Female",
			"Кузнецовой Анны", false},
		{"female -ая surname, -я first", "Преображенская", "Мария", "Female",
			"Преображенской Марии", false},
		{"female -а after ш", "Петрова", "Даша", "Female",
			"Петровой Даши", false},
		{"edge: male vowel ending", "Дурново", "Ари", "Male",
			"Дурново Ари [ПРОВЕРЬТЕ ПАДЕЖ]", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, tagged := GenitiveFullName(c.surname, c.firstName, c.gender)
			if got != c.wantName {
				t.Errorf("name got %q, want %q", got, c.wantName)
			}
			if tagged != c.wantTag {
				t.Errorf("tagged got %v, want %v", tagged, c.wantTag)
			}
		})
	}
}
