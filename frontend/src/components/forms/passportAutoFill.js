// passportAutoFill.js — pure helpers for mapping a /parse-passport response
// onto the public-form payload. Extracted from SubmissionForm.jsx so the
// wizard step components can reuse it without duplicating the merge logic.
//
// The "only fill empty fields" policy is the load-bearing behaviour: a
// tourist who already typed something wins over the OCR result.

import { ruToLatICAO } from '../../utils/translit';
import { isoToDmy } from '../../utils/dates';

// Map a passport-parser response (ai.PassportFields, JSON-decoded) to the
// form payload shape.  `type` selects between the internal (general-civil)
// field set and the foreign (travel) field set.
//
// Returns { mapped: {field: value}, filled: ["field", ...] }.
export function mapPassportFieldsToPayload(fields, payload, type = 'internal') {
  const mapped = {};
  const filled = [];
  const empty = (name) => !String(payload?.[name] ?? '').trim();
  const set = (name, value) => {
    const v = String(value ?? '').trim();
    if (!v) return;
    if (empty(name)) {
      mapped[name] = v;
      filled.push(name);
    }
  };

  // ── Name (shared) ──
  const nameParts = [fields.last_name, fields.first_name, fields.patronymic]
    .map((s) => String(s ?? '').trim())
    .filter(Boolean);
  if (nameParts.length > 0) {
    const cyr = nameParts.join(' ');
    if (empty('name_cyr')) {
      mapped.name_cyr = cyr;
      filled.push('name_cyr');
      if (empty('name_lat')) {
        mapped.name_lat = ruToLatICAO(cyr);
        filled.push('name_lat');
      }
    }
  }
  if (type === 'foreign' && fields.name_latin) {
    const lat = String(fields.name_latin).trim().toUpperCase();
    if (lat && empty('name_lat')) {
      mapped.name_lat = lat;
      if (!filled.includes('name_lat')) filled.push('name_lat');
    }
  }

  // ── Gender (shared) ──
  if (fields.gender === 'МУЖ') set('gender_ru', 'Мужской');
  else if (fields.gender === 'ЖЕН') set('gender_ru', 'Женский');

  // ── Birth date / place of birth (shared) ──
  if (fields.birth_date) set('birth_date', isoToDmy(fields.birth_date));
  if (fields.place_of_birth) set('place_of_birth_ru', fields.place_of_birth);

  if (type === 'foreign') {
    if (fields.number) {
      const num = String(fields.number).replace(/\s+/g, '');
      if (num.length === 9) set('passport_number', num);
    }
    if (fields.issue_date) set('issue_date', isoToDmy(fields.issue_date));
    if (fields.expiry_date) set('expiry_date', isoToDmy(fields.expiry_date));
    if (fields.issuing_authority) set('issued_by_ru', fields.issuing_authority);
  } else {
    if (fields.series) {
      const series = String(fields.series).replace(/\s+/g, '');
      if (/^\d{4}$/.test(series)) set('internal_series', series);
    }
    if (fields.number) {
      const number = String(fields.number).replace(/\s+/g, '');
      if (/^\d{6}$/.test(number)) set('internal_number', number);
    }
    if (fields.issue_date) set('internal_issued_ru', isoToDmy(fields.issue_date));
    if (fields.issuing_authority) set('internal_issued_by_ru', fields.issuing_authority);
    if (fields.reg_address) set('reg_address_ru', fields.reg_address);
  }

  return { mapped, filled };
}

// makePassportAutoFillHandler returns the handler the FileUploadField calls
// after a successful /parse-passport. Wraps mapPassportFieldsToPayload in a
// functional setState so concurrent typing doesn't race.
export function makePassportAutoFillHandler(setPayload, setNotice, type) {
  return (fields) => {
    setPayload((p) => {
      const { mapped, filled } = mapPassportFieldsToPayload(fields, p, type);
      if (filled.length === 0) {
        setNotice?.('Все поля уже заполнены.');
        return p;
      }
      setNotice?.(`Поля обновлены (${filled.length}).`);
      return { ...p, ...mapped };
    });
  };
}
