import { useEffect, useMemo, useState } from 'react';
import { getConsentText } from '../api/client';
import { ruToLatICAO } from '../utils/translit';
import { dmyToIso, isoToDmy } from '../utils/dates';
import { normalizePhone, phoneOnInput } from '../utils/phone';
import BoxedCharInput from './BoxedCharInput';
import FileUploadField from './forms/FileUploadField';

// Allowed characters in "кем выдан" — Cyrillic/Latin letters, digits, spaces
// and common passport-code punctuation (dash, slash, dot, comma, №). No
// length limit.
const ISSUED_BY_SANITIZE = (s) =>
  s.replace(/[^a-zA-Zа-яА-ЯёЁ0-9 №.,/\-]/g, '');

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
  // had_other_name gates the maiden_name_ru text input. Default "Нет"
  // — the common case — keeps the text input hidden by default.
  had_other_name: 'Нет',
  // Nationality dropdown defaults — mirror the wizard. nationality_choice
  // is UI-only state; nationality_ru is what the backend reads.
  nationality_choice: 'Россия',
  nationality_ru: 'Россия',
  // Former nationality dropdown — Нет / СССР / Другое. The choice is
  // UI-only; former_nationality_ru is the persisted field.
  former_nationality_choice: 'Нет',
  former_nationality_ru: 'Нет',
};

// Nationality preset list — keys MUST match
// backend/internal/ai/mappings.go:countryISOMap so CountryISO() resolves.
const NATIONALITY_PRESETS = ['Россия', 'Беларусь', 'Казахстан'];

// All fields the form touches. Phone fields aren't yet in the backend
// assembler but are part of the payload JSONB we POST.
// occupation_type is form-only state — the backend doesn't read it,
// but persisting it in the JSONB lets a future load of the same
// submission restore the chosen category without guessing.
const ALL_FIELDS = [
  'name_cyr', 'name_lat', 'gender_ru', 'birth_date', 'marital_status_ru',
  'place_of_birth_ru', 'nationality_ru', 'nationality_choice',
  'former_nationality_ru', 'former_nationality_choice',
  'had_other_name', 'maiden_name_ru',
  'passport_number', 'passport_type_ru', 'issue_date', 'expiry_date', 'issued_by_ru',
  'internal_series', 'internal_number', 'internal_issued_ru', 'internal_issued_by_ru',
  'reg_address_ru', 'home_address_ru', 'phone',
  'occupation_type', 'occupation_ru', 'employer_ru', 'employer_address_ru', 'employer_phone',
  'been_to_japan_ru', 'previous_visits_ru', 'criminal_record_ru',
];

// Default occupation_type when nothing was previously saved.
const OCCUPATION_DEFAULT = 'employed';

// Categories where the user types nothing in the Работа section — the
// four employer fields are hidden and auto-filled at submit time.
const BLANK_OCCUPATION_TYPES = new Set(['ip', 'pensioner', 'housewife', 'unemployed']);

// Categories where the institution name / address / phone are still
// shown (with re-labelled placeholders) but occupation_ru itself is
// fixed.
const STUDENT_OCCUPATION_TYPES = new Set(['student', 'schoolchild']);

const OCCUPATION_OPTIONS = [
  { value: 'employed', label: 'Работаю по найму' },
  { value: 'ip', label: 'Индивидуальный предприниматель' },
  // Same employer-fields layout as 'employed' (LLC name + address +
  // phone, all user-typed). Only the title is pinned to "Владелец ООО"
  // by applyOccupationAutoFill below.
  { value: 'business_owner', label: 'Владелец ООО' },
  { value: 'pensioner', label: 'Пенсионер' },
  { value: 'housewife', label: 'Домохозяйка' },
  { value: 'unemployed', label: 'Безработный' },
  { value: 'student', label: 'Студент' },
  { value: 'schoolchild', label: 'Школьник' },
];

// applyOccupationAutoFill returns a new payload with the Работа section
// normalised to what the visa form expects. Called once at submit time
// — keystroke-time auto-fill would create write-loops with React state
// and stale text would also be unrecoverable when the user switches
// categories.
function applyOccupationAutoFill(payload) {
  const type = payload.occupation_type || OCCUPATION_DEFAULT;
  const dash = '—'; // em-dash, matches the convention in the visa anketa.
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
      // employer_ru / employer_address_ru / employer_phone left as the
      // user typed (LLC name / registered address / contact phone).
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
      // employer_ru / employer_address_ru / employer_phone left as the
      // user typed (institution name / address / phone).
      break;
    case 'schoolchild':
      out.occupation_ru = 'Школьник';
      break;
    case 'employed':
    default:
      // No auto-fill — leave whatever the user typed.
      break;
  }
  return out;
}

function sanitizeLatin(value) {
  return value.toUpperCase().replace(/[^A-Z\s]/g, '');
}

// Map a passport-parser response (ai.PassportFields, JSON-decoded) to the
// form payload shape. The `type` parameter selects between the internal
// (general-civil) field set and the foreign (travel) field set — these
// share the personal-data part (name, gender, birth date, place of birth)
// but diverge on which passport-specific fields are populated. We never
// overwrite a field the user has already typed; the caller is responsible
// for the merge.
//
// Returns:
//   { mapped: {field: value}, filled: ["field", ...] }
function mapPassportFieldsToPayload(fields, payload, type = 'internal') {
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
  // Backend gives us last/first/patronymic separately; rebuild the
  // single-string format the form uses ("Bamba Erik Sergeevich").
  const nameParts = [fields.last_name, fields.first_name, fields.patronymic]
    .map((s) => String(s ?? '').trim())
    .filter(Boolean);
  if (nameParts.length > 0) {
    const cyr = nameParts.join(' ');
    if (empty('name_cyr')) {
      mapped.name_cyr = cyr;
      filled.push('name_cyr');
      // Mirror the typing-time cyr→lat auto-fill: only fill name_lat if
      // it was also empty, otherwise the user's manual override survives.
      if (empty('name_lat')) {
        mapped.name_lat = ruToLatICAO(cyr);
        filled.push('name_lat');
      }
    }
  }
  // Foreign passports carry an MRZ-derived Latin name string. When present
  // it is the authoritative spelling printed on the data page — override
  // the ICAO best-guess we just produced (but still respect a manual user
  // entry: only overwrite if name_lat hasn't been set by the user before
  // this call). To check that we look at the *original* payload, not the
  // one we may have just mutated above.
  if (type === 'foreign' && fields.name_latin) {
    const lat = String(fields.name_latin).trim().toUpperCase();
    if (lat && empty('name_lat')) {
      mapped.name_lat = lat;
      if (!filled.includes('name_lat')) filled.push('name_lat');
    }
  }

  // ── Gender (shared) ──
  // Parser returns the MRZ-style "МУЖ"/"ЖЕН"; the form select uses
  // "Мужской"/"Женский" (see selectField('gender_ru', ...) below).
  if (fields.gender === 'МУЖ') set('gender_ru', 'Мужской');
  else if (fields.gender === 'ЖЕН') set('gender_ru', 'Женский');

  // ── Birth date / place of birth (shared) ──
  if (fields.birth_date) set('birth_date', isoToDmy(fields.birth_date));
  if (fields.place_of_birth) set('place_of_birth_ru', fields.place_of_birth);

  if (type === 'foreign') {
    // ── Foreign passport number ──
    // Parser returns the 9-character document number as printed; the
    // form stores it as a single string in passport_number (the segmented
    // input below splits it 2+7 visually).
    if (fields.number) {
      const num = String(fields.number).replace(/\s+/g, '');
      if (num.length === 9) set('passport_number', num);
    }
    // ── Foreign passport dates ──
    if (fields.issue_date) set('issue_date', isoToDmy(fields.issue_date));
    if (fields.expiry_date) set('expiry_date', isoToDmy(fields.expiry_date));
    // ── Issuing authority ──
    if (fields.issuing_authority) set('issued_by_ru', fields.issuing_authority);
    // department_code / reg_address are not present on a foreign passport.
  } else {
    // ── Internal passport: series + number ──
    // Strip whitespace defensively — the parser returns clean digits but
    // the OCR text it consumed sometimes has spaces.
    if (fields.series) {
      const series = String(fields.series).replace(/\s+/g, '');
      if (/^\d{4}$/.test(series)) set('internal_series', series);
    }
    if (fields.number) {
      const number = String(fields.number).replace(/\s+/g, '');
      if (/^\d{6}$/.test(number)) set('internal_number', number);
    }
    // ── Internal passport date / authority / reg address ──
    if (fields.issue_date) set('internal_issued_ru', isoToDmy(fields.issue_date));
    if (fields.issuing_authority) set('internal_issued_by_ru', fields.issuing_authority);
    if (fields.reg_address) set('reg_address_ru', fields.reg_address);
    // department_code has no form field in the public-form payload — ignored.
  }

  return { mapped, filled };
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
  // Студент / Школьник — institution name + address required, phone optional.
  // ip / pensioner / housewife / unemployed — auto-filled, no validation.
  // employed — no extra rules beyond what's already here.
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

export default function SubmissionForm({
  onSubmit,
  initialPayload = {},
  submitLabel = 'Отправить анкету',
  showConsent = true,
  // Phase 3 props — when both are set the form renders the public-form
  // file-upload widgets and includes the submission_id in the final POST
  // so the backend finalises the existing draft. If they're unset (e.g.
  // legacy "manager-creates-submission" call sites), the form behaves
  // exactly as it did before.
  slug = null,
  submissionId = null,
}) {
  const initialState = useMemo(() => {
    const base = {};
    for (const name of ALL_FIELDS) {
      base[name] = SELECT_DEFAULTS[name] ?? '';
    }
    const merged = { ...base, ...initialPayload };
    // One-time migration: existing submissions saved before
    // occupation_type existed will have it unset. If they used the old
    // single "ИП" checkbox flow (occupation_ru === "ИП"), restore the
    // ip category. Anything else defaults to employed.
    if (!merged.occupation_type) {
      const occRu = String(merged.occupation_ru || '').trim().toLowerCase();
      merged.occupation_type = occRu === 'ип' ? 'ip' : OCCUPATION_DEFAULT;
    }
    // Backwards-compat for the had_other_name toggle (added after some
    // submissions were already saved). When it's missing we infer it
    // from whether maiden_name_ru carries any text. A literal "Нет"
    // typed into the old free-text field will appear as "Да" + the
    // saved string here — that's intentional: the assembler-side
    // resolveMaidenName guard still produces "" for the PDF, and the
    // manager can flip the toggle to drop the literal manually.
    if (!merged.had_other_name) {
      const hasMaiden = String(merged.maiden_name_ru || '').trim() !== '';
      merged.had_other_name = hasMaiden ? 'Да' : 'Нет';
    }
    // Same restore rules as FormWizard — keep the two flat-and-wizard
    // forms in lockstep so a submission edited in either renders
    // identically.
    if (!merged.nationality_choice) {
      const ru = String(merged.nationality_ru || '').trim();
      if (NATIONALITY_PRESETS.includes(ru)) {
        merged.nationality_choice = ru;
      } else if (ru) {
        merged.nationality_choice = 'other';
      } else {
        merged.nationality_choice = 'Россия';
        merged.nationality_ru = 'Россия';
      }
    }
    // Restore former_nationality_choice from former_nationality_ru when
    // missing. Mirrors FormWizard. Legacy free-text values that aren't
    // exactly "Нет" / "СССР" land on "Другое" so the saved string survives.
    if (!merged.former_nationality_choice) {
      const ru = String(merged.former_nationality_ru || '').trim();
      if (ru === 'Нет' || ru === '') {
        merged.former_nationality_choice = 'Нет';
        merged.former_nationality_ru = 'Нет';
      } else if (ru === 'СССР') {
        merged.former_nationality_choice = 'СССР';
      } else {
        merged.former_nationality_choice = 'other';
      }
    }
    return merged;
  }, [initialPayload]);

  const [payload, setPayload] = useState(initialState);
  const [consentChecked, setConsentChecked] = useState(false);
  const [consent, setConsent] = useState(null);
  const [consentLoading, setConsentLoading] = useState(showConsent);
  const [consentExpanded, setConsentExpanded] = useState(false);
  const [errors, setErrors] = useState({});
  const [apiError, setApiError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  // Phase 3 — keyed by file_type. All four document slots are initialised
  // explicitly (default null) so the shape is uniform regardless of which
  // widgets the parent surfaces (slug-gated).
  const [files, setFiles] = useState({
    passport_internal: null,
    passport_foreign: null,
    ticket: null,
    voucher: null,
  });
  const [autoFillNotice, setAutoFillNotice] = useState('');

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

  // Cyrillic → Latin one-way auto-fill via ICAO Doc 9303 (the МВД
  // standard used on Russian foreign passports — deterministic,
  // matches what the passport shows exactly). Typing in the Latin
  // field is free-form for the rare case of a non-standard passport
  // spelling the tourist wants to override (e.g. "YULIA" vs "IULIIA").
  const handleCyrChange = (value) => {
    setPayload((p) => ({
      ...p,
      name_cyr: value,
      name_lat: ruToLatICAO(value),
    }));
    clearError('name_cyr');
    clearError('name_lat');
  };

  const handleLatChange = (value) => {
    setField('name_lat', sanitizeLatin(value));
  };

  const handleDateChange = (name) => (e) => {
    setField(name, isoToDmy(e.target.value));
  };

  const handlePhoneBlur = (name) => (e) => {
    const normalized = normalizePhone(e.target.value);
    if (normalized !== e.target.value) setField(name, normalized);
  };

  // Auto-fill from a /parse-passport response. Only empty fields are
  // touched — anything the user has already typed wins. We use the
  // functional setState form so the latest payload is read inside the
  // updater (avoids races against concurrent typing). `type` switches
  // between the internal-passport and foreign-passport field maps.
  const handlePassportAutoFill = (type) => (fields) => {
    setPayload((p) => {
      const { mapped, filled } = mapPassportFieldsToPayload(fields, p, type);
      if (filled.length === 0) {
        setAutoFillNotice('Все поля уже заполнены.');
        return p;
      }
      setAutoFillNotice(`Поля обновлены (${filled.length}).`);
      return { ...p, ...mapped };
    });
  };

  // Drop the toast after a few seconds so it doesn't accumulate visually.
  useEffect(() => {
    if (!autoFillNotice) return;
    const t = setTimeout(() => setAutoFillNotice(''), 4000);
    return () => clearTimeout(t);
  }, [autoFillNotice]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setApiError('');
    // Normalise the Работа section once at submit time. The on-screen
    // payload state is left as the user typed — applyOccupationAutoFill
    // produces the final shape the visa form expects (ИП / Пенсионер /
    // dashes / institution fields). Done before validate() so student
    // category validation sees the user-typed strings.
    const finalPayload = applyOccupationAutoFill(payload);
    const errs = validate(finalPayload);
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
      await onSubmit(finalPayload, consentChecked);
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

  // Same visual language as the flight number input — character boxes,
  // unlimited length. Used for "кем выдан" fields which often carry
  // mixed digits/Cyrillic punctuation.
  const boxedField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <BoxedCharInput
          value={payload[name] ?? ''}
          onChange={(v) => setField(name, v)}
          sanitize={ISSUED_BY_SANITIZE}
          ariaLabel={label}
        />
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
          onChange={(e) => setField(name, phoneOnInput(e.target.value))}
          onBlur={handlePhoneBlur(name)}
          autoComplete="off"
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  // Segmented passport number: 2 digits + 7 digits = 9 total.
  // Stores the concatenated 9-digit string in payload.passport_number.
  const passportNumberField = () => {
    const err = errors.passport_number;
    const raw = (payload.passport_number || '').replace(/\D/g, '').slice(0, 9);
    const part1 = raw.slice(0, 2);
    const part2 = raw.slice(2, 9);

    const setCombined = (p1, p2) => {
      setField('passport_number', (p1 + p2).slice(0, 9));
    };

    const onFirst = (e) => {
      const v = e.target.value.replace(/\D/g, '').slice(0, 2);
      setCombined(v, part2);
      if (v.length === 2) {
        const next = document.getElementById('passport-part2');
        if (next) next.focus();
      }
    };

    const onSecond = (e) => {
      const v = e.target.value.replace(/\D/g, '').slice(0, 7);
      setCombined(part1, v);
    };

    const onSecondKey = (e) => {
      if (e.key === 'Backspace' && !part2) {
        const prev = document.getElementById('passport-part1');
        if (prev) prev.focus();
      }
    };

    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field="passport_number">
        <span className="sf-label">Номер загранпаспорта</span>
        <div className="sf-passport-input">
          <div className="sf-passport-col">
            <input
              id="passport-part1"
              type="text"
              inputMode="numeric"
              value={part1}
              onChange={onFirst}
              maxLength={2}
              className="sf-passport-seg sf-passport-seg--short"
              autoComplete="off"
            />
            <span className="sf-passport-sublabel">Серия</span>
          </div>
          <span className="sf-passport-sep">№</span>
          <div className="sf-passport-col">
            <input
              id="passport-part2"
              type="text"
              inputMode="numeric"
              value={part2}
              onChange={onSecond}
              onKeyDown={onSecondKey}
              maxLength={7}
              className="sf-passport-seg sf-passport-seg--long"
              autoComplete="off"
            />
            <span className="sf-passport-sublabel">Номер</span>
          </div>
        </div>
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  // Segmented internal Russian passport: 4-digit series + 6-digit number,
  // stored as separate fields internal_series / internal_number.
  const internalPassportField = () => {
    const serErr = errors.internal_series;
    const numErr = errors.internal_number;
    const err = serErr || numErr;
    const series = (payload.internal_series || '').replace(/\D/g, '').slice(0, 4);
    const number = (payload.internal_number || '').replace(/\D/g, '').slice(0, 6);

    const onSeries = (e) => {
      const v = e.target.value.replace(/\D/g, '').slice(0, 4);
      setField('internal_series', v);
      if (v.length === 4) {
        const next = document.getElementById('internal-number');
        if (next) next.focus();
      }
    };

    const onNumber = (e) => {
      const v = e.target.value.replace(/\D/g, '').slice(0, 6);
      setField('internal_number', v);
    };

    const onNumberKey = (e) => {
      if (e.key === 'Backspace' && !number) {
        const prev = document.getElementById('internal-series');
        if (prev) prev.focus();
      }
    };

    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field="internal_series">
        <span className="sf-label">Серия и номер</span>
        <div className="sf-passport-input">
          <div className="sf-passport-col">
            <input
              id="internal-series"
              type="text"
              inputMode="numeric"
              value={series}
              onChange={onSeries}
              maxLength={4}
              className="sf-passport-seg sf-passport-seg--series"
              autoComplete="off"
            />
            <span className="sf-passport-sublabel">Серия</span>
          </div>
          <span className="sf-passport-sep">№</span>
          <div className="sf-passport-col">
            <input
              id="internal-number"
              type="text"
              inputMode="numeric"
              value={number}
              onChange={onNumber}
              onKeyDown={onNumberKey}
              maxLength={6}
              className="sf-passport-seg sf-passport-seg--number"
              autoComplete="off"
            />
            <span className="sf-passport-sublabel">Номер</span>
          </div>
        </div>
        {serErr && <span className="sf-error">{serErr}</span>}
        {numErr && !serErr && <span className="sf-error">{numErr}</span>}
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
          authoritative field the backend reads is nationality_ru. We
          mirror the wizard's PersonalStep logic in handleNationalityChoice
          below. */}
      <label className="sf-field" data-field="nationality_choice">
        <span className="sf-label">Гражданство</span>
        <select
          value={payload.nationality_choice ?? 'Россия'}
          onChange={(e) => {
            const next = e.target.value;
            setPayload((p) => ({
              ...p,
              nationality_choice: next,
              nationality_ru: next === 'other' ? '' : next,
            }));
            clearError('nationality_choice');
            clearError('nationality_ru');
          }}
        >
          {NATIONALITY_PRESETS.map((v) => (
            <option key={v} value={v}>{v}</option>
          ))}
          <option value="other">Другое (указать)</option>
        </select>
      </label>

      {payload.nationality_choice === 'other'
        && textField('nationality_ru', 'Гражданство (введите страну)')}

      {/* Former nationality dropdown — Нет / СССР / Другое. The choice
          is UI-only state; the authoritative field the backend reads is
          former_nationality_ru. */}
      <div className="sf-hint" style={{ marginBottom: 6 }}>
        Если вы родились в СССР, выберите «СССР».
      </div>
      <label className="sf-field" data-field="former_nationality_choice">
        <span className="sf-label">Прежнее гражданство</span>
        <select
          value={payload.former_nationality_choice ?? 'Нет'}
          onChange={(e) => {
            const next = e.target.value;
            setPayload((p) => ({
              ...p,
              former_nationality_choice: next,
              former_nationality_ru: next === 'other' ? '' : next,
            }));
            clearError('former_nationality_choice');
            clearError('former_nationality_ru');
          }}
        >
          <option value="Нет">Нет</option>
          <option value="СССР">СССР</option>
          <option value="other">Другое (указать)</option>
        </select>
      </label>

      {payload.former_nationality_choice === 'other'
        && textField('former_nationality_ru', 'Прежнее гражданство (введите страну)')}

      {/* Yes/No toggle (replaces the old free-text trap where typing
          "Нет" became "NET" in the visa anketa PDF). Switching to "Нет"
          also clears any previously typed maiden_name_ru so a stale
          surname does not ship to the PDF. */}
      <label className="sf-field" data-field="had_other_name">
        <span className="sf-label">Была ли другая фамилия?</span>
        <select
          value={payload.had_other_name ?? 'Нет'}
          onChange={(e) => {
            const next = e.target.value;
            setPayload((p) => ({
              ...p,
              had_other_name: next,
              ...(next === 'Да' ? {} : { maiden_name_ru: '' }),
            }));
            clearError('had_other_name');
            clearError('maiden_name_ru');
          }}
        >
          <option value="Нет">Нет</option>
          <option value="Да">Да</option>
        </select>
      </label>

      {payload.had_other_name === 'Да' && textField('maiden_name_ru', 'Какая фамилия была раньше?')}

      {/* Phone — moved here from "Контакты" to mirror the wizard's
          PersonalStep. Belongs with the tourist's personal contact
          details, not with the address block. */}
      {phoneField('phone', 'Телефон')}

      <h2 className="sf-heading">Загранпаспорт</h2>

      {passportNumberField()}
      {selectField('passport_type_ru', 'Тип паспорта', [
        { value: 'Обычный', label: 'Обычный' },
        { value: 'Дипломатический', label: 'Дипломатический' },
        { value: 'Служебный', label: 'Служебный' },
      ])}
      {dateField('issue_date', 'Дата выдачи')}
      {dateField('expiry_date', 'Дата окончания')}
      {boxedField('issued_by_ru', 'Кем выдан')}

      {/* Foreign passport upload — placed AFTER manual fields so the
          tourist who already typed everything still sees a clear
          "or upload a scan" affordance. Auto-fill targets the same
          загранпаспорт fields above plus name / gender / birth date. */}
      {slug && (
        <FileUploadField
          label="Скан загранпаспорта (необязательно)"
          fileType="passport_foreign"
          slug={slug}
          submissionId={submissionId}
          currentFile={files.passport_foreign || null}
          onUploaded={(meta) => setFiles((f) => ({ ...f, passport_foreign: meta }))}
          onDeleted={() => setFiles((f) => {
            const next = { ...f };
            delete next.passport_foreign;
            return next;
          })}
          onAutoFill={handlePassportAutoFill('foreign')}
          parseType="foreign"
          acceptMime="application/pdf,image/jpeg,image/png"
        />
      )}

      <h2 className="sf-heading">Внутренний паспорт РФ</h2>

      {internalPassportField()}
      {dateField('internal_issued_ru', 'Дата выдачи')}
      {boxedField('internal_issued_by_ru', 'Кем выдан')}
      {textareaField('reg_address_ru', 'Адрес регистрации')}

      {/* Phase 3 — show the upload widget only when the parent supplied
          a slug. Without a draft id the widget renders disabled (handled
          inside FileUploadField). The widget is intentionally placed
          AFTER the manual fields so a tourist who already typed
          everything sees a clear "or just upload a scan" affordance. */}
      {slug && (
        <FileUploadField
          label="Скан внутреннего паспорта (необязательно)"
          fileType="passport_internal"
          slug={slug}
          submissionId={submissionId}
          currentFile={files.passport_internal || null}
          onUploaded={(meta) => setFiles((f) => ({ ...f, passport_internal: meta }))}
          onDeleted={() => setFiles((f) => {
            const next = { ...f };
            delete next.passport_internal;
            return next;
          })}
          onAutoFill={handlePassportAutoFill('internal')}
          parseType="internal"
          acceptMime="application/pdf,image/jpeg,image/png"
        />
      )}
      {autoFillNotice && (
        <div className="sf-autofill-notice">{autoFillNotice}</div>
      )}

      <h2 className="sf-heading">Контакты</h2>

      {textareaField('home_address_ru', 'Домашний адрес')}

      <h2 className="sf-heading">Работа</h2>

      <label className="sf-field" data-field="occupation_type">
        <span className="sf-label">Род занятий</span>
        <select
          value={payload.occupation_type || OCCUPATION_DEFAULT}
          onChange={(e) => setField('occupation_type', e.target.value)}
        >
          {OCCUPATION_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </label>

      {(payload.occupation_type || OCCUPATION_DEFAULT) === 'employed' && (
        <>
          {textField('occupation_ru', 'Должность')}
          {textField('employer_ru', 'Название организации')}
          {textareaField('employer_address_ru', 'Адрес организации')}
          {phoneField('employer_phone', 'Телефон организации')}
        </>
      )}

      {/* Владелец ООО — same employer fields as 'employed', but the
          title is auto-pinned to "Владелец ООО" so we hide the input. */}
      {(payload.occupation_type || OCCUPATION_DEFAULT) === 'business_owner' && (
        <>
          {textField('employer_ru', 'Название организации')}
          {textareaField('employer_address_ru', 'Адрес организации')}
          {phoneField('employer_phone', 'Телефон организации')}
        </>
      )}

      {STUDENT_OCCUPATION_TYPES.has(payload.occupation_type || OCCUPATION_DEFAULT) && (
        <>
          {textField('employer_ru', 'Название учебного заведения')}
          {textareaField('employer_address_ru', 'Адрес учебного заведения')}
          {phoneField('employer_phone', 'Телефон учебного заведения')}
        </>
      )}

      {/* ip / pensioner / housewife / unemployed — fields hidden,
          values filled in at submit by applyOccupationAutoFill. */}
      {BLANK_OCCUPATION_TYPES.has(payload.occupation_type) && (
        <p className="sf-hint sf-occupation-note">
          Поля раздела «Работа» заполнятся автоматически.
        </p>
      )}

      {slug && (
        <>
          <h2 className="sf-heading">Документы поездки</h2>

          {/* Ticket and voucher uploads — no Распознать button. Managers
              run the parsers when attaching the submission to a group, so
              we just persist the bytes here. */}
          <FileUploadField
            label="Авиабилет(ы)"
            fileType="ticket"
            slug={slug}
            submissionId={submissionId}
            currentFile={files.ticket || null}
            onUploaded={(meta) => setFiles((f) => ({ ...f, ticket: meta }))}
            onDeleted={() => setFiles((f) => {
              const next = { ...f };
              delete next.ticket;
              return next;
            })}
            acceptMime="application/pdf,image/jpeg,image/png"
          />
          <span className="sf-hint">
            Менеджер распознает скан автоматически после прикрепления к туру.
          </span>

          <FileUploadField
            label="Ваучер на отель(и)"
            fileType="voucher"
            slug={slug}
            submissionId={submissionId}
            currentFile={files.voucher || null}
            onUploaded={(meta) => setFiles((f) => ({ ...f, voucher: meta }))}
            onDeleted={() => setFiles((f) => {
              const next = { ...f };
              delete next.voucher;
              return next;
            })}
            acceptMime="application/pdf,image/jpeg,image/png"
          />
          <span className="sf-hint">
            Менеджер распознает скан автоматически после прикрепления к туру.
          </span>
        </>
      )}

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

        .sf-occupation-note {
          margin: 0;
          padding: 8px 12px;
          background: var(--gray-dark);
          border: 1px dashed var(--border);
          border-radius: 6px;
        }

        .sf-autofill-notice {
          font-size: 12px;
          color: var(--accent);
          background: var(--accent-dim);
          border: 1px solid var(--accent);
          padding: 6px 10px;
          border-radius: 6px;
        }

        .sf-checkbox-row {
          display: flex;
          align-items: center;
          gap: 10px;
          font-size: 13px;
          color: var(--white);
          cursor: pointer;
          user-select: none;
          padding: 6px 0;
        }

        .sf-checkbox-row input[type="checkbox"] {
          accent-color: var(--accent);
          width: 16px;
          height: 16px;
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
