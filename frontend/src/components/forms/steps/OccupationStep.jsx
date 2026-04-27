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
// the visa office expects a canonical title (e.g. "Владелец ООО",
// "Студент", "Школьник"). The text input is hidden; auto-fill pins
// the value at submit time.
export const LOCKED_OCCUPATION_TITLE_TYPES = new Set([
  'business_owner', 'student', 'schoolchild',
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
      // Same employer-fields layout as 'employed' (LLC name + registered
      // address + contact phone, all user-typed). Only the title is
      // pinned — the visa office expects "Владелец ООО" verbatim, mapped
      // by the assembler to "BUSINESS OWNER" in English.
      out.occupation_ru = 'Владелец ООО';
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

      {/* Владелец ООО — occupation_ru is auto-pinned to "Владелец ООО"
          (see applyOccupationAutoFill), so the title input is hidden.
          The three employer fields keep their "Название организации /
          Адрес организации / Телефон организации" labels because
          they're filled with the LLC's data, exactly as in 'employed'. */}
      {type === 'business_owner' && (
        <>
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

      {BLANK_OCCUPATION_TYPES.has(type) && (
        <p className="sf-hint sf-occupation-note">
          Поля раздела «Работа» заполнятся автоматически.
        </p>
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
