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
// Files are tracked in two flavours, controlled by `adapter.filesMode`:
//
//   'upload-now' (admin)
//     The wizard has a real submissionId from mount. Each pick fires an
//     immediate upload to the backend; the meta returned by the server
//     (id, original_name, size_bytes, mime_type, file_type) lands in
//     `files`. onSubmit just gets the payload — files are already
//     persisted server-side under the same row.
//
//   'upload-on-submit' (public)
//     No submissionId. Picks store synthetic meta records carrying
//     `_localFile: File` in `files`. On final submit the wizard hands
//     the entire `files` object to onSubmit so the adapter can pack it
//     into a multipart body.
//
// localStorage draft persistence is opt-in via `adapter.persistEnabled`:
//   - public mode → true (tourist may close the tab mid-anketa)
//   - admin  mode → false (server is the source of truth; persisting per
//                   browser would cross-contaminate manager sessions)
// The persistence key is namespaced by the optional `persistKey` prop
// (the slug, in public mode); admin mode passes nothing and short-
// circuits the persistence layer entirely.
//
// Public-mode persistence intentionally omits `files` and `submissionId`:
// File objects can't be serialised, and there is no draft row to rebind.
// On reload the typed text returns; selected files have to be picked
// again.

import { useEffect, useMemo, useRef, useState } from 'react';
import { getConsentText } from '../../api/client';
import StepSidebar from './StepSidebar';
import { loadWizardBlob, saveWizardBlob, clearWizardBlob } from './wizardPersistence';
import './wizard.css';

import PersonalStep, { validate as validatePersonal } from './steps/PersonalStep';
import ForeignPassportStep, { validate as validateForeign } from './steps/ForeignPassportStep';
import AddressesStep, { validate as validateAddresses } from './steps/AddressesStep';
import OccupationStep, { validate as validateOccupation, applyOccupationAutoFill, OCCUPATION_DEFAULT } from './steps/OccupationStep';
import TravelStep, { validate as validateTravel } from './steps/TravelStep';
import DocumentsStep, { validate as validateDocuments } from './steps/DocumentsStep';
import ReviewStep, { validate as validateReview } from './steps/ReviewStep';

// Step registry — order matters: this is the order shown in the sidebar
// and the order `Next` walks through.
//
// Both public (tourist) and admin (manager) wizards share the same
// layout: no dedicated "Внутренний паспорт" step (tourist uploads a scan;
// the manager runs recognition from the Документы step), and a clean
// split between "Поездка" (travel history) and "Документы" (uploads).
//
// The only mode-specific bit is the last step's label — tourist sees
// "Проверка и отправка" (matches the action — they submit), manager
// sees "Сохранение" (they save the row, no submission concept).
function buildSteps(isPublic) {
  return [
    { id: 'personal',  label: 'Личные данные',  Component: PersonalStep,        validate: validatePersonal   },
    { id: 'foreign',   label: 'Загранпаспорт',  Component: ForeignPassportStep, validate: validateForeign    },
    { id: 'addresses', label: 'Адреса',         Component: AddressesStep,       validate: validateAddresses  },
    { id: 'occupation',label: 'Работа',         Component: OccupationStep,      validate: validateOccupation },
    { id: 'travel',    label: 'Поездка',        Component: TravelStep,          validate: validateTravel     },
    { id: 'documents', label: 'Документы',      Component: DocumentsStep,       validate: validateDocuments  },
    {
      id: 'review',
      label: isPublic ? 'Проверка и отправка' : 'Сохранение',
      Component: ReviewStep,
      validate: validateReview,
    },
  ];
}

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
  // Former nationality dropdown defaults — intentionally LEFT EMPTY so
  // PersonalStep's birth_date watcher can suggest "СССР" / "Нет" on a
  // fresh form. The select's UI shows "Нет" via a `?? 'Нет'` fallback
  // until the watcher (or the user) sets a real value. The restore
  // guard for `_former_nat_user_set` keys off "non-empty" — using a
  // non-empty default here would trip the guard and block the watcher
  // on every fresh form load.
};

// All fields the wizard touches. `same_address` is wizard-only state — the
// backend ignores extra keys in the payload JSON, so persisting it inline
// is fine.
const ALL_FIELDS = [
  'name_cyr', 'name_lat', 'gender_ru', 'birth_date', 'marital_status_ru',
  'place_of_birth_ru', 'nationality_ru', 'nationality_choice',
  'former_nationality_ru', 'former_nationality_choice',
  // _former_nat_user_set — UI-only flag (leading underscore signals
  // "transient, ignored by the backend"). Tracks whether the tourist
  // has explicitly picked the former-nationality dropdown so the
  // birth_date auto-fill in PersonalStep stops overriding their
  // choice. Persisted in the localStorage draft + DB JSONB roundtrip
  // so the override survives reloads.
  '_former_nat_user_set',
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
}) {
  const persistEnabled = Boolean(adapter?.persistEnabled && persistKey);
  // 'upload-now' (admin) vs 'upload-on-submit' (public). Defaults to
  // 'upload-on-submit' if the adapter forgot to declare it — the safer
  // default for any new entry point because it avoids a server round-
  // trip per pick.
  const filesMode = adapter?.filesMode || 'upload-on-submit';
  // Step list — wrapped in useMemo so step navigation / validation refs
  // stay stable across renders. Only the last step's label depends on
  // adapter.isPublic right now.
  const STEPS = useMemo(() => buildSteps(Boolean(adapter?.isPublic)), [adapter]);

  // Read the persisted blob exactly once at first render (public mode
  // only). We split the load out so the initial state for every piece of
  // wizard state can be seeded from it without an extra effect+setState
  // round-trip (which would briefly flash the empty form).
  //
  // The blob no longer carries `files` or `submissionId` — see the file
  // header comment for why. A v1 blob written by an older build will
  // still load (those fields are simply ignored).
  const restoredBlob = useMemo(
    () => (persistEnabled ? loadWizardBlob(persistKey) : null),
    [persistEnabled, persistKey],
  );
  const wasRestored = Boolean(
    restoredBlob !== null
    && (
      Object.keys(restoredBlob.payload || {}).length > 0
      || (restoredBlob.currentStep ?? 0) > 0
    ),
  );

  const initialState = useMemo(() => {
    const base = {};
    for (const name of ALL_FIELDS) {
      if (name === 'same_address') {
        base[name] = false;
      } else if (name === '_former_nat_user_set') {
        base[name] = false;
      } else {
        base[name] = SELECT_DEFAULTS[name] ?? '';
      }
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
      if (ru === 'Нет') {
        merged.former_nationality_choice = 'Нет';
      } else if (ru === '') {
        // Empty → fresh form. Show "Нет" in the dropdown via the JSX's
        // `?? 'Нет'` fallback but leave former_nationality_ru EMPTY so
        // PersonalStep's birth_date watcher can suggest "СССР"/"Нет"
        // without tripping the _former_nat_user_set guard below.
        merged.former_nationality_choice = 'Нет';
      } else if (ru === 'СССР') {
        merged.former_nationality_choice = 'СССР';
      } else {
        merged.former_nationality_choice = 'other';
      }
    }
    // Restore rule for _former_nat_user_set: any non-empty saved
    // former_nationality_ru means a previous explicit choice was made
    // (by the tourist or a manager editing their submission). Seed the
    // flag so PersonalStep's birth_date watcher doesn't auto-override
    // on first mount. The empty-string and the literal "" cases are
    // also fine — those leave the watcher free to suggest СССР if the
    // birth date warrants it.
    if (!merged._former_nat_user_set
        && String(merged.former_nationality_ru || '').trim() !== '') {
      merged._former_nat_user_set = true;
    }
    return merged;
  }, [initialPayload, restoredBlob]);

  const [payload, setPayload] = useState(initialState);
  // Files state. Admin path receives `initialFiles` from the parent
  // (server-loaded list); public path always starts empty — there are
  // no File refs to restore from localStorage.
  const [files, setFiles] = useState(() => {
    const empty = {
      passport_internal: [],
      // Public-mode split: main page and registration page(s) are tracked
      // separately so DocumentsStep can show two distinct upload slots.
      // collectLocalFiles() merges both into passport_internal at submit.
      passport_main: [],
      passport_reg: [],
      passport_foreign: null,
      ticket: [],
      voucher: [],
    };
    if (!initialFiles) return empty;
    // Backwards compat: ticket/voucher used to be single objects (one per
    // submission). After 000023 they are arrays. passport_internal joined
    // the array types in 000024. Promote any legacy single-object seed
    // into a one-element array so the rest of the wizard sees a uniform
    // shape.
    const toArr = (v) => {
      if (!v) return [];
      if (Array.isArray(v)) return v;
      return [v];
    };
    return {
      passport_internal: [],
      // Existing passport files (admin loading a saved submission) land in
      // passport_main — they can't be distinguished by page type after the
      // fact, so the first slot is the best home. The manager can move files
      // by deleting and re-uploading into the correct slot.
      passport_main: toArr(initialFiles.passport_internal),
      passport_reg: [],
      passport_foreign: initialFiles.passport_foreign ?? null,
      ticket: toArr(initialFiles.ticket),
      voucher: toArr(initialFiles.voucher),
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
  // SmartCaptcha state. Only used when the wizard is in public mode AND
  // a build-time site key is present (see siteKey below). The token is
  // single-use, so we bump captchaResetSignal after every failed submit
  // attempt to refresh the widget for a retry. Both pieces of state
  // are no-ops when captcha is disabled.
  const [captchaToken, setCaptchaToken] = useState('');
  const [captchaResetSignal, setCaptchaResetSignal] = useState(0);
  // SmartCaptcha public site key, baked in at build time via Vite. Empty
  // string → captcha is soft-disabled on this build (no widget rendered,
  // no token sent, backend treats it as "captcha off" provided its own
  // server secret is also unset).
  const siteKey = (import.meta.env && import.meta.env.VITE_YANDEX_CAPTCHA_SITE_KEY) || '';
  // Server-reported missing fields (set when the backend returns the
  // "missing fields" 400 with a `missing` array). Used by ReviewStep to
  // glow the relevant section so the tourist can find what's empty.
  const [missingFields, setMissingFields] = useState([]);
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

  // Auto-dismiss the restore banner after 5 s. The tourist can also
  // dismiss it manually via the close button.
  useEffect(() => {
    if (!showRestoreBanner) return;
    const t = setTimeout(() => setShowRestoreBanner(false), 5000);
    return () => clearTimeout(t);
  }, [showRestoreBanner]);

  // Debounced persistence — re-save 250 ms after the last change to
  // payload / step. Public mode only; admin mode skips entirely. Files
  // and submissionId are NOT persisted (see file header comment).
  useEffect(() => {
    if (!persistEnabled) return undefined;
    const t = setTimeout(() => {
      saveWizardBlob(persistKey, {
        payload,
        currentStep,
      });
    }, 250);
    return () => clearTimeout(t);
  }, [persistEnabled, persistKey, payload, currentStep]);

  // "Начать заново" — clears the persisted blob and resets every piece
  // of in-memory state to defaults.
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
      passport_internal: [],
      passport_main: [],
      passport_reg: [],
      passport_foreign: null,
      ticket: [],
      voucher: [],
    });
    setCurrentStep(0);
    setErrors({});
    setApiError('');
    setConsentChecked(!showConsent);
    setShowRestoreBanner(false);
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
    // Per-step validation deliberately does NOT fire here — we let the
    // tourist navigate freely between steps. All required-field checks
    // run on the Submit attempt at the end (handleSubmit), which jumps
    // the wizard to the first step that has an error and highlights the
    // empty fields. This keeps the wizard's "Далее" feeling responsive
    // and unrestricted while still guaranteeing a valid payload reaches
    // the backend.
    setApiError('');
    setErrors({});
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
    if (i === currentStep) return;
    setApiError('');
    setErrors({});
    setCurrentStep(i);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  // Strip locally-collected `_localFile` references out of the meta we
  // hand to onSubmit's first arg path, etc. We need the real File refs
  // when the public adapter packs the multipart body — extract them
  // here into a {category: File[] | File | null} shape.
  const collectLocalFiles = () => {
    const fileOrNull = (v) => (v && v._localFile) || null;
    const arrFiles = (arr) =>
      (Array.isArray(arr) ? arr : [])
        .map((m) => (m && m._localFile) || null)
        .filter(Boolean);
    return {
      // Merge public-mode split slots (passport_main + passport_reg) with
      // any admin-mode passport_internal files so publicSubmit receives a
      // single flat array under the passport_internal multipart field name.
      passport_internal: [
        ...arrFiles(files.passport_internal),
        ...arrFiles(files.passport_main),
        ...arrFiles(files.passport_reg),
      ],
      passport_foreign: fileOrNull(files.passport_foreign),
      ticket: arrFiles(files.ticket),
      voucher: arrFiles(files.voucher),
    };
  };

  const handleSubmit = async () => {
    setApiError('');
    setMissingFields([]);

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

    // Captcha gate: only meaningful when public-mode AND a site key is
    // baked into the build. Order matches the spec: required fields →
    // consent → captcha → submit.
    const captchaActive = Boolean(adapter?.isPublic) && Boolean(siteKey);
    if (captchaActive && !captchaToken) {
      setApiError('Подтвердите, что вы не робот.');
      return;
    }

    setSubmitting(true);
    try {
      // In upload-on-submit mode (public) we hand the wizard's collected
      // File refs up so the adapter can build the multipart body. In
      // upload-now mode (admin) the files are already on the server, so
      // onSubmit doesn't need them — we still pass an empty bag so the
      // signature is uniform.
      const filesArg = filesMode === 'upload-on-submit'
        ? collectLocalFiles()
        : { passport_internal: [], passport_foreign: null, ticket: [], voucher: [] };
      // Public callers receive the captcha token as a 4th arg; admin
      // mode never reads it (no captcha rendered).
      await onSubmit(finalPayload, consentChecked, filesArg, captchaToken);
      // Submission accepted — drop the persisted draft so a back-nav
      // starts fresh. Public mode only.
      if (persistEnabled) clearWizardBlob(persistKey);
    } catch (err) {
      // If the server reported a list of missing keys, we surface them
      // via missingFields so ReviewStep can highlight the offending
      // sections — and we keep the message short and Russian here so
      // the toast under the action bar stays clean.
      if (Array.isArray(err?.missing) && err.missing.length > 0) {
        setMissingFields(err.missing);
        setApiError('Заполнены не все обязательные поля. Проверьте подсвеченные разделы.');
      } else {
        setApiError(err?.message || 'Не удалось отправить анкету.');
      }
      // Token is single-use even on a 409 — bump the reset signal so
      // the widget refreshes before the next attempt. This also covers
      // the backend's "Не удалось подтвердить, что вы не робот" case
      // where the user simply needs to press the challenge again.
      if (captchaActive) {
        setCaptchaToken('');
        setCaptchaResetSignal((n) => n + 1);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const StepComponent = STEPS[currentStep].Component;
  const isFirst = currentStep === 0;
  const isLast = currentStep === STEPS.length - 1;

  // Step prop bag — every step receives the same shape. The adapter is
  // forwarded so passport / ticket / voucher upload widgets can drive
  // the right backend without the wizard switching on mode. `filesMode`
  // tells the file widgets whether to upload-now or hold-and-ship.
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
    filesMode,
    // Review step uses this to jump back to a specific step when the
    // tourist clicks one of the summary section headers.
    goToStepById: (id) => {
      const idx = STEPS.findIndex((s) => s.id === id);
      if (idx >= 0) handleJump(idx);
    },
    // List of payload keys the server flagged as missing in the last
    // submit attempt. ReviewStep maps each key to the step that owns it
    // and glows the matching summary section.
    missingFields,
    // Review step needs consent state.
    consent: showConsent ? consent : undefined,
    consentLoading,
    consentChecked,
    setConsentChecked,
    consentExpanded,
    setConsentExpanded,
    // SmartCaptcha plumbing — ReviewStep renders the widget and pipes
    // the token back up via setCaptchaToken. captchaResetSignal lets us
    // refresh the widget after a failed submit (token is single-use).
    // siteKey doubles as the on/off switch: empty → no widget, captcha
    // soft-disabled.
    captchaToken,
    setCaptchaToken,
    captchaResetSignal,
    siteKey,
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
              disabled={
                submitting
                || (showConsent && !consentChecked)
                || (Boolean(adapter?.isPublic) && Boolean(siteKey) && !captchaToken)
              }
            >
              {submitting
                ? (adapter?.isPublic ? 'Отправка…' : 'Сохранение…')
                : (adapter?.isPublic ? 'Отправить анкету' : 'Сохранить')}
            </button>
          )}
        </div>
      </main>
    </div>
  );
}
