/* eslint-disable react-refresh/only-export-components */
// ReviewStep — wizard step 7. Read-only summary + consent + submit.
// The wizard's bottom action bar replaces the "Next" button with a Submit
// button on this step; the actual onSubmit handler lives in FormWizard.

import { OCCUPATION_OPTIONS, OCCUPATION_DEFAULT } from './OccupationStep';

const dash = '—';
const fmt = (v) => {
  const s = String(v ?? '').trim();
  return s === '' ? dash : s;
};

const FILE_TYPE_LABELS = {
  passport_internal: 'Скан внутреннего паспорта',
  passport_foreign: 'Скан загранпаспорта',
  ticket: 'Авиабилеты',
  voucher: 'Ваучеры на отели',
};

// passport_* are single rows; ticket / voucher are lists since
// migration 000023.
const MULTI_FILE_TYPES = new Set(['ticket', 'voucher']);

function formatSize(bytes) {
  if (!bytes && bytes !== 0) return '';
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

function occupationLabel(payload) {
  const t = payload.occupation_type || OCCUPATION_DEFAULT;
  const opt = OCCUPATION_OPTIONS.find((o) => o.value === t);
  return opt ? opt.label : t;
}

export default function ReviewStep({
  payload, files,
  // Consent props are managed by FormWizard so the submit button can read
  // them without prop-drilling state setters all the way down.
  consent, consentChecked, setConsentChecked, consentLoading,
  consentExpanded, setConsentExpanded,
}) {
  return (
    <div className="fw-step-content">
      <section className="fw-review-section">
        <h3>Личные данные</h3>
        <dl className="fw-review-dl">
          <dt>ФИО кириллицей</dt><dd>{fmt(payload.name_cyr)}</dd>
          <dt>ФИО латиницей</dt><dd>{fmt(payload.name_lat)}</dd>
          <dt>Пол</dt><dd>{fmt(payload.gender_ru)}</dd>
          <dt>Дата рождения</dt><dd>{fmt(payload.birth_date)}</dd>
          <dt>Семейное положение</dt><dd>{fmt(payload.marital_status_ru)}</dd>
          <dt>Место рождения</dt><dd>{fmt(payload.place_of_birth_ru)}</dd>
          <dt>Гражданство</dt><dd>{fmt(payload.nationality_ru)}</dd>
          <dt>Прежнее гражданство</dt><dd>{fmt(payload.former_nationality_ru)}</dd>
          <dt>Другая фамилия</dt><dd>{fmt(payload.maiden_name_ru)}</dd>
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Внутренний паспорт РФ</h3>
        <dl className="fw-review-dl">
          <dt>Серия</dt><dd>{fmt(payload.internal_series)}</dd>
          <dt>Номер</dt><dd>{fmt(payload.internal_number)}</dd>
          <dt>Дата выдачи</dt><dd>{fmt(payload.internal_issued_ru)}</dd>
          <dt>Кем выдан</dt><dd>{fmt(payload.internal_issued_by_ru)}</dd>
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Загранпаспорт</h3>
        <dl className="fw-review-dl">
          <dt>Номер</dt><dd>{fmt(payload.passport_number)}</dd>
          <dt>Тип паспорта</dt><dd>{fmt(payload.passport_type_ru)}</dd>
          <dt>Дата выдачи</dt><dd>{fmt(payload.issue_date)}</dd>
          <dt>Дата окончания</dt><dd>{fmt(payload.expiry_date)}</dd>
          <dt>Кем выдан</dt><dd>{fmt(payload.issued_by_ru)}</dd>
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Адреса</h3>
        <dl className="fw-review-dl">
          <dt>Адрес регистрации</dt><dd>{fmt(payload.reg_address_ru)}</dd>
          <dt>Домашний адрес</dt><dd>{fmt(payload.home_address_ru)}</dd>
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Работа</h3>
        <dl className="fw-review-dl">
          <dt>Род занятий</dt><dd>{occupationLabel(payload)}</dd>
          {(payload.occupation_type || OCCUPATION_DEFAULT) === 'employed' && (
            <>
              <dt>Должность</dt><dd>{fmt(payload.occupation_ru)}</dd>
              <dt>Название организации</dt><dd>{fmt(payload.employer_ru)}</dd>
              <dt>Адрес организации</dt><dd>{fmt(payload.employer_address_ru)}</dd>
              <dt>Телефон организации</dt><dd>{fmt(payload.employer_phone)}</dd>
            </>
          )}
          {(payload.occupation_type === 'student' || payload.occupation_type === 'schoolchild') && (
            <>
              <dt>Учебное заведение</dt><dd>{fmt(payload.employer_ru)}</dd>
              <dt>Адрес учебного заведения</dt><dd>{fmt(payload.employer_address_ru)}</dd>
              <dt>Телефон учебного заведения</dt><dd>{fmt(payload.employer_phone)}</dd>
            </>
          )}
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Поездка</h3>
        <dl className="fw-review-dl">
          <dt>Был ли в Японии</dt><dd>{fmt(payload.been_to_japan_ru)}</dd>
          {payload.been_to_japan_ru === 'Да' && (
            <>
              <dt>Прошлые визиты</dt><dd>{fmt(payload.previous_visits_ru)}</dd>
            </>
          )}
          <dt>Была ли судимость</dt><dd>{fmt(payload.criminal_record_ru)}</dd>
          <dt>Телефон</dt><dd>{fmt(payload.phone)}</dd>
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Прикреплённые файлы</h3>
        <div className="fw-review-files">
          {Object.keys(FILE_TYPE_LABELS).map((key) => {
            const slot = files?.[key];
            const list = MULTI_FILE_TYPES.has(key)
              ? (Array.isArray(slot) ? slot : [])
              : (slot ? [slot] : []);

            if (list.length === 0) {
              return (
                <div key={key} className="fw-review-file">
                  <span className="fw-review-file-name">{FILE_TYPE_LABELS[key]}</span>
                  <span className="fw-review-file-empty">не прикреплён</span>
                </div>
              );
            }
            return (
              <div key={key} className="fw-review-file" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 4 }}>
                <div className="fw-review-file-name">
                  {FILE_TYPE_LABELS[key]}
                  {MULTI_FILE_TYPES.has(key) && list.length > 1 && ` (${list.length})`}
                </div>
                {list.map((f, i) => (
                  <div
                    key={f.id || i}
                    style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}
                  >
                    <span className="fw-review-file-meta" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {f.original_name || 'файл'}
                    </span>
                    <span className="fw-review-file-meta">{formatSize(f.size_bytes)}</span>
                  </div>
                ))}
              </div>
            );
          })}
        </div>
      </section>

      {consent !== undefined && (
        <section className="sf-consent">
          <details
            open={consentExpanded}
            onToggle={(e) => setConsentExpanded(e.target.open)}
          >
            <summary>
              Согласие на обработку персональных данных {consent?.version ? `(v${consent.version})` : ''}
            </summary>
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
    </div>
  );
}

export function validate() {
  // Review has no fields to validate — submit-time validation re-runs every
  // step's validator anyway.
  return {};
}
