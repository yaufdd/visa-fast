// Russian phone number normalizer.
// Strips non-digits, rewrites leading 8/bare-10-digit numbers into +7,
// and formats as `+7 (XXX) XXX-XX-XX`. If the digits don't match a full
// Russian number yet (user still typing), returns the raw input unchanged
// so we don't fight the user on keystrokes.

export function normalizePhone(raw) {
  const digits = (raw || '').replace(/\D/g, '');
  if (digits.length === 0) return '';
  let normalized = digits;
  if (digits.length === 11 && digits.startsWith('8')) {
    normalized = '7' + digits.slice(1);
  } else if (digits.length === 10) {
    normalized = '7' + digits;
  } else if (digits.length === 11 && digits.startsWith('7')) {
    normalized = digits;
  } else {
    return raw; // user still typing — don't reformat
  }
  const m = normalized.match(/^7(\d{3})(\d{3})(\d{2})(\d{2})$/);
  if (!m) return '+' + normalized;
  return `+7 (${m[1]}) ${m[2]}-${m[3]}-${m[4]}`;
}
