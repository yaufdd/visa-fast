package ai

import (
	"strings"
	"time"
)

// TouristRef is a thin reference to a tourist used for minor-detection /
// parent-matching logic. It intentionally avoids coupling to the full
// Tourist DB type so this package stays DB-free.
type TouristRef struct {
	ID         string
	SurnameCyr string // first word of name_cyr
	BirthDate  string // DD.MM.YYYY
	IsMinor    bool
}

// IsMinorOnDate returns true if the tourist is under 18 as of the given
// departure date. Dates are DD.MM.YYYY. Unparseable dates → false.
func IsMinorOnDate(birthDate, departureDate string) bool {
	birth, bErr := time.Parse("02.01.2006", strings.TrimSpace(birthDate))
	dep, dErr := time.Parse("02.01.2006", strings.TrimSpace(departureDate))
	if bErr != nil || dErr != nil {
		return false
	}
	eighteenth := birth.AddDate(18, 0, 0)
	// Minor if they haven't reached their 18th birthday by departure.
	return dep.Before(eighteenth)
}

// FindParent returns the first adult tourist in the group with the same
// surname as the minor, or nil if none found.
func FindParent(minor TouristRef, group []TouristRef) *TouristRef {
	target := strings.ToLower(strings.TrimSpace(minor.SurnameCyr))
	if target == "" {
		return nil
	}
	for i := range group {
		t := group[i]
		if t.ID == minor.ID || t.IsMinor {
			continue
		}
		if strings.ToLower(strings.TrimSpace(t.SurnameCyr)) == target {
			return &t
		}
	}
	return nil
}

// FirstWord returns the first whitespace-separated word of a string,
// useful for extracting surname from "Фамилия Имя".
func FirstWord(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, " \t"); idx >= 0 {
		return s[:idx]
	}
	return s
}
