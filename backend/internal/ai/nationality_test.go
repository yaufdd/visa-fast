package ai

import "testing"

func TestComputeFormerNationality(t *testing.T) {
	cases := []struct {
		name           string
		formerRu       string
		placeOfBirthRu string
		birthDate      string // DD.MM.YYYY — kept in the signature; unused.
		want           string
	}{
		{"explicit Нет returns NO", "Нет", "Москва", "15.03.1970", "NO"},
		{"explicit no (latin) returns NO", "No", "Moscow", "15.03.1970", "NO"},
		{"explicit СССР returns USSR", "СССР", "Москва", "15.03.1985", "USSR"},
		{"explicit Soviet Union returns USSR", "Soviet Union", "Moscow", "15.03.1985", "USSR"},
		{"empty returns NO", "", "", "", "NO"},
		{"custom country (Сербия) returns NO", "Сербия", "Белград", "15.03.1985", "NO"},
		// Birth-date heuristic is gone — born before 1992 with explicit
		// "Нет" must NOT auto-promote to USSR any more.
		{"born before 1992 with explicit Нет returns NO", "Нет", "Москва", "15.03.1985", "NO"},
		// place_of_birth is no longer consulted — the form layer owns the
		// suggestion now.
		{"USSR in place_of_birth alone returns NO", "", "Москва, СССР", "15.03.1985", "NO"},
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
