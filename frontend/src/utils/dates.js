// Date format helpers.
// Backend expects `DD.MM.YYYY` for all date fields, while <input type="date">
// uses ISO `YYYY-MM-DD`. Convert only for the input's value/onChange; keep
// state in backend format.

export function dmyToIso(dmy) {
  if (!dmy) return '';
  const m = dmy.match(/^(\d{2})\.(\d{2})\.(\d{4})$/);
  return m ? `${m[3]}-${m[2]}-${m[1]}` : '';
}

export function isoToDmy(iso) {
  if (!iso) return '';
  const m = iso.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  return m ? `${m[3]}.${m[2]}.${m[1]}` : '';
}
