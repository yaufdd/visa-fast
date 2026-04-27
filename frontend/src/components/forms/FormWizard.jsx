// FormWizard — multi-step submission form. Owns the full payload, the
// current step index, errors per field, and the file attachment metadata
// keyed by file_type. Each step component is lightweight: it renders the
// inputs and exports a validate(payload) sibling. The wizard runs the
// active step's validator on Next; on the final step it runs every
// validator and jumps back to the first one that fails.
//
// API access is abstracted through the `adapter` prop (see
// publicWizardAdapter / adminWizardAdapter). The wizard never imports
// api/files or api/client directly so the same component can drive both
// the public /form/<slug> page and the manager-side /submissions/<id>
// page.
//
// localStorage draft persistence is opt-in via `adapter.persistEnabled`:
//   - public mode → true (tourist may close the tab mid-anketa)
//   - admin  mode → false (server is the source of truth; persisting per
//                   browser would cross-contaminate manager sessions)
// The persistence key is namespaced by the optional `persistKey` prop
// (the slug, in public mode); admin mode passes nothing and short-
// circuits the persistence layer entirely.

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
  // had_other_name is the Yes/No toggle that gates the maiden_name_ru
  // text input on PersonalStep. Default is "Нет" — the common case —
  // so the text input stays hidden until the tourist explicitly says
  // they had a previous surname.
  had_other_name: 'Нет',
  // Nationality dropdown defaults — see PersonalStep.NATIONALITY_PRESETS.
  // nationality_choice is UI-only state; nationality_ru is the field the
  // backend reads, kept in sync by PersonalStep when the dropdown
  // changes. The "Россия" default mirrors the most common case.
  nationality_choice: 'Россия',
  nationality_ru: 'Россия',
  // Former nationality dropdown defaults — Нет / СССР / Другое.
  // former_nationality_choice is UI-only state; former_nationality_ru is
  // the field the backend reads (kept in sync by PersonalStep). Default
  // "Нет" matches the common case. The assembler's
  // ComputeFormerNationality recognises only "СССР" / "Soviet" / "USSR"
  // patterns — anything from the "Другое" branch falls through to the
  // place-of-birth fallback or "NO". That's accepted.
  former_nationality_choice: 'Нет',
  former_nationality_ru: 'Нет',
};

// All fields the wizard touches. `same_address` is wizard-only state — the
// backend ignores extra keys in the payload JSON, so persisting it inline
// is fine.
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
  'same_address',
];

export default function FormWizard({
  adapter,
  onSubmit,
  initialPayload = {},
  initialFiles = null,
  initialStep = 0,
  submissionId = null,
  // Namespace for the localStorage draft blob. Public mode passes the
  // slug; admin mode passes nothing (persistence is off).
  persistKey = null,
  // Show or hide the consent block on the Review step. Public mode and
  // "create new" admin mode set this to true; "edit existing" admin
  // mode hides it because the consent stamp from the original tourist
  // submission is preserved on the row.
  showConsent = true,
  // onRestoreSubmissionId — invoked once when the wizard finds a saved
  // submissionId in localStorage on mount. The parent (SubmissionFormPage)
  // uses this to skip its draft-allocation call so a reload doesn't orphan
  // the previous draft + its uploaded files. Only relevant in public mode.
  onRestoreSubmissionId = null,
  // onResetDraft — invoked when the tourist clicks "Начать заново" in the
  // restore banner. Lets the parent issue a fresh draft.
  onResetDraft = null,
}) {
  const persistEnabled = Boolean(adapter?.persistEnabled && persistKey);

  // Read the persisted blob exactly once at first render (public mode
  // only). We split the load out so the initial state for every piece of
  // wizard state can be seeded from it without an extra effect+setState
  // round-trip (which would briefly flash the empty form).
  const restoredBlob = useMemo(
    () => (persistEnabled ? loadWizardBlob(persistKey) : null),
    [persistEnabled, persistKey],
  );
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
    // Backwards-compat restore for the had_other_name toggle. Submissions
    // saved before the Yes/No control was added have neither the explicit
    // flag nor a clean blank surname — they often store a typed "Нет"
    // (which is exactly the bug the toggle prevents). The safe rule:
    //   * flag already set        → respect it (user picked something).
    //   * flag missing + non-empty maiden_name_ru → "Да" so the saved
    //     surname stays visible and the manager can fix it manually.
    //     This deliberately leaves a legacy literal "Нет" in the field
    //     untouched on first load — the assembler-side resolveMaidenName
    //     guard still produces "" so the PDF renders "NO" correctly. The
    //     manager can flip the toggle to "Нет" to drop the literal.
    //   * flag missing + empty maiden_name_ru → "Нет".
    if (!merged.had_other_name) {
      const hasMaiden = String(merged.maiden_name_ru || '').trim() !== '';
      merged.had_other_name = hasMaiden ? 'Да' : 'Нет';
    }
    // Restore nationality_choice from nationality_ru when missing (legacy
    // submissions saved before the dropdown existed). Match against the
    // preset list verbatim — the canonical strings agreed with the
    // assembler's countryISOMap. Anything else falls into "other" so the
    // user sees what was previously typed and can keep / edit it.
    if (!merged.nationality_choice) {
      const presets = ['Россия', 'Беларусь', 'Казахстан'];
      const ru = String(merged.nationality_ru || '').trim();
      if (presets.includes(ru)) {
        merged.nationality_choice = ru;
      } else if (ru) {
        merged.nationality_choice = 'other';
      } else {
        merged.nationality_choice = 'Россия';
        merged.nationality_ru = 'Россия';
      }
    }
    // Restore former_nationality_choice from former_nationality_ru when
    // missing. The dropdown options are Нет / СССР / Другое. Legacy
    // free-text values that aren't exactly "Нет" / "СССР" land on
    // "Другое" so the saved string survives — the user can keep, edit,
    // or replace it. The assembler's ComputeFormerNationality only acts
    // on "СССР" / "Soviet" / "USSR" patterns, so other free-text simply
    // falls through to its place-of-birth fallback.
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
  }, [initialPayload, restoredBlob]);

  const [payload, setPayload] = useState(initialState);
  const [files, setFiles] = useState(() => {
    const empty = {
      passport_internal: null,
      passport_foreign: null,
      ticket: null,
      voucher: null,
    };
    // Server-provided seeds (admin edit mode) take precedence over
    // localStorage. In public mode initialFiles is usually null and the
    // restoredBlob path runs.
    const seed = initialFiles || restoredBlob?.files || null;
    if (!seed) return empty;
    return {
      passport_internal: seed.passport_internal ?? null,
      passport_foreign: seed.passport_foreign ?? null,
      ticket: seed.ticket ?? null,
      voucher: seed.voucher ?? null,
    };
  });
  const [currentStep, setCurrentStep] = useState(() => {
    if (Number.isInteger(initialStep) && initialStep >= 0 && initialStep < STEPS.length) {
      // initialStep wins when explicitly provided; otherwise fall back to
      // the persisted blob.
      const restored = restoredBlob?.currentStep;
      if (initialStep > 0) return initialStep;
      if (Number.isInteger(restored) && restored >= 0 && restored < STEPS.length) {
        return restored;
      }
      return 0;
    }
    return 0;
  });
  const [errors, setErrors] = useState({});
  const [apiError, setApiError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [autoFillNotice, setAutoFillNotice] = useState('');
  const [showRestoreBanner, setShowRestoreBanner] = useState(wasRestored);

  // Consent — fetched once when the wizard mounts; rendered on the Review
  // step only when showConsent is true. In edit-existing mode the row
  // already carries a consent stamp, so re-asking would be confusing.
  const [consent, setConsent] = useState(null);
  const [consentLoading, setConsentLoading] = useState(true);
  // When consent is hidden we treat it as already-checked so the submit
  // button stays enabled.
  const [consentChecked, setConsentChecked] = useState(!showConsent);
  const [consentExpanded, setConsentExpanded] = useState(false);

  useEffect(() => {
    if (!showConsent) {
      setConsentLoading(false);
      return;
    }
    setConsentLoading(true);
    getConsentText()
      .then((data) => setConsent(data))
      .catch(() => setConsent({ version: '?', body: 'Не удалось загрузить текст согласия.' }))
      .finally(() => setConsentLoading(false));
  }, [showConsent]);

  // Drop the auto-fill toast after a few seconds so it doesn't accumulate.
  useEffect(() => {
    if (!autoFillNotice) return;
    const t = setTimeout(() => setAutoFillNotice(''), 4000);
    return () => clearTimeout(t);
  }, [autoFillNotice]);

  // Hand the restored submission id back to the parent ONCE so it can
  // skip its draft-allocation call. We do this in an effect (not at render
  // time) to avoid setState-during-render warnings in the parent.
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
  // tracked piece of state. Public mode only; admin mode skips entirely.
  useEffect(() => {
    if (!persistEnabled) return undefined;
    const t = setTimeout(() => {
      saveWizardBlob(persistKey, {
        payload,
        currentStep,
        files,
        submissionId,
      });
    }, 250);
    return () => clearTimeout(t);
  }, [persistEnabled, persistKey, payload, currentStep, files, submissionId]);

  // "Начать заново" — clears the persisted blob and resets every piece
  // of in-memory state to defaults. The parent is asked (via callback)
  // to issue a fresh draft so any uploads on the new draft land on a
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
    if (persistEnabled) clearWizardBlob(persistKey);
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
    setConsentChecked(!showConsent);
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
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleBack = () => {
    setApiError('');
    setErrors({});
    setCurrentStep((s) => Math.max(s - 1, 0));
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleJump = (i) => {
    if (i >= currentStep) return;
    setApiError('');
    setErrors({});
    setCurrentStep(i);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleSubmit = async () => {
    setApiError('');

    const finalPayload = applyOccupationAutoFill(payload);

    for (let i = 0; i < STEPS.length; i++) {
      const errs = STEPS[i].validate(finalPayload) || {};
      if (Object.keys(errs).length > 0) {
        setErrors(errs);
        setCurrentStep(i);
        scrollToFirstError(errs);
        return;
      }
    }

    if (showConsent && !consentChecked) {
      setApiError('Необходимо подтвердить согласие на обработку персональных данных.');
      return;
    }

    setSubmitting(true);
    try {
      await onSubmit(finalPayload, consentChecked);
      // Submission accepted — drop the persisted draft so a back-nav
      // starts fresh. Public mode only.
      if (persistEnabled) clearWizardBlob(persistKey);
    } catch (err) {
      setApiError(err?.message || 'Не удалось отправить анкету.');
    } finally {
      setSubmitting(false);
    }
  };

  const StepComponent = STEPS[currentStep].Component;
  const isFirst = currentStep === 0;
  const isLast = currentStep === STEPS.length - 1;

  // Step prop bag — every step receives the same shape. The adapter is
  // forwarded so passport / ticket / voucher upload widgets can drive
  // the right backend without the wizard switching on mode.
  const stepProps = {
    payload,
    setField,
    errors,
    files,
    setFiles,
    adapter,
    submissionId,
    setPayload,
    autoFillNotice,
    setAutoFillNotice,
    // Review step needs consent state.
    consent: showConsent ? consent : undefined,
    consentLoading,
    consentChecked,
    setConsentChecked,
    consentExpanded,
    setConsentExpanded,
  };

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
              disabled={submitting || (showConsent && !consentChecked)}
            >
              {submitting ? 'Отправка…' : 'Отправить анкету'}
            </button>
          )}
        </div>
      </main>
    </div>
  );
}
