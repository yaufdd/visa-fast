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

export default function PersonalStep({ payload, setField, errors }) {
  const { textField, selectField, dateField } = makeFieldFactories({ payload, errors, setField });

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
      {textField('nationality_ru', 'Гражданство')}
      {textField('former_nationality_ru', 'Прежнее гражданство')}

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
    </div>
  );
}

// Validator for PersonalStep — only the Latin name regex; everything else
// is optional in the legacy form too.
export function validate(payload) {
  const errors = {};
  const nameLat = (payload.name_lat || '').trim();
  if (nameLat && !/^[A-Z ]+$/.test(nameLat)) {
    errors.name_lat = 'Только латинские буквы A–Z и пробелы';
  }
  return errors;
}
