import { useEffect, useMemo, useState } from 'react';
import { getConsentText } from '../api/client';
import { ruToLatICAO } from '../utils/translit';
import { dmyToIso, isoToDmy } from '../utils/dates';
import { normalizePhone } from '../utils/phone';

// Flat submission form. Field names MUST match the backend "payload"
// contract (see backend/internal/api/submissions.go and
// backend/internal/ai/assembler.go). All enum values are the Russian
// strings expected by MapGender / MapMaritalStatus / MapPassportType /
// MapYesNo / CountryISO helpers.

// Defaults for selects that should not start empty.
const SELECT_DEFAULTS = {
  passport_type_ru: 'Обычный',
  been_to_japan_ru: 'Нет',
  criminal_record_ru: 'Нет',
  gender_ru: '',
  marital_status_ru: '',
};

// All fields the form touches. Phone fields aren't yet in the backend
// assembler but are part of the payload JSONB we POST.
const ALL_FIELDS = [
  'name_cyr', 'name_lat', 'gender_ru', 'birth_date', 'marital_status_ru',
  'place_of_birth_ru', 'nationality_ru', 'former_nationality_ru', 'maiden_name_ru',
  'passport_number', 'passport_type_ru', 'issue_date', 'expiry_date', 'issued_by_ru',
  'internal_series', 'internal_number', 'internal_issued_ru', 'internal_issued_by_ru',
  'reg_address_ru', 'home_address_ru', 'phone',
  'occupation_ru', 'employer_ru', 'employer_address_ru', 'employer_phone',
  'been_to_japan_ru', 'previous_visits_ru', 'criminal_record_ru',
];

function sanitizeLatin(value) {
  return value.toUpperCase().replace(/[^A-Z\s]/g, '');
}

function validate(payload) {
  const errors = {};
  const nameLat = (payload.name_lat || '').trim();
  if (nameLat && !/^[A-Z ]+$/.test(nameLat)) {
    errors.name_lat = 'Только латинские буквы A–Z и пробелы';
  }
  const series = (payload.internal_series || '').trim();
  if (series && !/^\d{4}$/.test(series)) {
    errors.internal_series = 'Должно быть ровно 4 цифры';
  }
  const intNum = (payload.internal_number || '').trim();
  if (intNum && !/^\d{6}$/.test(intNum)) {
    errors.internal_number = 'Должно быть ровно 6 цифр';
  }
  const passNum = (payload.passport_number || '').trim();
  if (passNum && passNum.length !== 9) {
    errors.passport_number = 'Должно быть 9 символов';
  }
  return errors;
}

export default function SubmissionForm({
  onSubmit,
  initialPayload = {},
  submitLabel = 'Отправить анкету',
  showConsent = true,
}) {
  const initialState = useMemo(() => {
    const base = {};
    for (const name of ALL_FIELDS) {
      base[name] = SELECT_DEFAULTS[name] ?? '';
    }
    return { ...base, ...initialPayload };
  }, [initialPayload]);

  const [payload, setPayload] = useState(initialState);
  const [latManuallyEdited, setLatManuallyEdited] = useState(
    Boolean(initialPayload.name_lat)
  );
  const [consentChecked, setConsentChecked] = useState(false);
  const [consent, setConsent] = useState(null);
  const [consentLoading, setConsentLoading] = useState(showConsent);
  const [consentExpanded, setConsentExpanded] = useState(false);
  const [errors, setErrors] = useState({});
  const [apiError, setApiError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!showConsent) return;
    setConsentLoading(true);
    getConsentText()
      .then((data) => setConsent(data))
      .catch(() => setConsent({ version: '?', body: 'Не удалось загрузить текст согласия.' }))
      .finally(() => setConsentLoading(false));
  }, [showConsent]);

  const clearError = (name) => {
    if (errors[name]) {
      setErrors((prev) => {
        const n = { ...prev };
        delete n[name];
        return n;
      });
    }
  };

  const setField = (name, value) => {
    setPayload((p) => ({ ...p, [name]: value }));
    clearError(name);
  };

  // Cyrillic name → auto-derived Latin, unless user has edited Latin.
  const handleCyrChange = (value) => {
    setPayload((p) => ({
      ...p,
      name_cyr: value,
      name_lat: latManuallyEdited ? p.name_lat : ruToLatICAO(value),
    }));
    clearError('name_cyr');
    if (!latManuallyEdited) clearError('name_lat');
  };

  const handleLatChange = (value) => {
    setLatManuallyEdited(true);
    setField('name_lat', sanitizeLatin(value));
  };

  const handleDateChange = (name) => (e) => {
    setField(name, isoToDmy(e.target.value));
  };

  const handlePhoneBlur = (name) => (e) => {
    const normalized = normalizePhone(e.target.value);
    if (normalized !== e.target.value) setField(name, normalized);
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setApiError('');
    const errs = validate(payload);
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      // Scroll the first errored field into view
      const firstErr = Object.keys(errs)[0];
      const el = document.querySelector(`[data-field="${firstErr}"]`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      return;
    }
    if (showConsent && !consentChecked) {
      setApiError('Необходимо подтвердить согласие на обработку персональных данных.');
      return;
    }
    setSubmitting(true);
    try {
      await onSubmit(payload, consentChecked);
    } catch (err) {
      setApiError(err?.message || 'Не удалось отправить анкету.');
    } finally {
      setSubmitting(false);
    }
  };

  const canSubmit = !submitting && (!showConsent || consentChecked);

  const showPreviousVisits = payload.been_to_japan_ru === 'Да';

  // Small helpers for common field types
  const textField = (name, label, extra = {}) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="text"
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
          placeholder={extra.placeholder || ''}
          autoComplete="off"
        />
        {extra.hint && !err && <span className="sf-hint">{extra.hint}</span>}
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const textareaField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <textarea
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
          rows={3}
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const selectField = (name, label, options) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <select
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const dateField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="date"
          value={dmyToIso(payload[name] ?? '')}
          onChange={handleDateChange(name)}
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const phoneField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="tel"
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
          onBlur={handlePhoneBlur(name)}
          placeholder="+7 (999) 123-45-67"
          autoComplete="off"
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  return (
    <form className="submission-form" onSubmit={handleSubmit} noValidate>
      <h2 className="sf-heading">Личные данные</h2>

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
            Заполняется автоматически из кириллицы (ICAO). Если в загранпаспорте другой вариант — отредактируйте.
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
      {textField('maiden_name_ru', 'Была ли другая фамилия? Какая?')}

      <h2 className="sf-heading">Загранпаспорт</h2>

      {textField('passport_number', 'Номер загранпаспорта', { hint: '9 символов' })}
      {selectField('passport_type_ru', 'Тип паспорта', [
        { value: 'Обычный', label: 'Обычный' },
        { value: 'Дипломатический', label: 'Дипломатический' },
        { value: 'Служебный', label: 'Служебный' },
      ])}
      {dateField('issue_date', 'Дата выдачи')}
      {dateField('expiry_date', 'Дата окончания')}
      {textField('issued_by_ru', 'Кем выдан')}

      <h2 className="sf-heading">Внутренний паспорт РФ</h2>

      {textField('internal_series', 'Серия', { hint: '4 цифры' })}
      {textField('internal_number', 'Номер', { hint: '6 цифр' })}
      {dateField('internal_issued_ru', 'Дата выдачи')}
      {textField('internal_issued_by_ru', 'Кем выдан')}
      {textareaField('reg_address_ru', 'Адрес регистрации')}

      <h2 className="sf-heading">Контакты</h2>

      {textareaField('home_address_ru', 'Домашний адрес')}
      {phoneField('phone', 'Телефон')}

      <h2 className="sf-heading">Работа</h2>

      {textField('occupation_ru', 'Должность')}
      {textField('employer_ru', 'Название организации')}
      {textareaField('employer_address_ru', 'Адрес организации')}
      {phoneField('employer_phone', 'Телефон организации')}

      <h2 className="sf-heading">История</h2>

      {selectField('been_to_japan_ru', 'Был ли в Японии', [
        { value: 'Нет', label: 'Нет' },
        { value: 'Да', label: 'Да' },
      ])}

      {showPreviousVisits && textareaField('previous_visits_ru', 'Даты прошлых визитов')}

      {selectField('criminal_record_ru', 'Была ли судимость', [
        { value: 'Нет', label: 'Нет' },
        { value: 'Да', label: 'Да' },
      ])}

      {showConsent && (
        <section className="sf-consent">
          <details open={consentExpanded} onToggle={(e) => setConsentExpanded(e.target.open)}>
            <summary>Согласие на обработку персональных данных {consent?.version ? `(v${consent.version})` : ''}</summary>
            <pre className="sf-consent-text">
              {consentLoading ? 'Загрузка…' : (consent?.body || '')}
            </pre>
          </details>
          <label className="sf-consent-check">
            <input
              type="checkbox"
              checked={consentChecked}
              onChange={(e) => setConsentChecked(e.target.checked)}
            />
            <span>Я прочитал(а) и подтверждаю согласие на обработку персональных данных.</span>
          </label>
        </section>
      )}

      {apiError && <div className="sf-api-error">{apiError}</div>}

      <div className="sf-actions">
        <button type="submit" className="sf-submit" disabled={!canSubmit}>
          {submitting ? 'Отправка…' : submitLabel}
        </button>
      </div>

      <style>{`
        .submission-form {
          display: flex;
          flex-direction: column;
          gap: 14px;
          max-width: 560px;
          margin: 0 auto;
        }

        .sf-heading {
          margin: 18px 0 2px;
          font-size: 13px;
          font-weight: 600;
          letter-spacing: 0.08em;
          text-transform: uppercase;
          color: var(--accent);
          font-family: var(--font-mono);
          border-bottom: 1px solid var(--border);
          padding-bottom: 6px;
        }

        .sf-heading:first-child {
          margin-top: 0;
        }

        .sf-field {
          display: flex;
          flex-direction: column;
          gap: 6px;
        }

        .sf-label {
          font-size: 12px;
          color: var(--white-dim);
          letter-spacing: 0.02em;
        }

        .sf-field input,
        .sf-field textarea,
        .sf-field select {
          background: var(--gray-dark);
          color: var(--white);
          border: 1px solid var(--border);
          border-radius: 6px;
          padding: 10px 12px;
          font-size: 14px;
          font-family: var(--font-body);
          transition: border-color 0.15s, background 0.15s;
          width: 100%;
        }

        .sf-field textarea {
          resize: vertical;
          min-height: 72px;
        }

        .sf-field input:focus,
        .sf-field textarea:focus,
        .sf-field select:focus {
          outline: none;
          border-color: var(--accent);
          background: var(--gray);
        }

        .sf-field.has-error input,
        .sf-field.has-error textarea,
        .sf-field.has-error select {
          border-color: var(--danger);
        }

        .sf-hint {
          font-size: 11px;
          color: var(--white-dim);
          font-family: var(--font-mono);
          line-height: 1.4;
        }

        .sf-error {
          font-size: 12px;
          color: var(--danger);
        }

        .sf-consent {
          background: var(--graphite);
          border: 1px solid var(--border);
          border-radius: 10px;
          padding: 14px 18px;
          display: flex;
          flex-direction: column;
          gap: 12px;
          margin-top: 10px;
        }

        .sf-consent details > summary {
          cursor: pointer;
          font-size: 14px;
          font-weight: 600;
          color: var(--white);
          list-style: none;
        }

        .sf-consent details > summary::-webkit-details-marker { display: none; }

        .sf-consent details > summary::before {
          content: '▸';
          display: inline-block;
          margin-right: 8px;
          color: var(--white-dim);
          transition: transform 0.15s;
        }

        .sf-consent details[open] > summary::before {
          transform: rotate(90deg);
        }

        .sf-consent-text {
          margin-top: 10px;
          padding: 12px 14px;
          background: var(--gray-dark);
          border: 1px solid var(--border);
          border-radius: 6px;
          max-height: 260px;
          overflow-y: auto;
          font-family: var(--font-mono);
          font-size: 12px;
          color: var(--white-dim);
          white-space: pre-wrap;
          line-height: 1.5;
        }

        .sf-consent-check {
          display: flex;
          align-items: flex-start;
          gap: 10px;
          font-size: 13px;
          color: var(--white);
          cursor: pointer;
          user-select: none;
        }

        .sf-consent-check input[type="checkbox"] {
          margin-top: 3px;
          accent-color: var(--accent);
        }

        .sf-api-error {
          background: rgba(239, 68, 68, 0.1);
          border: 1px solid var(--danger);
          color: var(--danger);
          padding: 10px 14px;
          border-radius: 6px;
          font-size: 13px;
        }

        .sf-actions {
          display: flex;
          justify-content: flex-end;
          margin-top: 8px;
        }

        .sf-submit {
          background: var(--accent);
          color: #fff;
          border: none;
          border-radius: 8px;
          padding: 12px 22px;
          font-size: 14px;
          font-weight: 600;
          cursor: pointer;
          transition: background 0.15s;
        }

        .sf-submit:hover:not(:disabled) {
          background: var(--accent-hover);
        }

        .sf-submit:disabled {
          opacity: 0.5;
          cursor: not-allowed;
        }
      `}</style>
    </form>
  );
}
