package ai

import "time"

// ComputeIntendedStayDays returns (departure - arrival) + 1 in days.
// Returns 0 for one-way tickets (empty departure) or unparseable dates.
// Dates are DD.MM.YYYY.
func ComputeIntendedStayDays(arrival, departure string) int {
	if departure == "" {
		return 0
	}
	a, aErr := time.Parse("02.01.2006", arrival)
	d, dErr := time.Parse("02.01.2006", departure)
	if aErr != nil || dErr != nil {
		return 0
	}
	return int(d.Sub(a).Hours()/24) + 1
}
