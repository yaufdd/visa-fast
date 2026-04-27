package ai

// Pass2Tourist is one tourist row in the final pass2.json.
// JSON field names and shape must exactly match what docgen/generate.py
// expects (do not rename without updating the Python side).
type Pass2Tourist struct {
	NameLat               string `json:"name_lat"`
	NameCyr               string `json:"name_cyr"`
	PassportNumber        string `json:"passport_number"`
	BirthDate             string `json:"birth_date"`
	Nationality           string `json:"nationality"`
	PlaceOfBirth          string `json:"place_of_birth"`
	IssueDate             string `json:"issue_date"`
	ExpiryDate            string `json:"expiry_date"`
	FormerNationality     string `json:"former_nationality"`
	Gender                string `json:"gender"`
	MaritalStatus         string `json:"marital_status"`
	PassportType          string `json:"passport_type"`
	IssuedBy              string `json:"issued_by"`
	HomeAddress           string `json:"home_address"`
	Phone                 string `json:"phone"`
	Occupation            string `json:"occupation"`
	Employer              string `json:"employer"`
	EmployerAddress       string `json:"employer_address"`
	EmployerPhone         string `json:"employer_phone"`
	BeenToJapan           string `json:"been_to_japan"`
	PreviousVisits        string `json:"previous_visits"`
	CriminalRecord        string `json:"criminal_record"`
	MaidenName            string `json:"maiden_name"`
	NationalityISO        string `json:"nationality_iso"`
	FormerNationalityText string `json:"former_nationality_text"`
	GenderRB              string `json:"gender_rb"`
	MaritalStatusRB       string `json:"marital_status_rb"`
	PassportTypeRB        string `json:"passport_type_rb"`
	ArrivalDateJapan      string `json:"arrival_date_japan"`
	ArrivalTime           string `json:"arrival_time"`
	ArrivalAirport        string `json:"arrival_airport"`
	ArrivalFlight         string `json:"arrival_flight"`
	DepartureDateJapan    string `json:"departure_date_japan"`
	DepartureTime         string `json:"departure_time"`
	DepartureAirport      string `json:"departure_airport"`
	DepartureFlight       string `json:"departure_flight"`
	IntendedStayDays      int    `json:"intended_stay_days"`
}

// Pass2 is the root document passed to docgen/generate.py.
type Pass2 struct {
	DocumentDate     string         `json:"document_date"`
	Tourists         []Pass2Tourist `json:"tourists"`
	Programme        []ProgrammeDay `json:"programme"`
	Anketa           Pass2Anketa    `json:"anketa"`
	Doverenost       []Pass2Dov     `json:"doverenost"`
	VCRequest        Pass2VC        `json:"vc_request"`
	InnaDoc          Pass2Inna      `json:"inna_doc"`
	FirstHotel       Pass2Hotel     `json:"first_hotel"`
	Arrival          Pass2ArrDep    `json:"arrival"`
	Departure        Pass2ArrDep    `json:"departure"`
	IntendedStayDays int            `json:"intended_stay_days"`
	Email            Pass2Email     `json:"email"`
}

type Pass2Anketa struct {
	CriminalRB        string `json:"criminal_rb"`
	Email             string `json:"email"`
	DateOfApplication string `json:"date_of_application"`
	FirstHotelName    string `json:"first_hotel_name"`
	FirstHotelAddress string `json:"first_hotel_address"`
	FirstHotelPhone   string `json:"first_hotel_phone"`
}

type Pass2Dov struct {
	NameRu         string `json:"name_ru"`
	DOB            string `json:"dob"`
	PassportSeries string `json:"passport_series"`
	PassportNumber string `json:"passport_number"`
	IssuedDate     string `json:"issued_date"`
	IssuedBy       string `json:"issued_by"`
	RegAddress     string `json:"reg_address"`
	IsMinor        bool   `json:"is_minor"`
	ChildNameRu    string `json:"child_name_ru"`
	ChildGender    string `json:"child_gender"`
}

type Pass2VC struct {
	Applicants          []string `json:"applicants"`
	Count               int      `json:"count"`
	ServiceFeePerPerson int      `json:"service_fee_per_person"`
	ServiceFeeTotal     int      `json:"service_fee_total"`
}

type Pass2Inna struct {
	SubmissionDate string   `json:"submission_date"`
	ApplicantsRu   []string `json:"applicants_ru"`
}

type Pass2Hotel struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

type Pass2ArrDep struct {
	Date    string `json:"date"`
	Airport string `json:"airport"`
	Flight  string `json:"flight"`
	Time    string `json:"time"`
}

type Pass2Email struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
