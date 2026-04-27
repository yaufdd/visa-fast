// FormWizard — multi-step public submission form. Owns the full payload,
// the current step index, errors per field, and the file attachment metadata
// keyed by file_type. Each step component is lightweight: it renders the
// inputs and exports a validate(payload) sibling. The wizard runs the
// active step's validator on Next; on the final step it runs every
// validator and jumps back to the first one that fails.

import { useEffect, useMemo, useRef, useState } from 'react';
import { getConsentText } from '../../api/client';
import StepSidebar from './StepSidebar';
import { loadWizardBlob, saveWizardBlob, clearWizardBlob } from './wizardPersistence';
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
  // onRestoreSubmissionId — invoked once when the wizard finds a saved
  // submissionId in localStorage on mount. The parent (SubmissionFormPage)
  // uses this to skip its `/start` call so a reload doesn't orphan the
  // previous draft + its uploaded files.
  onRestoreSubmissionId = null,
  // onResetDraft — invoked when the tourist clicks "Начать заново" in the
  // restore banner. Lets the parent issue a fresh `/start` for a new draft.
  onResetDraft = null,
}) {
  // Read the persisted blob exactly once at first render. We split the
  // load out so the initial state for every piece of wizard state can be
  // seeded from it without an extra effect+setState round-trip (which
  // would briefly flash the empty form).
  const restoredBlob = useMemo(() => loadWizardBlob(slug), [slug]);
  const wasRestored = Boolean(
    restoredBlob !== null
    && (
      Object.keys(restoredBlob.payload || {}).length > 0
      || (restoredBlob.currentStep ?? 0) > 0
      || restoredBlob.submissionId
      || restoredBlob.files
    ),
  );

  const initialState = useMemo(() => {
    const base = {};
    for (const name of ALL_FIELDS) {
      base[name] = SELECT_DEFAULTS[name] ?? (name === 'same_address' ? false : '');
    }
    // Schema drift: defaults fill missing keys, restored blob wins for
    // known keys, unknown keys from a future schema just ride along —
    // they won't break anything because the backend ignores extras.
    const merged = {
      ...base,
      ...initialPayload,
      ...(restoredBlob?.payload || {}),
    };
    if (!merged.occupation_type) {
      const occRu = String(merged.occupation_ru || '').trim().toLowerCase();
      merged.occupation_type = occRu === 'ип' ? 'ip' : OCCUPATION_DEFAULT;
    }
    return merged;
  }, [initialPayload, restoredBlob]);

  const [payload, setPayload] = useState(initialState);
  const [files, setFiles] = useState(() => {
    const empty = {
      passport_internal: null,
      passport_foreign: null,
      ticket: null,
      voucher: null,
    };
    if (!restoredBlob?.files) return empty;
    // Only carry over the four known keys; ignore anything else.
    return {
      passport_internal: restoredBlob.files.passport_internal ?? null,
      passport_foreign: restoredBlob.files.passport_foreign ?? null,
      ticket: restoredBlob.files.ticket ?? null,
      voucher: restoredBlob.files.voucher ?? null,
    };
  });
  const [currentStep, setCurrentStep] = useState(() => {
    const restored = restoredBlob?.currentStep;
    if (Number.isInteger(restored) && restored >= 0 && restored < STEPS.length) {
      return restored;
    }
    return 0;
  });
  const [errors, setErrors] = useState({});
  const [apiError, setApiError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [autoFillNotice, setAutoFillNotice] = useState('');
  const [showRestoreBanner, setShowRestoreBanner] = useState(wasRestored);

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

  // Hand the restored submission id back to the parent ONCE so it can
  // skip its `/start` call. We do this in an effect (not at render time)
  // to avoid setState-during-render warnings in the parent.
  const restoreNotifiedRef = useRef(false);
  useEffect(() => {
    if (restoreNotifiedRef.current) return;
    restoreNotifiedRef.current = true;
    if (restoredBlob?.submissionId && onRestoreSubmissionId) {
      onRestoreSubmissionId(restoredBlob.submissionId);
    }
  }, [restoredBlob, onRestoreSubmissionId]);

  // Auto-dismiss the restore banner after 5 s. The tourist can also
  // dismiss it manually via the close button.
  useEffect(() => {
    if (!showRestoreBanner) return;
    const t = setTimeout(() => setShowRestoreBanner(false), 5000);
    return () => clearTimeout(t);
  }, [showRestoreBanner]);

  // Debounced persistence — re-save 250 ms after the last change to any
  // tracked piece of state. The effect re-runs on every change, but
  // setTimeout + cleanup means only the LAST scheduled save fires; all
  // earlier ones are cancelled. Net result: one write per quiet 250 ms
  // window instead of one write per keystroke.
  useEffect(() => {
    if (!slug) return undefined;
    const t = setTimeout(() => {
      saveWizardBlob(slug, {
        payload,
        currentStep,
        files,
        submissionId,
      });
    }, 250);
    return () => clearTimeout(t);
  }, [slug, payload, currentStep, files, submissionId]);

  // "Начать заново" — clears the persisted blob and resets every piece
  // of in-memory state to defaults. The parent is asked (via callback)
  // to issue a fresh `/start` so any uploads on the new draft land on a
  // clean submission row.
  const buildDefaults = () => {
    const base = {};
    for (const name of ALL_FIELDS) {
      base[name] = SELECT_DEFAULTS[name] ?? (name === 'same_address' ? false : '');
    }
    base.occupation_type = OCCUPATION_DEFAULT;
    return base;
  };

  const handleResetDraft = () => {
    clearWizardBlob(slug);
    setPayload(buildDefaults());
    setFiles({
      passport_internal: null,
      passport_foreign: null,
      ticket: null,
      voucher: null,
    });
    setCurrentStep(0);
    setErrors({});
    setApiError('');
    setConsentChecked(false);
    setShowRestoreBanner(false);
    onResetDraft?.();
  };

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
      // Submission accepted — drop the persisted draft so a back-nav to
      // /form/<slug> starts fresh. Done synchronously after onSubmit so
      // a navigation away in the parent doesn't race the cleanup.
      clearWizardBlob(slug);
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

  // Mobile progress strip — sibling of the sidebar. CSS hides this on
  // desktop and hides the sidebar on mobile; the two are mutually
  // exclusive. Past steps remain reachable via the Back button on mobile;
  // a clickable past-step navigation would need a dropdown / sheet and
  // is deferred.
  const mobileProgressPct = ((currentStep + 1) / STEPS.length) * 100;

  return (
    <div className="fw-shell">
      <StepSidebar
        steps={STEPS.map(({ id, label }) => ({ id, label }))}
        currentStep={currentStep}
        onJump={handleJump}
      />

      <div className="wizard-mobile-progress" aria-hidden="true">
        <div className="wizard-mobile-progress__label">
          Шаг {currentStep + 1} из {STEPS.length}: {STEPS[currentStep].label}
        </div>
        <div className="wizard-mobile-progress__track">
          <div
            className="wizard-mobile-progress__fill"
            style={{ width: `${mobileProgressPct}%` }}
          />
        </div>
      </div>

      <main className="fw-main">
        {showRestoreBanner && (
          <div className="fw-restore-banner" role="status">
            <span className="fw-restore-banner-text">
              Восстановили незавершённую анкету.
            </span>
            <button
              type="button"
              className="fw-restore-banner-action"
              onClick={handleResetDraft}
            >
              Начать заново
            </button>
            <button
              type="button"
              className="fw-restore-banner-close"
              onClick={() => setShowRestoreBanner(false)}
              aria-label="Закрыть"
            >
              ×
            </button>
          </div>
        )}

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
