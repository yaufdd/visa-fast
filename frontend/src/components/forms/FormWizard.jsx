// FormWizard — multi-step public submission form. Owns the full payload,
// the current step index, errors per field, and the file attachment metadata
// keyed by file_type. Each step component is lightweight: it renders the
// inputs and exports a validate(payload) sibling. The wizard runs the
// active step's validator on Next; on the final step it runs every
// validator and jumps back to the first one that fails.

import { useEffect, useMemo, useState } from 'react';
import { getConsentText } from '../../api/client';
import StepSidebar from './StepSidebar';
import './wizard.css';

import PersonalStep, { validate as validatePersonal } from './steps/PersonalStep';
import InternalPassportStep, { validate as validateInternal } from './steps/InternalPassportStep';
import ForeignPassportStep, { validate as validateForeign } from './steps/ForeignPassportStep';
import AddressesStep, { validate as validateAddresses } from './steps/AddressesStep';
import OccupationStep, { validate as validateOccupation, applyOccupationAutoFill, OCCUPATION_DEFAULT } from './steps/OccupationStep';
import TravelDocsStep, { validate as validateTravel } from './steps/TravelDocsStep';
import ReviewStep, { validate as validateReview } from './steps/ReviewStep';

// Step registry — order matters: this is the order shown in the sidebar
// and the order `Next` walks through.
const STEPS = [
  { id: 'personal',  label: 'Личные данные',         Component: PersonalStep,         validate: validatePersonal  },
  { id: 'internal',  label: 'Внутренний паспорт',    Component: InternalPassportStep, validate: validateInternal  },
  { id: 'foreign',   label: 'Загранпаспорт',         Component: ForeignPassportStep,  validate: validateForeign   },
  { id: 'addresses', label: 'Адреса',                Component: AddressesStep,        validate: validateAddresses },
  { id: 'occupation',label: 'Работа',                Component: OccupationStep,       validate: validateOccupation},
  { id: 'travel',    label: 'Поездка и документы',   Component: TravelDocsStep,       validate: validateTravel    },
  { id: 'review',    label: 'Проверка и отправка',   Component: ReviewStep,           validate: validateReview    },
];

// Defaults for selects that should not start empty.
const SELECT_DEFAULTS = {
  passport_type_ru: 'Обычный',
  been_to_japan_ru: 'Нет',
  criminal_record_ru: 'Нет',
  gender_ru: '',
  marital_status_ru: '',
};

// All fields the wizard touches. `same_address` is wizard-only state — the
// backend ignores extra keys in the payload JSON, so persisting it inline
// is fine.
const ALL_FIELDS = [
  'name_cyr', 'name_lat', 'gender_ru', 'birth_date', 'marital_status_ru',
  'place_of_birth_ru', 'nationality_ru', 'former_nationality_ru', 'maiden_name_ru',
  'passport_number', 'passport_type_ru', 'issue_date', 'expiry_date', 'issued_by_ru',
  'internal_series', 'internal_number', 'internal_issued_ru', 'internal_issued_by_ru',
  'reg_address_ru', 'home_address_ru', 'phone',
  'occupation_type', 'occupation_ru', 'employer_ru', 'employer_address_ru', 'employer_phone',
  'been_to_japan_ru', 'previous_visits_ru', 'criminal_record_ru',
  'same_address',
];

export default function FormWizard({
  onSubmit,
  initialPayload = {},
  slug = null,
  submissionId = null,
}) {
  const initialState = useMemo(() => {
    const base = {};
    for (const name of ALL_FIELDS) {
      base[name] = SELECT_DEFAULTS[name] ?? (name === 'same_address' ? false : '');
    }
    const merged = { ...base, ...initialPayload };
    if (!merged.occupation_type) {
      const occRu = String(merged.occupation_ru || '').trim().toLowerCase();
      merged.occupation_type = occRu === 'ип' ? 'ip' : OCCUPATION_DEFAULT;
    }
    return merged;
  }, [initialPayload]);

  const [payload, setPayload] = useState(initialState);
  const [files, setFiles] = useState({
    passport_internal: null,
    passport_foreign: null,
    ticket: null,
    voucher: null,
  });
  const [currentStep, setCurrentStep] = useState(0);
  const [errors, setErrors] = useState({});
  const [apiError, setApiError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [autoFillNotice, setAutoFillNotice] = useState('');

  // Consent — fetched once when the wizard mounts; rendered on the Review
  // step. Loading state surfaces inside the consent block itself.
  const [consent, setConsent] = useState(null);
  const [consentLoading, setConsentLoading] = useState(true);
  const [consentChecked, setConsentChecked] = useState(false);
  const [consentExpanded, setConsentExpanded] = useState(false);

  useEffect(() => {
    setConsentLoading(true);
    getConsentText()
      .then((data) => setConsent(data))
      .catch(() => setConsent({ version: '?', body: 'Не удалось загрузить текст согласия.' }))
      .finally(() => setConsentLoading(false));
  }, []);

  // Drop the auto-fill toast after a few seconds so it doesn't accumulate.
  useEffect(() => {
    if (!autoFillNotice) return;
    const t = setTimeout(() => setAutoFillNotice(''), 4000);
    return () => clearTimeout(t);
  }, [autoFillNotice]);

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

  const scrollToFirstError = (errs) => {
    const first = Object.keys(errs)[0];
    if (!first) return;
    // requestAnimationFrame ensures the DOM has the .has-error class
    // applied before we look up the field.
    requestAnimationFrame(() => {
      const el = document.querySelector(`[data-field="${first}"]`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });
  };

  const handleNext = () => {
    setApiError('');
    const validator = STEPS[currentStep].validate;
    const errs = validator(payload) || {};
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      scrollToFirstError(errs);
      return;
    }
    setCurrentStep((s) => Math.min(s + 1, STEPS.length - 1));
    // Reset scroll to top of main content for a clean step transition.
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleBack = () => {
    setApiError('');
    setErrors({});
    setCurrentStep((s) => Math.max(s - 1, 0));
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleJump = (i) => {
    // Sidebar only allows jumps to past steps — guard here too in case
    // the prop wiring drifts.
    if (i >= currentStep) return;
    setApiError('');
    setErrors({});
    setCurrentStep(i);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleSubmit = async () => {
    setApiError('');

    // Apply the occupation auto-fill once at submit time (matches the
    // legacy SubmissionForm behaviour). The on-screen state is left as
    // typed; only the posted payload contains the normalised version.
    const finalPayload = applyOccupationAutoFill(payload);

    // Re-run every step's validator — a user might have navigated
    // backward and emptied a required field.
    for (let i = 0; i < STEPS.length; i++) {
      const errs = STEPS[i].validate(finalPayload) || {};
      if (Object.keys(errs).length > 0) {
        setErrors(errs);
        setCurrentStep(i);
        scrollToFirstError(errs);
        return;
      }
    }

    if (!consentChecked) {
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

  const StepComponent = STEPS[currentStep].Component;
  const isFirst = currentStep === 0;
  const isLast = currentStep === STEPS.length - 1;

  // Step prop bag — every step receives the same shape.
  const stepProps = {
    payload,
    setField,
    errors,
    files,
    setFiles,
    slug,
    submissionId,
    // Internal/Foreign passport steps need direct setPayload access for
    // the auto-fill merge; addresses + others ignore the rest. Keeping
    // every step's prop signature uniform avoids surprises.
    setPayload,
    autoFillNotice,
    setAutoFillNotice,
    // Review step needs consent state.
    consent,
    consentLoading,
    consentChecked,
    setConsentChecked,
    consentExpanded,
    setConsentExpanded,
  };

  return (
    <div className="fw-shell">
      <StepSidebar
        steps={STEPS.map(({ id, label }) => ({ id, label }))}
        currentStep={currentStep}
        onJump={handleJump}
      />

      <main className="fw-main">
        <h2 className="fw-step-title">{STEPS[currentStep].label}</h2>

        <StepComponent {...stepProps} />

        {apiError && <div className="fw-api-error">{apiError}</div>}

        <div className="fw-actions">
          {!isFirst && (
            <button type="button" className="fw-btn" onClick={handleBack} disabled={submitting}>
              ← Назад
            </button>
          )}
          <div className="fw-actions-spacer" />
          {!isLast && (
            <button type="button" className="fw-btn fw-btn-primary" onClick={handleNext}>
              Далее →
            </button>
          )}
          {isLast && (
            <button
              type="button"
              className="fw-btn fw-btn-primary"
              onClick={handleSubmit}
              disabled={submitting || !consentChecked}
            >
              {submitting ? 'Отправка…' : 'Отправить анкету'}
            </button>
          )}
        </div>
      </main>
    </div>
  );
}
