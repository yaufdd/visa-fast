/* eslint-disable react-refresh/only-export-components */
// OccupationStep — wizard step 5. Род занятий. Mirrors the radio/select
// + conditional fields layout from SubmissionForm.jsx; the
// applyOccupationAutoFill() runs at final submit, not on per-step Next.

import { makeFieldFactories } from '../fieldFactories';

// Categories where the user types nothing in the Работа section — the
// four employer fields are hidden and auto-filled at submit time.
export const BLANK_OCCUPATION_TYPES = new Set(['ip', 'pensioner', 'housewife', 'unemployed']);

// Categories where the institution name / address / phone are still
// shown (with re-labelled placeholders) but occupation_ru itself is
// fixed.
export const STUDENT_OCCUPATION_TYPES = new Set(['student', 'schoolchild']);

// Categories where the user must NOT type their own occupation_ru —
// the visa office expects a canonical title (e.g. "Студент",
// "Школьник"). The text input is hidden; auto-fill pins the value
// at submit time.
//
// Владелец ООО intentionally falls OUTSIDE this set: many LLC owners
// are "Генеральный директор" / "Учредитель" / "Директор" and the visa
// office expects the real title. The category soft-defaults occupation_ru
// to "Владелец ООО" only when the user leaves the field empty — see
// applyOccupationAutoFill below.
export const LOCKED_OCCUPATION_TITLE_TYPES = new Set([
  'student', 'schoolchild',
]);

export const OCCUPATION_DEFAULT = 'employed';

export const OCCUPATION_OPTIONS = [
  { value: 'employed', label: 'Работаю по найму' },
  { value: 'ip', label: 'Индивидуальный предприниматель' },
  // Owner of an LLC / ООО — same employer fields as 'employed' (LLC name,
  // registered address, contact phone), but occupation_ru is locked to
  // "Владелец ООО" because the visa office wants the canonical title.
  { value: 'business_owner', label: 'Владелец ООО' },
  { value: 'pensioner', label: 'Пенсионер' },
  { value: 'housewife', label: 'Домохозяйка' },
  { value: 'unemployed', label: 'Безработный' },
  { value: 'student', label: 'Студент' },
  { value: 'schoolchild', label: 'Школьник' },
];

// applyOccupationAutoFill — called at final submit time only. Returns a new
// payload with the Работа section normalised to what the visa form expects.
export function applyOccupationAutoFill(payload) {
  const type = payload.occupation_type || OCCUPATION_DEFAULT;
  const dash = '—';
  const lastName = String(payload.name_cyr || '').trim().split(/\s+/)[0] || '';
  const out = { ...payload };
  switch (type) {
    case 'ip':
      out.occupation_ru = 'ИП';
      out.employer_ru = lastName ? `ИП ${lastName}` : 'ИП';
      out.employer_address_ru = payload.home_address_ru || payload.reg_address_ru || '';
      out.employer_phone = payload.phone || '';
      break;
    case 'business_owner':
      // Soft default: only fill occupation_ru when the user left the
      // position field empty. The visa office expects the real title
      // (e.g. "Генеральный директор", "Учредитель", "Директор") and
      // overwriting a typed value would be wrong. The "Владелец ООО"
      // → "BUSINESS OWNER" mapping in assembler.go still handles the
      // empty-field case so the legacy behaviour is preserved.
      if (!String(out.occupation_ru || '').trim()) {
        out.occupation_ru = 'Владелец ООО';
      }
      break;
    case 'pensioner':
      out.occupation_ru = 'Пенсионер';
      out.employer_ru = dash;
      out.employer_address_ru = dash;
      out.employer_phone = dash;
      break;
    case 'housewife':
      out.occupation_ru = 'Домохозяйка';
      out.employer_ru = dash;
      out.employer_address_ru = dash;
      out.employer_phone = dash;
      break;
    case 'unemployed':
      out.occupation_ru = 'Безработный';
      out.employer_ru = dash;
      out.employer_address_ru = dash;
      out.employer_phone = dash;
      break;
    case 'student':
      out.occupation_ru = 'Студент';
      break;
    case 'schoolchild':
      out.occupation_ru = 'Школьник';
      break;
    case 'employed':
    default:
      break;
  }
  return out;
}

export default function OccupationStep({ payload, setField, errors }) {
  const { textField, textareaField, phoneField } = makeFieldFactories({ payload, errors, setField });
  const type = payload.occupation_type || OCCUPATION_DEFAULT;

  return (
    <div className="fw-step-content">
      <label className="sf-field" data-field="occupation_type">
        <span className="sf-label">Род занятий</span>
        <select
          value={type}
          onChange={(e) => setField('occupation_type', e.target.value)}
        >
          {OCCUPATION_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </label>

      {type === 'employed' && (
        <>
          {textField('occupation_ru', 'Должность')}
          {textField('employer_ru', 'Название организации')}
          {textareaField('employer_address_ru', 'Адрес организации')}
          {phoneField('employer_phone', 'Телефон организации')}
        </>
      )}

      {/* Владелец ООО — same employer fields as 'employed' plus a free
          position field. occupation_ru soft-defaults to "Владелец ООО"
          only when left empty (see applyOccupationAutoFill); typing
          "Генеральный директор" / "Учредитель" / "Директор" overrides
          the default and ships verbatim to the visa office. */}
      {type === 'business_owner' && (
        <>
          {textField('occupation_ru', 'Должность', {
            hint: 'Например: Генеральный директор / Учредитель / Владелец ООО.',
          })}
          {textField('employer_ru', 'Название организации')}
          {textareaField('employer_address_ru', 'Адрес организации')}
          {phoneField('employer_phone', 'Телефон организации')}
        </>
      )}

      {STUDENT_OCCUPATION_TYPES.has(type) && (
        <>
          {textField('employer_ru', 'Название учебного заведения')}
          {textareaField('employer_address_ru', 'Адрес учебного заведения')}
          {phoneField('employer_phone', 'Телефон учебного заведения')}
        </>
      )}
    </div>
  );
}

export function validate(payload) {
  const errors = {};
  const occType = payload.occupation_type || OCCUPATION_DEFAULT;
  if (STUDENT_OCCUPATION_TYPES.has(occType)) {
    if (!String(payload.employer_ru || '').trim()) {
      errors.employer_ru = 'Укажите название учебного заведения';
    }
    if (!String(payload.employer_address_ru || '').trim()) {
      errors.employer_address_ru = 'Укажите адрес учебного заведения';
    }
  }
  return errors;
}
