package ai

import (
	"fmt"
	"strings"
	"time"

	"fujitravel-admin/backend/internal/translit"
)

// cleanedLookup returns m[s] when present and non-empty, otherwise s
// itself. Used by AssembleTourist / AssembleDoverenost to pick up the
// pre-cleaned doverenost free-text fields produced by
// CleanDoverenostFields. A nil map or missing key is the no-op
// fallback so unit tests that do not exercise the cleaner still work.
func cleanedLookup(m map[string]string, s string) string {
	if s == "" {
		return ""
	}
	if m != nil {
		if c, ok := m[s]; ok && c != "" {
			return c
		}
	}
	return s
}

// AssembleTourist builds one Pass2Tourist from the submission payload,
// translations map, and flight_data.
// Arguments use untyped maps because they come from JSONB columns — the
// caller (orchestrator) already has them as map[string]any.
//
// cleanedDoverenost is a raw → canonical map of free-text Russian
// fields (home/registration addresses, internal-passport issuing
// authority) produced by CleanDoverenostFields once per /generate run
// upstream. Pass nil in tests that do not exercise that path: the
// lookup falls back to the raw string (CLAUDE.md explicitly notes this
// is no longer canonical Russian, but the rest of the assembler is
// orthogonal to cleanup so unit tests still get deterministic output).
func AssembleTourist(payload map[string]any, translations map[string]string, cleanedDoverenost map[string]string, flight map[string]any) Pass2Tourist {
	get := func(k string) string {
		if v, ok := payload[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	tr := func(k string) string {
		src := get(k)
		if src == "" {
			return ""
		}
		if translations != nil {
			if t, ok := translations[src]; ok && t != "" {
				return t
			}
		}
		return src // fallback to raw if no translation
	}

	gender := MapGender(get("gender_ru"))
	marital := MapMaritalStatus(get("marital_status_ru"))
	passportType := MapPassportType(get("passport_type_ru"))

	// "ИП" is a self-employment marker entered via the form checkbox — the
	// anketa PDF expects the full English term in caps, and we don't want
	// to hand this deterministic phrase to the translator (which may
	// produce "IE", "Individual Entrepreneur", etc.).
	occupation := tr("occupation_ru")
	if strings.EqualFold(strings.TrimSpace(get("occupation_ru")), "ИП") {
		occupation = "INDIVIDUAL ENTREPRENEUR"
	}

	arrival := subMap(flight, "arrival")
	departure := subMap(flight, "departure")

	stayDays := ComputeIntendedStayDays(strGet(arrival, "date"), strGet(departure, "date"))

	return Pass2Tourist{
		NameLat:           firstNonEmpty(get("name_lat"), translit.RuToLatICAO(get("name_cyr"))),
		NameCyr:           get("name_cyr"),
		PassportNumber:    get("passport_number"),
		BirthDate:         get("birth_date"),
		Nationality:       strings.ToUpper(tr("nationality_ru")),
		PlaceOfBirth:      tr("place_of_birth_ru"),
		IssueDate:         get("issue_date"),
		ExpiryDate:        get("expiry_date"),
		FormerNationality: ComputeFormerNationality(get("former_nationality_ru"), get("place_of_birth_ru"), get("birth_date")),
		Gender:            gender,
		MaritalStatus:     marital,
		PassportType:      passportType,
		IssuedBy:          tr("issued_by_ru"),
		// home_address_ru is PII. It is canonicalised by
		// CleanDoverenostFields (YandexGPT, RU-resident — see PII note in
		// doverenost_clean.go) and transliterated via ICAO for the
		// anketa PDF here. The transliteration step stays local because
		// it is deterministic.
		HomeAddress:           translit.RuToLatICAO(cleanedLookup(cleanedDoverenost, get("home_address_ru"))),
		Phone:                 get("phone"),
		Occupation:            occupation,
		Employer:              tr("employer_ru"),
		EmployerAddress:       tr("employer_address_ru"),
		BeenToJapan:           MapYesNo(get("been_to_japan_ru")),
		PreviousVisits:        tr("previous_visits_ru"),
		CriminalRecord:        MapYesNo(get("criminal_record_ru")),
		MaidenName:            translit.RuToLatICAO(get("maiden_name_ru")),
		NationalityISO:        CountryISO(get("nationality_ru")),
		FormerNationalityText: ComputeFormerNationality(get("former_nationality_ru"), get("place_of_birth_ru"), get("birth_date")),
		GenderRB:              GenderRB(gender),
		MaritalStatusRB:       MaritalRB(marital),
		PassportTypeRB:        PassportTypeRB(passportType),
		ArrivalDateJapan:      strGet(arrival, "date"),
		ArrivalTime:           strGet(arrival, "time"),
		ArrivalAirport:        strGet(arrival, "airport"),
		ArrivalFlight:         strGet(arrival, "flight_number"),
		DepartureDateJapan:    strGet(departure, "date"),
		DepartureTime:         strGet(departure, "time"),
		DepartureAirport:      strGet(departure, "airport"),
		DepartureFlight:       strGet(departure, "flight_number"),
		IntendedStayDays:      stayDays,
	}
}

// AssembleDoverenost builds the doverenost entries. `tourists` are already
// assembled Pass2Tourist records. `payloads[i]` corresponds to tourists[i].
// departureDate is DD.MM.YYYY used for minor-age detection.
//
// cleanedDoverenost is a raw → canonical map produced by
// CleanDoverenostFields (YandexGPT) covering home_address_ru,
// reg_address_ru and internal_issued_by_ru across all tourists in the
// run. A nil / missing entry falls back to the raw value so older call
// sites and unit tests still work.
func AssembleDoverenost(tourists []Pass2Tourist, payloads []map[string]any, cleanedDoverenost map[string]string, departureDate string) []Pass2Dov {
	refs := make([]TouristRef, len(tourists))
	for i, t := range tourists {
		refs[i] = TouristRef{
			ID:         fmt.Sprint(i),
			SurnameCyr: FirstWord(t.NameCyr),
			BirthDate:  t.BirthDate,
			IsMinor:    IsMinorOnDate(t.BirthDate, departureDate),
		}
	}

	out := make([]Pass2Dov, len(tourists))
	for i, t := range tourists {
		minor := refs[i].IsMinor
		payload := payloads[i]
		dov := Pass2Dov{
			NameRu:         TitleCaseRuName(t.NameCyr),
			DOB:            t.BirthDate,
			PassportSeries: strGet(payload, "internal_series"),
			PassportNumber: strGet(payload, "internal_number"),
			IssuedDate:     russianIssuedDate(strGet(payload, "internal_issued_ru")),
			// IssuedBy + RegAddress are PII-adjacent free-text fields.
			// They are canonicalised by CleanDoverenostFields (YandexGPT,
			// RU-resident processing — see PII note in
			// doverenost_clean.go). The assembler simply looks the cleaned
			// string up in the map produced by the orchestrator.
			IssuedBy:   cleanedLookup(cleanedDoverenost, strGet(payload, "internal_issued_by_ru")),
			RegAddress: cleanedLookup(cleanedDoverenost, strGet(payload, "reg_address_ru")),
			IsMinor:    minor,
		}
		if minor {
			parent := FindParent(refs[i], refs)
			if parent != nil {
				pIdx := indexOf(refs, parent.ID)
				pp := payloads[pIdx]
				dov.NameRu = TitleCaseRuName(tourists[pIdx].NameCyr)
				dov.DOB = tourists[pIdx].BirthDate
				dov.PassportSeries = strGet(pp, "internal_series")
				dov.PassportNumber = strGet(pp, "internal_number")
				dov.IssuedDate = russianIssuedDate(strGet(pp, "internal_issued_ru"))
				dov.IssuedBy = cleanedLookup(cleanedDoverenost, strGet(pp, "internal_issued_by_ru"))
				dov.RegAddress = cleanedLookup(cleanedDoverenost, strGet(pp, "reg_address_ru"))
			}
			// Child name in genitive case — title-case surname/first before
			// building the genitive so "ИВАНОВ Петя" stays proper-cased.
			surname := TitleCaseRuName(FirstWord(t.NameCyr))
			firstName := TitleCaseRuName(strings.TrimSpace(strings.TrimPrefix(t.NameCyr, FirstWord(t.NameCyr))))
			childName, _ := GenitiveFullName(surname, firstName, t.Gender)
			dov.ChildNameRu = childName
			if t.Gender == "Male" {
				dov.ChildGender = "сына"
			} else {
				dov.ChildGender = "дочери"
			}
		}
		out[i] = dov
	}
	return out
}

// AssemblePass2 is the top-level entry point called by generate.go.
// It composes the full pass2.json structure from already-fetched inputs.
// todayDate: DD.MM.YYYY for the date_of_application.
//
// cleanedDoverenost is the raw → canonical map of free-text Russian
// addresses + issuing-authority strings produced once per run by
// CleanDoverenostFields (YandexGPT). Pass nil if you do not want
// canonicalisation — the assembler falls back to the raw values.
func AssemblePass2(
	payloads []map[string]any,
	translations []map[string]string,
	cleanedDoverenost map[string]string,
	flights []map[string]any,
	programme []ProgrammeDay,
	hotels []HotelBrief,
	todayDate string,
) Pass2 {
	tourists := make([]Pass2Tourist, len(payloads))
	for i := range payloads {
		var tr map[string]string
		if i < len(translations) {
			tr = translations[i]
		}
		var fl map[string]any
		if i < len(flights) {
			fl = flights[i]
		}
		tourists[i] = AssembleTourist(payloads[i], tr, cleanedDoverenost, fl)
	}

	var firstHotel Pass2Hotel
	if len(hotels) > 0 {
		firstHotel = Pass2Hotel{Name: hotels[0].Name, Address: hotels[0].Address, Phone: hotels[0].Phone}
	}

	// Arrival/departure block uses first tourist with populated flight data
	var arr, dep Pass2ArrDep
	for _, t := range tourists {
		if t.ArrivalFlight != "" {
			arr = Pass2ArrDep{Date: t.ArrivalDateJapan, Airport: t.ArrivalAirport, Flight: t.ArrivalFlight, Time: t.ArrivalTime}
			break
		}
	}
	for _, t := range tourists {
		if t.DepartureFlight != "" {
			dep = Pass2ArrDep{Date: t.DepartureDateJapan, Airport: t.DepartureAirport, Flight: t.DepartureFlight, Time: t.DepartureTime}
			break
		}
	}

	doverenost := AssembleDoverenost(tourists, payloads, cleanedDoverenost, dep.Date)

	var cyrNames []string
	for _, t := range tourists {
		cyrNames = append(cyrNames, t.NameCyr)
	}

	return Pass2{
		DocumentDate: todayDate,
		Tourists:     tourists,
		Programme:    programme,
		Anketa: Pass2Anketa{
			CriminalRB:        "1",
			Email:             "tour@fujitravel.ru",
			DateOfApplication: todayDate,
			FirstHotelName:    firstHotel.Name,
			FirstHotelAddress: firstHotel.Address,
			FirstHotelPhone:   firstHotel.Phone,
		},
		Doverenost: doverenost,
		VCRequest: Pass2VC{
			Applicants:          cyrNames,
			Count:               len(cyrNames),
			ServiceFeePerPerson: 970,
			ServiceFeeTotal:     970 * len(cyrNames),
		},
		InnaDoc: Pass2Inna{
			SubmissionDate: arr.Date,
			ApplicantsRu:   cyrNames,
		},
		FirstHotel:       firstHotel,
		Arrival:          arr,
		Departure:        dep,
		IntendedStayDays: 0, // per-tourist field is authoritative
		Email: Pass2Email{
			To:      "ta_japan_moscow@vfsglobal.com",
			Subject: todayDate + " FujiTravel",
			Body:    strings.Join(cyrNames, "\n"),
		},
	}
}

// ---------- helpers ----------

func subMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]any); ok {
			return sub
		}
	}
	return nil
}

func strGet(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

var russianMonths = []string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

// russianIssuedDate formats DD.MM.YYYY → «DD» Month YYYY
func russianIssuedDate(s string) string {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return s
	}
	return fmt.Sprintf("«%02d» %s %d", t.Day(), russianMonths[int(t.Month())], t.Year())
}

func indexOf(refs []TouristRef, id string) int {
	for i, r := range refs {
		if r.ID == id {
			return i
		}
	}
	return 0
}
