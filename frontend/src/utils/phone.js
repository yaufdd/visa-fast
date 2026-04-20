// Russian phone number helpers.
// `phoneOnInput` runs on every keystroke — converts leading 8 to +7
// immediately and formats progressively as the user types.
// `normalizePhone` runs on blur — full canonical "+7 (XXX) XXX-XX-XX".

// phoneOnInput:
//  ""          → ""
//  "8"         → "+7"
//  "89"        → "+7 (9"
//  "8999"      → "+7 (999) "
//  "89991234567" → "+7 (999) 123-45-67"
//  "+7 999..." → preserved (already prefixed)
export function phoneOnInput(raw) {
  const digits = (raw || '').replace(/\D/g, '');
  if (digits.length === 0) return '';
  // Leading 8 or 7 → Russian country code.
  let d = digits;
  if (d.startsWith('8') || d.startsWith('7')) {
    d = d.slice(1);
  }
  // Cap at 10 digits (the national number). Extra digits ignored.
  d = d.slice(0, 10);
  if (d.length === 0) return '+7';
  if (d.length <= 3) return `+7 (${d}`;
  if (d.length <= 6) return `+7 (${d.slice(0, 3)}) ${d.slice(3)}`;
  if (d.length <= 8) return `+7 (${d.slice(0, 3)}) ${d.slice(3, 6)}-${d.slice(6)}`;
  return `+7 (${d.slice(0, 3)}) ${d.slice(3, 6)}-${d.slice(6, 8)}-${d.slice(8, 10)}`;
}

// normalizePhone: full canonical form on blur.
export function normalizePhone(raw) {
  const digits = (raw || '').replace(/\D/g, '');
  if (digits.length === 0) return '';
  let d = digits;
  if (d.startsWith('8') || d.startsWith('7')) d = d.slice(1);
  if (d.length !== 10) return raw; // incomplete — don't mangle
  return `+7 (${d.slice(0, 3)}) ${d.slice(3, 6)}-${d.slice(6, 8)}-${d.slice(8, 10)}`;
}
