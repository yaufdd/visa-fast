import { useEffect, useMemo, useState } from 'react';
import { getConsentText } from '../api/client';

// Field definitions grouped into blocks. Each field has a name, label, and optional type/hint.
// Field names match the backend "payload" shape expected by /api/submissions.
const BLOCKS = [
  {
    id: 'persons',
    title: 'Блок 1 · Личные данные',
    fields: [
      { name: 'name_cyr', label: 'ФИО (кириллицей, как в паспорте РФ)' },
      { name: 'name_lat', label: 'ФИО латиницей (как в загранпаспорте)', latin: true, hint: 'Только A–Z и пробелы' },
      { name: 'sex', label: 'Пол', type: 'select', options: [
        { value: '', label: '—' },
        { value: 'M', label: 'Мужской (M)' },
        { value: 'F', label: 'Женский (F)' },
      ] },
      { name: 'date_of_birth', label: 'Дата рождения', type: 'date-hint', hint: 'ДД.ММ.ГГГГ' },
      { name: 'place_of_birth', label: 'Место рождения (город, страна / USSR, если до 1991)' },
      { name: 'nationality', label: 'Гражданство', placeholder: 'RUSSIA' },
      { name: 'former_nationality', label: 'Прежнее гражданство', placeholder: 'USSR / NO' },
      { name: 'marital_status', label: 'Семейное положение', type: 'select', options: [
        { value: '', label: '—' },
        { value: 'SINGLE', label: 'Не женат / не замужем' },
        { value: 'MARRIED', label: 'Женат / замужем' },
        { value: 'DIVORCED', label: 'В разводе' },
        { value: 'WIDOWED', label: 'Вдовец / вдова' },
      ] },
    ],
  },
  {
    id: 'passport',
    title: 'Блок 2 · Загранпаспорт',
    fields: [
      { name: 'passport_number', label: 'Номер загранпаспорта', hint: '9 символов' },
      { name: 'passport_issue_date', label: 'Дата выдачи', type: 'date-hint', hint: 'ДД.ММ.ГГГГ' },
      { name: 'passport_expiry_date', label: 'Дата окончания', type: 'date-hint', hint: 'ДД.ММ.ГГГГ' },
      { name: 'passport_issued_by', label: 'Кем выдан' },
    ],
  },
  {
    id: 'internal',
    title: 'Блок 3 · Внутренний паспорт РФ',
    fields: [
      { name: 'internal_series', label: 'Серия', hint: '4 цифры' },
      { name: 'internal_number', label: 'Номер', hint: '6 цифр' },
      { name: 'internal_issue_date', label: 'Дата выдачи', type: 'date-hint', hint: 'ДД.ММ.ГГГГ' },
      { name: 'internal_issued_by', label: 'Кем выдан' },
      { name: 'internal_code', label: 'Код подразделения' },
    ],
  },
  {
    id: 'contacts',
    title: 'Блок 4 · Контакты',
    fields: [
      { name: 'home_address', label: 'Адрес регистрации', type: 'textarea' },
      { name: 'phone', label: 'Телефон', placeholder: '+7...' },
      { name: 'email', label: 'Email', type: 'email' },
    ],
  },
  {
    id: 'work',
    title: 'Блок 5 · Работа',
    fields: [
      { name: 'occupation', label: 'Должность / профессия' },
      { name: 'employer_name', label: 'Название организации' },
      { name: 'employer_address', label: 'Адрес организации', type: 'textarea' },
      { name: 'employer_phone', label: 'Телефон организации' },
    ],
  },
  {
    id: 'history',
    title: 'Блок 6 · История поездок',
    fields: [
      { name: 'prev_japan_visits', label: 'Предыдущие визиты в Японию (даты / нет)', type: 'textarea' },
      { name: 'prev_visa_refusals', label: 'Отказы в визе (страна, дата / нет)', type: 'textarea' },
      { name: 'relatives_in_japan', label: 'Родственники в Японии (ФИО, адрес / нет)', type: 'textarea' },
    ],
  },
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
    for (const block of BLOCKS) {
      for (const f of block.fields) base[f.name] = '';
    }
    return { ...base, ...initialPayload };
  }, [initialPayload]);

  const [payload, setPayload] = useState(initialState);
  const [openBlock, setOpenBlock] = useState('persons');
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

  const onFieldChange = (name, latin) => (e) => {
    const raw = e.target.value;
    const v = latin ? sanitizeLatin(raw) : raw;
    setPayload((p) => ({ ...p, [name]: v }));
    if (errors[name]) {
      setErrors((prev) => {
        const n = { ...prev };
        delete n[name];
        return n;
      });
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setApiError('');
    const errs = validate(payload);
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      // Open the first block that has an error
      for (const block of BLOCKS) {
        if (block.fields.some((f) => errs[f.name])) {
          setOpenBlock(block.id);
          break;
        }
      }
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

  return (
    <form className="submission-form" onSubmit={handleSubmit} noValidate>
      {BLOCKS.map((block) => {
        const isOpen = openBlock === block.id;
        return (
          <section key={block.id} className={`sf-block${isOpen ? ' open' : ''}`}>
            <button
              type="button"
              className="sf-block-header"
              onClick={() => setOpenBlock(isOpen ? null : block.id)}
              aria-expanded={isOpen}
            >
              <span>{block.title}</span>
              <span className="sf-chevron">{isOpen ? '−' : '+'}</span>
            </button>
            {isOpen && (
              <div className="sf-block-body">
                {block.fields.map((f) => {
                  const val = payload[f.name] ?? '';
                  const err = errors[f.name];
                  return (
                    <label key={f.name} className={`sf-field${err ? ' has-error' : ''}`}>
                      <span className="sf-label">{f.label}</span>
                      {f.type === 'textarea' ? (
                        <textarea
                          value={val}
                          onChange={onFieldChange(f.name, f.latin)}
                          placeholder={f.placeholder || ''}
                          rows={3}
                        />
                      ) : f.type === 'select' ? (
                        <select value={val} onChange={onFieldChange(f.name, false)}>
                          {f.options.map((opt) => (
                            <option key={opt.value} value={opt.value}>{opt.label}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          type={f.type === 'email' ? 'email' : 'text'}
                          value={val}
                          onChange={onFieldChange(f.name, f.latin)}
                          placeholder={f.placeholder || ''}
                          autoComplete="off"
                        />
                      )}
                      {f.hint && !err && <span className="sf-hint">{f.hint}</span>}
                      {err && <span className="sf-error">{err}</span>}
                    </label>
                  );
                })}
              </div>
            )}
          </section>
        );
      })}

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
          gap: 12px;
          max-width: 760px;
          margin: 0 auto;
        }

        .sf-block {
          background: var(--graphite);
          border: 1px solid var(--border);
          border-radius: 10px;
          overflow: hidden;
        }

        .sf-block-header {
          width: 100%;
          background: transparent;
          color: var(--white);
          padding: 14px 18px;
          font-size: 14px;
          font-weight: 600;
          display: flex;
          align-items: center;
          justify-content: space-between;
          cursor: pointer;
          border: none;
          text-align: left;
        }

        .sf-block-header:hover {
          background: var(--gray-dark);
        }

        .sf-chevron {
          font-family: var(--font-mono);
          color: var(--white-dim);
          font-size: 16px;
          line-height: 1;
          width: 16px;
          text-align: center;
        }

        .sf-block.open .sf-chevron {
          color: var(--accent);
        }

        .sf-block-body {
          padding: 16px 18px 20px;
          border-top: 1px solid var(--border);
          display: grid;
          grid-template-columns: 1fr 1fr;
          gap: 14px;
        }

        @media (max-width: 600px) {
          .sf-block-body { grid-template-columns: 1fr; }
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
