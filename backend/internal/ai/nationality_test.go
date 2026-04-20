package ai

import "testing"

func TestComputeFormerNationality(t *testing.T) {
	cases := []struct {
		name           string
		formerRu       string
		placeOfBirthRu string
		birthDate      string // DD.MM.YYYY
		want           string
	}{
		{"explicit USSR", "СССР", "Москва", "15.03.1970", "USSR"},
		{"explicit Soviet Union", "Soviet Union", "Moscow", "15.03.1970", "USSR"},
		{"USSR in place of birth", "", "Москва, СССР", "15.03.1970", "USSR"},
		{"born before end of USSR (Dec 1991)", "", "Москва", "15.03.1985", "USSR"},
		{"born on dissolution day", "", "Москва", "25.12.1991", "USSR"},
		{"born after USSR", "", "Москва", "15.03.1995", "NO"},
		{"empty everything", "", "", "", "NO"},
		{"malformed birth date, no USSR elsewhere", "", "Almaty", "garbage", "NO"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeFormerNationality(c.formerRu, c.placeOfBirthRu, c.birthDate)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
