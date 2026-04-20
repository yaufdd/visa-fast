package ai

import "testing"

func TestComputeIntendedStayDays(t *testing.T) {
	cases := []struct {
		name         string
		arrival, dep string
		want         int
	}{
		{"normal", "25.04.2026", "05.05.2026", 11},
		{"same day", "25.04.2026", "25.04.2026", 1},
		{"one-way empty departure", "25.04.2026", "", 0},
		{"cross-month", "30.04.2026", "02.05.2026", 3},
		{"invalid arrival", "garbage", "05.05.2026", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeIntendedStayDays(c.arrival, c.dep)
			if got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
