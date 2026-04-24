// Canonical list of Japanese airports used by the flight-data form dropdown.
// The `value` is the exact string written into tourists.flight_data.*.airport —
// it must match the canonical names in backend/internal/ai/ticket_parser.go so
// auto-parsed tickets land on an existing dropdown option.
export const JAPANESE_AIRPORTS = [
  { value: 'Narita International Airport',        label: 'Narita International Airport (NRT) — Tokyo' },
  { value: 'Haneda Airport',                      label: 'Haneda Airport (HND) — Tokyo' },
  { value: 'Kansai International Airport',        label: 'Kansai International Airport (KIX) — Osaka' },
  { value: 'Chubu Centrair International Airport',label: 'Chubu Centrair International Airport (NGO) — Nagoya' },
  { value: 'Fukuoka Airport',                     label: 'Fukuoka Airport (FUK)' },
  { value: 'New Chitose Airport',                 label: 'New Chitose Airport (CTS) — Sapporo' },
  { value: 'Naha Airport',                        label: 'Naha Airport (OKA) — Okinawa' },
];

export const JAPANESE_AIRPORT_VALUES = new Set(JAPANESE_AIRPORTS.map((a) => a.value));
