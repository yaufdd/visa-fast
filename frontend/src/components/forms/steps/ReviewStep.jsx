/* eslint-disable react-refresh/only-export-components */
// ReviewStep — wizard step 7. Read-only summary + consent + submit.
// The wizard's bottom action bar replaces the "Next" button with a Submit
// button on this step; the actual onSubmit handler lives in FormWizard.

import { OCCUPATION_OPTIONS, OCCUPATION_DEFAULT } from './OccupationStep';
import SmartCaptcha from '../SmartCaptcha';

const dash = '—';
const fmt = (v) => {
  const s = String(v ?? '').trim();
  return s === '' ? dash : s;
};

// File-type labels for the review's "Прикреплённые файлы" section.
// passport_foreign is intentionally not in the public list — the public
// form no longer offers a foreign-passport upload step (uploading the
// scan is admin-only), so showing a "не прикреплён" row would just
// confuse the tourist. Admin sees the full list below.
const FILE_TYPE_LABELS_PUBLIC = {
  passport_internal: 'Скан внутреннего паспорта',
  passport_foreign: 'Скан загранпаспорта',
  ticket: 'Авиабилеты',
  voucher: 'Ваучеры на отели',
};
const FILE_TYPE_LABELS_ADMIN = {
  passport_internal: 'Скан внутреннего паспорта',
  passport_foreign: 'Скан загранпаспорта',
  ticket: 'Авиабилеты',
  voucher: 'Ваучеры на отели',
};

// Map every server-required payload key to the wizard step id that
// owns its input. ReviewStep uses this to translate the server's
// `missing` array into the set of section ids that should glow.
const FIELD_TO_STEP_ID = {
  name_cyr: 'personal',
  name_lat: 'personal',
  gender_ru: 'personal',
  birth_date: 'personal',
  marital_status_ru: 'personal',
  phone: 'personal',
  passport_number: 'foreign',
  issue_date: 'foreign',
  expiry_date: 'foreign',
  home_address_ru: 'addresses',
};

// passport_foreign is single; passport_internal joined ticket/voucher as
// a stackable type in migration 000024 (manager often uploads multiple
// passport pages).
const MULTI_FILE_TYPES = new Set(['passport_internal', 'ticket', 'voucher']);

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
  payload, files, adapter, goToStepById, missingFields,
  // Consent props are managed by FormWizard so the submit button can read
  // them without prop-drilling state setters all the way down.
  consent, consentChecked, setConsentChecked, consentLoading,
  consentExpanded, setConsentExpanded,
  // SmartCaptcha — only rendered when the wizard is in public mode AND
  // siteKey is non-empty. Both flags are passed down by FormWizard.
  captchaToken, setCaptchaToken, captchaResetSignal, siteKey,
}) {
  // The public form skips the internal-passport step entirely, so the
  // review summary should hide that section too — otherwise the tourist
  // sees four "—" rows that they could never have filled.
  const isPublic = Boolean(adapter?.isPublic);
  // Translate the server's `missing` array into the set of section ids
  // whose container should glow. Empty by default — only fills after a
  // failed submit attempt that came back with a missing-fields error.
  const glowingSections = new Set(
    (Array.isArray(missingFields) ? missingFields : [])
      .map((k) => FIELD_TO_STEP_ID[k])
      .filter(Boolean)
  );
  // Class helper: every clickable summary card gets fw-review-clickable;
  // the ones in glowingSections additionally get fw-review-missing so
  // CSS can fade in a red border.
  const sectionClass = (id) => {
    const base = 'fw-review-section fw-review-clickable';
    return glowingSections.has(id) ? `${base} fw-review-missing` : base;
  };
  // Each summary section is clickable: tap the header → wizard jumps back
  // to that step so the tourist can edit. Falls back to a no-op if the
  // wizard didn't pass goToStepById (defensive).
  const jump = (id) => () => { if (typeof goToStepById === 'function') goToStepById(id); };
  const FILE_TYPE_LABELS = isPublic ? FILE_TYPE_LABELS_PUBLIC : FILE_TYPE_LABELS_ADMIN;
  return (
    <div className="fw-step-content">
      <section className={sectionClass('personal')} onClick={jump('personal')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('personal')(); } }}>
        <h3>Личные данные <span className="fw-review-edit">изменить</span></h3>
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

      {!isPublic && (
        <section className="fw-review-section fw-review-clickable" onClick={jump('documents')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('documents')(); } }}>
          <h3>Внутренний паспорт РФ <span className="fw-review-edit">изменить</span></h3>
          <dl className="fw-review-dl">
            <dt>Серия</dt><dd>{fmt(payload.internal_series)}</dd>
            <dt>Номер</dt><dd>{fmt(payload.internal_number)}</dd>
            <dt>Дата выдачи</dt><dd>{fmt(payload.internal_issued_ru)}</dd>
            <dt>Кем выдан</dt><dd>{fmt(payload.internal_issued_by_ru)}</dd>
          </dl>
        </section>
      )}

      <section className={sectionClass('foreign')} onClick={jump('foreign')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('foreign')(); } }}>
        <h3>Загранпаспорт <span className="fw-review-edit">изменить</span></h3>
        <dl className="fw-review-dl">
          <dt>Номер</dt><dd>{fmt(payload.passport_number)}</dd>
          <dt>Тип паспорта</dt><dd>{fmt(payload.passport_type_ru)}</dd>
          <dt>Дата выдачи</dt><dd>{fmt(payload.issue_date)}</dd>
          <dt>Дата окончания</dt><dd>{fmt(payload.expiry_date)}</dd>
          <dt>Кем выдан</dt><dd>{fmt(payload.issued_by_ru)}</dd>
        </dl>
      </section>

      <section className={sectionClass('addresses')} onClick={jump('addresses')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('addresses')(); } }}>
        <h3>Адреса <span className="fw-review-edit">изменить</span></h3>
        <dl className="fw-review-dl">
          <dt>Адрес регистрации</dt><dd>{fmt(payload.reg_address_ru)}</dd>
          <dt>Домашний адрес</dt><dd>{fmt(payload.home_address_ru)}</dd>
        </dl>
      </section>

      <section className={sectionClass('occupation')} onClick={jump('occupation')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('occupation')(); } }}>
        <h3>Работа <span className="fw-review-edit">изменить</span></h3>
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

      <section className={sectionClass('travel')} onClick={jump('travel')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('travel')(); } }}>
        <h3>Поездка <span className="fw-review-edit">изменить</span></h3>
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

      <section className={sectionClass('documents')} onClick={jump('documents')} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); jump('documents')(); } }}>
        <h3>Прикреплённые файлы <span className="fw-review-edit">изменить</span></h3>
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

      {/* SmartCaptcha — public form only, and only when a build-time
          site key is present. The actual submit button lives in
          FormWizard's bottom action bar; the widget sits right above it
          on the visible page so it's the last thing the tourist
          interacts with before pressing "Отправить анкету". */}
      {isPublic && siteKey && (
        <section className="fw-captcha-section">
          <div className="sf-hint" style={{ marginBottom: 8 }}>
            Подтвердите, что вы не робот:
          </div>
          <SmartCaptcha
            siteKey={siteKey}
            onToken={setCaptchaToken}
            resetSignal={captchaResetSignal}
          />
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
