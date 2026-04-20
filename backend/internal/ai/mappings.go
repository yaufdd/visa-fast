package ai

import "strings"

// MapGender converts Russian gender values to English.
func MapGender(ru string) string {
	switch strings.ToLower(strings.TrimSpace(ru)) {
	case "мужской", "м":
		return "Male"
	case "женский", "ж":
		return "Female"
	}
	return ""
}

// MapMaritalStatus converts Russian marital status to English.
func MapMaritalStatus(ru string) string {
	switch strings.TrimSpace(ru) {
	case "Холост/не замужем", "Холост", "Не замужем":
		return "Single"
	case "Женат/замужем", "Женат", "Замужем":
		return "Married"
	case "Вдовец/вдова", "Вдовец", "Вдова":
		return "Widowed"
	case "Разведен(а)", "Разведен", "Разведена":
		return "Divorced"
	}
	return ""
}

// MapPassportType converts Russian passport type. Empty/unknown defaults
// to "Ordinary" (the overwhelming majority of Russian passports).
func MapPassportType(ru string) string {
	switch strings.TrimSpace(ru) {
	case "Дипломатический":
		return "Diplomatic"
	case "Служебный":
		return "Official"
	default:
		return "Ordinary"
	}
}

// MapYesNo normalises Russian Yes/No. Empty defaults to "No".
func MapYesNo(ru string) string {
	switch strings.ToLower(strings.TrimSpace(ru)) {
	case "да":
		return "Yes"
	}
	return "No"
}

// countryISOMap covers post-Soviet countries and common Russian
// passport-issued nationalities. Extend as needed.
var countryISOMap = map[string]string{
	"Россия":               "RUS",
	"РФ":                   "RUS",
	"Российская Федерация": "RUS",
	"Казахстан":            "KAZ",
	"Беларусь":             "BLR",
	"Белоруссия":           "BLR",
	"Украина":              "UKR",
	"Узбекистан":           "UZB",
	"Киргизия":             "KGZ",
	"Кыргызстан":           "KGZ",
	"Таджикистан":          "TJK",
	"Туркменистан":         "TKM",
	"Армения":              "ARM",
	"Азербайджан":          "AZE",
	"Грузия":               "GEO",
	"Молдова":              "MDA",
	"Латвия":               "LVA",
	"Литва":                "LTU",
	"Эстония":              "EST",
}

// CountryISO returns the 3-letter ISO code for a Russian country name.
// Empty string if unknown.
func CountryISO(ru string) string {
	return countryISOMap[strings.TrimSpace(ru)]
}

// PDF radio-button codes (from existing docgen/generate.py contract)

// GenderRB: "0" = Male, "1" = Female
func GenderRB(gender string) string {
	if gender == "Female" {
		return "1"
	}
	return "0"
}

// MaritalRB: "0" = Single, "1" = Married, "2" = Widowed, "3" = Divorced
func MaritalRB(status string) string {
	switch status {
	case "Married":
		return "1"
	case "Widowed":
		return "2"
	case "Divorced":
		return "3"
	}
	return "0"
}

// PassportTypeRB: "0" = Diplomatic, "1" = Official, "2" = Ordinary, "3" = Other
func PassportTypeRB(pt string) string {
	switch pt {
	case "Diplomatic":
		return "0"
	case "Official":
		return "1"
	case "Other":
		return "3"
	}
	return "2"
}
