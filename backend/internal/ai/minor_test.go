package ai

import "testing"

func TestIsMinorOnDate(t *testing.T) {
	cases := []struct {
		birth, departure string
		want             bool
	}{
		{"01.01.2010", "01.01.2026", true},  // 15, 16 on next bday after dep
		{"15.03.2008", "14.03.2026", true},  // turns 18 the next day
		{"15.03.2008", "15.03.2026", false}, // turns 18 on departure day
		{"15.03.2008", "16.03.2026", false},
		{"01.01.1990", "01.01.2026", false},
		{"garbage", "01.01.2026", false},
	}
	for _, c := range cases {
		got := IsMinorOnDate(c.birth, c.departure)
		if got != c.want {
			t.Errorf("IsMinorOnDate(%q,%q) = %v, want %v", c.birth, c.departure, got, c.want)
		}
	}
}

func TestFindParentBySurname(t *testing.T) {
	tourists := []TouristRef{
		{ID: "a", SurnameCyr: "Кузнецов", IsMinor: true, BirthDate: "01.01.2015"},
		{ID: "b", SurnameCyr: "Кузнецов", IsMinor: false, BirthDate: "01.01.1985"},
		{ID: "c", SurnameCyr: "Иванов", IsMinor: false, BirthDate: "01.01.1980"},
	}
	got := FindParent(tourists[0], tourists)
	if got == nil {
		t.Fatal("expected parent found")
	}
	if got.ID != "b" {
		t.Errorf("expected parent b, got %s", got.ID)
	}

	// No parent case
	tourists2 := []TouristRef{
		{ID: "x", SurnameCyr: "Петров", IsMinor: true, BirthDate: "01.01.2015"},
	}
	if FindParent(tourists2[0], tourists2) != nil {
		t.Error("expected nil when no parent in group")
	}
}
