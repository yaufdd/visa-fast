// Canonical Russian city names for the hotels catalog. Input from users may
// arrive in English (legacy data + voucher auto-creation), Russian, or with
// typos / diacritics — normalizeCity() maps every known variant to its
// canonical Russian form.
//
// CANONICAL_CITIES is also exported for use as the <datalist> suggestion list.

export const CANONICAL_CITIES = [
  'Токио',
  'Киото',
  'Осака',
  'Хаконэ',
  'Идзу',
  'Окинава',
  'Наха',
  'Нара',
  'Канадзава',
  'Нагоя',
  'Хиросима',
  'Никко',
  'Иокогама',
  'Камакура',
  'Саппоро',
  'Фукуока',
  'Такаяма',
  'Мацумото',
  'Кобе',
  'Миядзима',
  'Сиракава-го',
  'Гора Фудзи',
  'Нагано',
  'Сэндай',
];

// Normalize a city string for map lookup: lowercase + strip whitespace,
// hyphens, dots, commas, underscores. Preserves Cyrillic characters.
function normKey(s) {
  return String(s || '')
    .trim()
    .toLowerCase()
    .replace(/[\s\-_.,/]+/g, '');
}

// Source of truth: every variant → canonical Russian.
// Keys will be passed through normKey() so you don't have to pre-normalize them.
const RAW_VARIANTS = {
  // Tokyo
  'Tokyo': 'Токио', 'Tōkyō': 'Токио', 'Tokio': 'Токио', 'Токио': 'Токио', 'токио': 'Токио',
  // Kyoto
  'Kyoto': 'Киото', 'Kyōto': 'Киото', 'Киото': 'Киото', 'Kioto': 'Киото',
  // Osaka
  'Osaka': 'Осака', 'Ōsaka': 'Осака', 'Осака': 'Осака', 'Asaka': 'Осака',
  // Hakone
  'Hakone': 'Хаконэ', 'Хаконэ': 'Хаконэ', 'Хаконе': 'Хаконэ',
  // Izu
  'Izu': 'Идзу', 'Идзу': 'Идзу', 'Izu-hanto': 'Идзу',
  // Okinawa / Naha
  'Okinawa': 'Окинава', 'Окинава': 'Окинава',
  'Naha': 'Наха', 'Наха': 'Наха',
  // Nara
  'Nara': 'Нара', 'Нара': 'Нара',
  // Kanazawa
  'Kanazawa': 'Канадзава', 'Канадзава': 'Канадзава', 'Каназава': 'Канадзава',
  // Nagoya
  'Nagoya': 'Нагоя', 'Нагоя': 'Нагоя',
  // Hiroshima
  'Hiroshima': 'Хиросима', 'Хиросима': 'Хиросима',
  // Nikko
  'Nikko': 'Никко', 'Nikkō': 'Никко', 'Никко': 'Никко',
  // Yokohama
  'Yokohama': 'Иокогама', 'Иокогама': 'Иокогама', 'Йокогама': 'Иокогама',
  // Kamakura
  'Kamakura': 'Камакура', 'Камакура': 'Камакура',
  // Sapporo
  'Sapporo': 'Саппоро', 'Саппоро': 'Саппоро', 'Сапоро': 'Саппоро',
  // Fukuoka
  'Fukuoka': 'Фукуока', 'Фукуока': 'Фукуока',
  // Takayama
  'Takayama': 'Такаяма', 'Такаяма': 'Такаяма',
  // Matsumoto
  'Matsumoto': 'Мацумото', 'Мацумото': 'Мацумото',
  // Kobe
  'Kobe': 'Кобе', 'Kōbe': 'Кобе', 'Кобе': 'Кобе',
  // Miyajima
  'Miyajima': 'Миядзима', 'Миядзима': 'Миядзима',
  // Shirakawa-go
  'Shirakawa-go': 'Сиракава-го', 'Shirakawago': 'Сиракава-го', 'Shirakawa': 'Сиракава-го',
  'Сиракава-го': 'Сиракава-го', 'Сиракаваго': 'Сиракава-го', 'Сиракава': 'Сиракава-го',
  // Mt Fuji
  'Mt Fuji': 'Гора Фудзи', 'Mount Fuji': 'Гора Фудзи', 'Fuji': 'Гора Фудзи',
  'Гора Фудзи': 'Гора Фудзи', 'Фудзи': 'Гора Фудзи',
  // Nagano
  'Nagano': 'Нагано', 'Нагано': 'Нагано',
  // Sendai
  'Sendai': 'Сэндай', 'Сэндай': 'Сэндай', 'Сендай': 'Сэндай',
};

const NORMALIZED_VARIANTS = Object.fromEntries(
  Object.entries(RAW_VARIANTS).map(([k, v]) => [normKey(k), v]),
);

// normalizeCity maps any known variant to its canonical Russian spelling.
// Unknown strings are returned with Title Case applied to the first letter so
// "sendai-city" stays legible ("Sendai-city") instead of being lowercased.
export function normalizeCity(raw) {
  const s = (raw || '').trim();
  if (!s) return '';
  const key = normKey(s);
  if (NORMALIZED_VARIANTS[key]) return NORMALIZED_VARIANTS[key];
  return s.charAt(0).toLocaleUpperCase('ru-RU') + s.slice(1);
}
