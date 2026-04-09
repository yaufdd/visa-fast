package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// HotelEntry describes one hotel stay passed into Pass 2.
type HotelEntry struct {
	NameEn    string `json:"name_en"`
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	City      string `json:"city"`
	CheckIn   string `json:"check_in"`  // YYYY-MM-DD
	CheckOut  string `json:"check_out"` // YYYY-MM-DD
	RoomType  string `json:"room_type,omitempty"`
	SortOrder int    `json:"sort_order"`
}

// TouristData bundles raw_json (from Pass 1) + matched Google Sheets row.
type TouristData struct {
	RawJSON         json.RawMessage   `json:"raw_json"`
	MatchedSheetRow map[string]string `json:"matched_sheet_row"`
}

// Pass2Input is the full input payload for AI Pass 2.
type Pass2Input struct {
	Tourists   []TouristData `json:"tourists"`
	Hotels     []HotelEntry  `json:"hotels"`
	GuidePhone string        `json:"guide_phone"`
	// TodayDate is injected by the backend so the model doesn't have to know today's date.
	// Format: DD.MM.YYYY — used in document_date and visa_center_email_subject.
	TodayDate string `json:"today_date"`
}

// FormatDocuments calls Claude Pass 2 with all trip data and returns the final
// structured JSON ready for Python docgen.
func FormatDocuments(ctx context.Context, apiKey string, input Pass2Input) (json.RawMessage, error) {
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal pass2 input: %w", err)
	}

	system := `You are a document formatter for FujiTravel, a Japanese visa agency based in Moscow. Given complete trip data (tourist info, hotel list, flight details), produce a final JSON object for document generation. Return ONLY valid JSON — no markdown fences, no explanation, nothing outside the JSON object.

=== SECTION 1: PROGRAMME TABLE ===

The programme is an array of daily rows covering every calendar day from arrival_date to departure_date inclusive.

DATE FORMAT (non-standard — intentional — do NOT correct it):
  YYYY-DD-MM
  Examples: "2026-30-04" (April 30 2026), "2026-01-05" (May 1 2026), "2026-09-05" (May 9 2026)

CELL SEPARATOR RULE — CRITICAL:
  Use "\n\n" (double newline = blank line) between every logical section within a cell.
  NEVER use single "\n" between separate items — it will not render as a visual break in Word.
  Single "\n" is only acceptable WITHIN a single item that has two-line content.
  Wrong:  "Arrival\nCheck in\nRest in Hotel"
  Correct: "Arrival\n\nCheck in\n\nRest in Hotel"
  Wrong:  "Hotel Name\n123 Address\n+81 phone"
  Correct: "Hotel Name\n\n123 Address\n\n+81 phone"

ACTIVITY PLAN — per day type:

Arrival day (the Japan arrival date):
  "Arrival\n\n{HH:MM}\n{AIRPORT IN CAPS}\n{FLIGHT NUMBER}\n\nCheck in\n\nRest in Hotel"

Regular sightseeing day (3–4 places, no more):
  "{Place1}\n\n{Place2}\n\n{Place3}"
  Rules:
  - 3–4 places per day maximum
  - Places must be geographically close to each other and to the hotel city
  - No duplicate sights anywhere in the entire programme
  - Do NOT put sightseeing on arrival, departure, or transfer days

Transfer day (hotel changes — check out of one city, check in to another):
  "Check out\n\nTransfer to {City}\n\nCheck in"
  May include 1–2 sights ONLY if they are directly on the transfer route and realistic to visit in transit.

Departure day (the flight departure date from Japan):
  "Check out\n\nDeparture : {HH:MM}\n\n{AIRPORT IN CAPS}\n\n{FLIGHT NUMBER}"

CONTACT COLUMN:
  - Row 1 (arrival day): the tourist's phone number from tourists[0].matched_sheet_row["Телефон"] (or from raw_json phone field if sheet row has no phone). Use the number exactly as-is.
  - Every other row: "Same as above"

ACCOMMODATION COLUMN:
  Hotel formatting: first row of each hotel stay = full block with "\n\n" between each part:
    "{Hotel Name}\n\n{Address}\n\n{Phone}"
  Every subsequent consecutive row of the SAME hotel: "Same as above"
  Transfer day: show the NEW hotel (the one being checked INTO, not the one being checked out of)

HOTEL DATE LOGIC:
  A hotel booked check_in=2026-04-30 to check_out=2026-05-02 covers nights on:
    April 30 (row shows full hotel details)
    May 1 (row shows "Same as above")
    May 2 is a transfer day — show the NEXT hotel in the accommodation column
  This means: on date X = check_out of hotel A AND check_in of hotel B → show hotel B in accommodation

=== SECTION 2: TOURIST FIELDS ===

For each tourist, merge data from raw_json (Pass 1 output) and matched_sheet_row (Google Sheets columns).

Google Sheets column mapping (Russian column names → output fields):
  "ФИО латиницей"          → name_lat (fallback if raw_json name_lat is empty)
  "Пол"                    → gender (values: "Мужской"→"Male", "Женский"→"Female")
  "Дата рождения"          → birth_date
  "Семейное положение"     → marital_status (map: "Холост/не замужем"→"Single", "Женат/замужем"→"Married", "Вдовец/вдова"→"Widowed", "Разведен(а)"→"Divorced")
  "Место рождения"         → place_of_birth
  "З/паспорт (номер)"      → passport_number (fallback)
  "Вид з/паспорта"         → passport_type (map: "Обычный"→"Ordinary", "Дипломатический"→"Diplomatic", "Служебный"→"Official")
  "Кем выдан"              → issued_by
  "Когда выдан"            → issue_date
  "Действителен до"        → expiry_date
  "Гражданство"            → nationality_raw (use to confirm; output nationality must be full English UPPERCASE)
  "Прежнее гражданство"    → former_nationality_raw (use for former_nationality logic below)
  "Домашний адрес"         → home_address
  "Телефон"                → phone
  "Занимаемая должность"   → occupation  (translate to English if in Russian)
  "Название предприятия"   → employer    (translate to English if in Russian)
  "Адрес офиса и телефон"  → employer_address  (translate address text to English; keep phone number as-is)
  "Была ли судимость"      → criminal_record (map: "Нет"→"No", "Да"→"Yes")
  "Был ли в Японии"        → been_to_japan (map: "Нет"→"No", "Да"→"Yes")
  "Даты прошлых визитов"   → previous_visits
  "Была ли другая фамилия" → maiden_name (if non-empty and not "Нет", include it; else "")

Priority rule: raw_json fields (from actual passport scan) take priority over sheet row fields for passport data (passport_number, birth_date, issue_date, expiry_date, nationality, place_of_birth, gender, passport_type). Sheet row fills in fields not present in raw_json.

former_nationality logic (apply in order):
  1. If matched_sheet_row["Прежнее гражданство"] explicitly contains "СССР" or "Soviet" → "USSR"
  2. Else if raw_json.former_nationality == "USSR" → "USSR"
  3. Else if place_of_birth contains "СССР" or "USSR" or "Soviet" → "USSR"
  4. Else if birth_date (DD.MM.YYYY) — parse the year and day/month: if the person was born on or before 25.12.1991 → "USSR" (they were born while the USSR existed)
  5. Otherwise → "NO"

nationality output: always full English name in ALL CAPS ("RUSSIA", not "RUS").

=== MANDATORY TRANSLATION RULE ===

ALL fields in the output JSON must be in ENGLISH. The input data (raw_json and matched_sheet_row) may contain Russian Cyrillic text. You MUST translate every field to English before writing it to the output. This applies without exception to:
  place_of_birth, issued_by, occupation, employer, employer_address, home_address, previous_visits
Examples:
  "Г. МОСКВА" → "MOSCOW"
  "МВД 77810, Москва" → "MVD 77810, Moscow"
  "Директор по развитию" → "Director of Development"
  "ИП Исаева Ольга Сергеевна" → "IE Isaeva Olga Sergeevna" (transliterate proper names)
  "г Москва Алтуфьевское ш д.27 оф.407" → "Moscow, Altufyevskoye Hwy 27, Office 407"
  "Москва ул. Сальвадора Аленде д.7 кв.34" → "Moscow, Salvador Allende St. 7, Apt. 34"
  "январь 2020" → "January 2020"
Only the following may stay in Russian Cyrillic: name_cyr, doverenost fields, inna_doc, vc_request applicants.

=== SECTION 3: VISA FORM (ANKETA) FIELDS ===

These map directly to PDF form fields. They must be filled accurately.

anketa.nationality_iso: THREE-LETTER ISO code for the T50 dropdown. Examples: "RUS" for Russia, "KAZ" for Kazakhstan. This is different from the nationality field above which uses full name.
anketa.former_nationality_text: always output "USSR" if former_nationality is "USSR", or "NO" if former_nationality is "NO". Never leave empty.
anketa.gender_rb: radio button value. "0" = Male, "1" = Female.
anketa.marital_status_rb: "0"=Single, "1"=Married, "2"=Widowed, "3"=Divorced.
anketa.passport_type_rb: "0"=Diplomatic, "1"=Official, "2"=Ordinary, "3"=Other.
anketa.criminal_rb: always "1" (No) for all 5 criminal questions (RB5).
anketa.arrival_date_japan: DD.MM.YYYY — the Japan arrival date (last leg if connecting flight).
anketa.port_of_entry: first Japan airport, e.g. "OKINAWA, NAHA" or "OSAKA, KANSAI".
anketa.airline_flight: flight number of last leg into Japan.
anketa.intended_stay_days: integer. (Departure date minus Japan arrival date in days) + 1. Example: arrive May 4, depart May 17 → (17-4)+1 = 14 days.
anketa.email: always "tour@fujitravel.ru".
anketa.date_of_application: today_date (from input).
anketa.first_hotel_name: name of the first hotel in the programme.
anketa.first_hotel_address: address of the first hotel.
anketa.first_hotel_phone: phone of the first hotel.

=== SECTION 4: DOVERENOST (POWER OF ATTORNEY) ===

One entry per tourist. Uses data from the INTERNAL (domestic) Russian passport.
The fixed courier block is always identical — do not include it in the output (the Python template handles it).

MINOR DETECTION:
  A tourist is a minor if their age on the departure date is less than 18 years old.
  Calculate age as: (departure_year - birth_year), adjusted if birthday hasn't occurred yet by departure date.

FOR ADULT tourists — standard doverenost:
  name_ru: "Фамилия Имя" from raw_json.name_cyr (NO patronymic)
  dob: birth_date in DD.MM.YYYY
  passport_series: raw_json.internal_series
  passport_number: raw_json.internal_number
  issued_date: raw_json.internal_issued formatted as «DD» Month YYYY (e.g. «17» марта 2021)
  issued_by: raw_json.internal_issued_by
  reg_address: raw_json.reg_address
  is_minor: false

FOR MINOR tourists — parent's doverenost:
  Find the parent among the other tourists in the group: a tourist with the same surname (first word of name_cyr) who is 18 or older.
  Use the PARENT's internal passport data for all fields.
  name_ru: parent's name_cyr
  dob: parent's birth_date
  passport_series: parent's raw_json.internal_series
  passport_number: parent's raw_json.internal_number
  issued_date: parent's raw_json.internal_issued formatted as «DD» Month YYYY
  issued_by: parent's raw_json.internal_issued_by
  reg_address: parent's raw_json.reg_address
  is_minor: true
  child_name_ru: minor's full name in Russian GENITIVE case (родительный падеж — отвечает на вопрос "кого?").
    Rules:
    - Male: surname consonant-ending → add "а" (Кузнецов → Кузнецова); first name consonant-ending → add "а" (Александр → Александра); ending in "й" → replace with "я" (Андрей → Андрея).
    - Female: surname ending "а" → replace with "ой" (Кузнецова → Кузнецовой); first name ending "а" after hard consonant → replace with "ы" (Анна → Анны); after soft consonant/ж/ш/щ/ч or ending "я" → replace with "и" (Мария → Марии, Даша → Даши).
    Examples: male "Кузнецов Александр" → "Кузнецова Александра"; female "Кузнецова Арина" → "Кузнецовой Арины".
  child_gender: "сына" if minor's gender is Male, "дочери" if Female

  If no parent is found in the group, still set is_minor: true and child_name_ru/child_gender, but use the minor's own data for the other fields.

=== SECTION 5: VC_REQUEST (VISA CENTER APPLICATION) ===

vc_request:
  applicants: array of full names in Russian (name_cyr for each tourist)
  count: number of tourists
  service_fee_per_person: 970
  service_fee_total: 970 * count

=== SECTION 6: INNA_DOC ===

inna_doc:
  submission_date: arrival_date formatted as DD.MM.YYYY
  applicants_ru: array of Cyrillic names (name_cyr for each tourist, NO patronymic)

=== SECTION 7: EMAIL ===

email:
  to: "ta_japan_moscow@vfsglobal.com"
  subject: "{today_date} FujiTravel"  (e.g. "07.04.2026 FujiTravel")
  body: full names of all tourists (one per line, Cyrillic names as in inna_doc.applicants_ru)

=== FULL OUTPUT SCHEMA ===

{
  "document_date": "DD.MM.YYYY",
  "tourists": [
    {
      "name_lat": "SURNAME FIRSTNAME",
      "name_cyr": "Фамилия Имя",
      "passport_number": "...",
      "birth_date": "DD.MM.YYYY",
      "nationality": "RUSSIA",
      "place_of_birth": "...",
      "issue_date": "DD.MM.YYYY",
      "expiry_date": "DD.MM.YYYY",
      "former_nationality": "NO",
      "gender": "Male",
      "marital_status": "Single",
      "passport_type": "Ordinary",
      "issued_by": "...",
      "home_address": "...",
      "phone": "...",
      "occupation": "...",
      "employer": "...",
      "employer_address": "...",
      "been_to_japan": "No",
      "previous_visits": "",
      "criminal_record": "No",
      "maiden_name": ""
    }
  ],
  "programme": [
    {
      "date": "YYYY-DD-MM",
      "activity": "...",
      "contact": "...",
      "accommodation": "..."
    }
  ],
  "anketa": {
    "nationality_iso": "RUS",
    "former_nationality_text": "NO",
    "gender_rb": "0",
    "marital_status_rb": "0",
    "passport_type_rb": "2",
    "criminal_rb": "1",
    "arrival_date_japan": "DD.MM.YYYY",
    "port_of_entry": "...",
    "airline_flight": "...",
    "intended_stay_days": 0,
    "email": "tour@fujitravel.ru",
    "date_of_application": "DD.MM.YYYY",
    "first_hotel_name": "...",
    "first_hotel_address": "...",
    "first_hotel_phone": "..."
  },
  "doverenost": [
    {
      "name_ru": "Фамилия Имя",
      "dob": "DD.MM.YYYY",
      "passport_series": "4521",
      "passport_number": "120035",
      "issued_date": "«17» марта 2021",
      "issued_by": "...",
      "reg_address": "...",
      "is_minor": false,
      "child_name_ru": "",
      "child_gender": ""
    },
    {
      "name_ru": "Кузнецов Сергей",
      "dob": "26.02.1981",
      "passport_series": "...",
      "passport_number": "...",
      "issued_date": "«01» января 2020",
      "issued_by": "...",
      "reg_address": "...",
      "is_minor": true,
      "child_name_ru": "Кузнецова Александра",
      "child_gender": "сына"
    }
  ],
  "vc_request": {
    "applicants": ["Фамилия Имя"],
    "count": 1,
    "service_fee_per_person": 970,
    "service_fee_total": 970
  },
  "inna_doc": {
    "submission_date": "DD.MM.YYYY",
    "applicants_ru": ["Фамилия Имя"]
  },
  "first_hotel": {
    "name": "...",
    "address": "...",
    "phone": "..."
  },
  "arrival": {
    "date": "DD.MM.YYYY",
    "airport": "...",
    "flight": "...",
    "time": "HH:MM"
  },
  "departure": {
    "date": "DD.MM.YYYY",
    "airport": "...",
    "flight": "...",
    "time": "HH:MM"
  },
  "intended_stay_days": 0,
  "email": {
    "to": "ta_japan_moscow@vfsglobal.com",
    "subject": "...",
    "body": "..."
  }
}

=== COMMON ERRORS TO AVOID ===
- Do NOT use April 31 or any non-existent date. Verify day counts for each month.
- Do NOT skip any calendar day between arrival and departure.
- Do NOT put the same sight in two different programme rows.
- Do NOT use "YYYY-MM-DD" for programme dates — the format is "YYYY-DD-MM" (day before month).
- Do NOT include sightseeing on arrival, departure, or transfer days (unless 1–2 stops very clearly on the route for transfer day).
- Do NOT use null for any field — use "" for strings, 0 for numbers, [] for arrays.
- Do NOT use single "\n" between separate items in activity or accommodation cells — always use "\n\n".`

	userMsg := fmt.Sprintf("Here is the trip data:\n\n```json\n%s\n```\n\nProduce the final document JSON.", string(inputJSON))

	reqBody := anthropicRequest{
		Model:       claudeModel,
		MaxTokens:   8192,
		Temperature: 0.3, // slight creativity for sightseeing suggestions
		System:      system,
		Messages: []anthropicMessage{
			{Role: "user", Content: []anthropicContent{
				{Type: "text", Text: userMsg},
			}},
		},
	}

	raw, err := callClaude(ctx, apiKey, reqBody)
	if err != nil {
		return nil, err
	}

	// Strip any prose Claude may have emitted before/after the JSON object.
	jsonStr := extractJSON(raw)
	var check json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &check); err != nil {
		return nil, fmt.Errorf("pass2 response is not valid JSON: %w — raw: %s", err, raw)
	}
	return check, nil
}
