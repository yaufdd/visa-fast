/* eslint-disable react-refresh/only-export-components */
// PersonalStep — wizard step 1. Name, gender, birth, marital status,
// place of birth, nationality, former nationality, maiden name. Cyrillic→
// Latin auto-fill is preserved on the name_cyr field.
//
// Co-locating the `validate(payload)` sibling with the component is
// intentional (one file per step). The HMR-only rule below objects to
// the mixed export — overriding it is the lighter trade-off.

import { ruToLatICAO } from '../../../utils/translit';
import { makeFieldFactories, sanitizeLatin } from '../fieldFactories';

// NATIONALITY_PRESETS — the small set of post-Soviet citizenships that
// the assembler's countryISOMap recognises. Anything else falls into the
// "Другое" branch where the tourist types the country name themselves.
// Keys MUST match the canonical strings in
// backend/internal/ai/mappings.go:countryISOMap so CountryISO() resolves
// correctly downstream.
export const NATIONALITY_PRESETS = ['Россия', 'Беларусь', 'Казахстан'];

export default function PersonalStep({ payload, setField, errors }) {
  const { textField, selectField, dateField, phoneField } = makeFieldFactories({ payload, errors, setField });

  // The dropdown stores its value in nationality_choice (UI-only). The
  // backend reads nationality_ru — we sync the two on every change so
  // the assembler sees the canonical value the dropdown picks. Selecting
  // "other" leaves nationality_ru for the user to type.
  const handleNationalityChoice = (next) => {
    setField('nationality_choice', next);
    if (next !== 'other') {
      setField('nationality_ru', next);
    } else {
      // Don't clobber whatever the user previously typed when they flip
      // back to "Другое" — but if nationality_ru still matched a preset
      // it would just reappear in the dropdown next render. Clearing it
      // here forces an explicit re-entry.
      setField('nationality_ru', '');
    }
  };

  // Former nationality dropdown — same shape as nationality. Anything other
  // than 'other' syncs verbatim to former_nationality_ru. Picking 'other'
  // clears the field so the user types a country name. Note:
  // ComputeFormerNationality on the backend recognises only "СССР" /
  // "Soviet" / "USSR" patterns — anything else falls through to the
  // place-of-birth fallback or "NO". That is accepted; the form just
  // collects what the user types.
  const handleFormerNationalityChoice = (next) => {
    setField('former_nationality_choice', next);
    if (next !== 'other') {
      setField('former_nationality_ru', next);
    } else {
      setField('former_nationality_ru', '');
    }
  };

  // handleCyrChange — preserves the existing one-way ICAO transliteration
  // from name_cyr → name_lat (only fires if the user is typing in Cyrillic;
  // editing the Latin field directly is supported below).
  const handleCyrChange = (value) => {
    setField('name_cyr', value);
    setField('name_lat', ruToLatICAO(value));
  };

  const handleLatChange = (value) => {
    setField('name_lat', sanitizeLatin(value));
  };

  return (
    <div className="fw-step-content">
      <label className="sf-field" data-field="name_cyr">
        <span className="sf-label">ФИО кириллицей</span>
        <input
          type="text"
          value={payload.name_cyr ?? ''}
          onChange={(e) => handleCyrChange(e.target.value)}
          autoComplete="off"
        />
      </label>

      <label className={`sf-field${errors.name_lat ? ' has-error' : ''}`} data-field="name_lat">
        <span className="sf-label">ФИО латиницей</span>
        <input
          type="text"
          value={payload.name_lat ?? ''}
          onChange={(e) => handleLatChange(e.target.value)}
          autoComplete="off"
        />
        {!errors.name_lat && (
          <span className="sf-hint">
            Пожалуйста, проверьте, что написание совпадает с вашим загранпаспортом. Если отличается — отредактируйте поле вручную.
          </span>
        )}
        {errors.name_lat && <span className="sf-error">{errors.name_lat}</span>}
      </label>

      {selectField('gender_ru', 'Пол', [
        { value: '', label: '—' },
        { value: 'Мужской', label: 'Мужской' },
        { value: 'Женский', label: 'Женский' },
      ])}

      {dateField('birth_date', 'Дата рождения')}

      {selectField('marital_status_ru', 'Семейное положение', [
        { value: '', label: '—' },
        { value: 'Холост/не замужем', label: 'Холост / не замужем' },
        { value: 'Женат/замужем', label: 'Женат / замужем' },
        { value: 'Вдовец/вдова', label: 'Вдовец / вдова' },
        { value: 'Разведен(а)', label: 'Разведён(а)' },
      ])}

      {textField('place_of_birth_ru', 'Место рождения')}

      {/* Nationality dropdown. nationality_choice is UI-only state; the
          authoritative field the backend reads is nationality_ru — kept
          in sync via handleNationalityChoice. The "Другое" branch reveals
          a free-text input for nationalities outside the preset list. */}
      <label className="sf-field" data-field="nationality_choice">
        <span className="sf-label">Гражданство</span>
        <select
          value={payload.nationality_choice ?? 'Россия'}
          onChange={(e) => handleNationalityChoice(e.target.value)}
        >
          {NATIONALITY_PRESETS.map((v) => (
            <option key={v} value={v}>{v}</option>
          ))}
          <option value="other">Другое (указать)</option>
        </select>
      </label>

      {payload.nationality_choice === 'other'
        && textField('nationality_ru', 'Гражданство (введите страну)')}

      {/* Former nationality dropdown — Нет / СССР / Другое. The choice is
          UI-only state; the authoritative field the backend reads is
          former_nationality_ru, kept in sync by handleFormerNationalityChoice.
          Hint sits under the field — when "Другое" is chosen the hint moves
          to the free-text input below so it always trails the active control. */}
      <label className="sf-field" data-field="former_nationality_choice">
        <span className="sf-label">Прежнее гражданство</span>
        <select
          value={payload.former_nationality_choice ?? 'Нет'}
          onChange={(e) => handleFormerNationalityChoice(e.target.value)}
        >
          <option value="Нет">Нет</option>
          <option value="СССР">СССР</option>
          <option value="other">Другое (указать)</option>
        </select>
        {payload.former_nationality_choice !== 'other' && (
          <span className="sf-hint">Если вы родились в СССР, выберите «СССР».</span>
        )}
      </label>

      {payload.former_nationality_choice === 'other'
        && textField('former_nationality_ru', 'Прежнее гражданство (введите страну)', {
          hint: 'Если вы родились в СССР, выберите «СССР».',
        })}

      {/* Yes/No toggle for previous surname. The free-text trap (typing
          "Нет" → ICAO transliterated to "NET" in the visa anketa PDF) was
          the original bug that motivated this control. When the tourist
          flips back from "Да" to "Нет" we also clear maiden_name_ru so a
          previously-typed surname doesn't ship to the PDF. */}
      <label className="sf-field" data-field="had_other_name">
        <span className="sf-label">Была ли другая фамилия?</span>
        <select
          value={payload.had_other_name ?? 'Нет'}
          onChange={(e) => {
            const next = e.target.value;
            setField('had_other_name', next);
            if (next !== 'Да') {
              setField('maiden_name_ru', '');
            }
          }}
        >
          <option value="Нет">Нет</option>
          <option value="Да">Да</option>
        </select>
      </label>

      {payload.had_other_name === 'Да' && textField('maiden_name_ru', 'Какая фамилия была раньше?')}

      {/* Phone moved here from TravelDocsStep — it belongs with the
          tourist's personal contact details, not with travel documents.
          Required (visa anketa needs a contact phone). */}
      {phoneField('phone', 'Телефон')}
    </div>
  );
}

// Validator for PersonalStep — Latin name regex + phone required.
// Phone migrated from TravelDocsStep; it is the visa anketa's contact
// number and the form should not let the tourist past Step 1 without
// providing it.
export function validate(payload) {
  const errors = {};
  const nameLat = (payload.name_lat || '').trim();
  if (nameLat && !/^[A-Z ]+$/.test(nameLat)) {
    errors.name_lat = 'Только латинские буквы A–Z и пробелы';
  }
  if (!String(payload.phone || '').trim()) {
    errors.phone = 'Укажите контактный телефон';
  }
  return errors;
}
