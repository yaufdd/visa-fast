// wizardPersistence — small wrapper around localStorage for the public
// form wizard. Per-slug blobs survive accidental refresh / tab close so
// the tourist doesn't lose progress. Strictly per-browser convenience —
// not a multi-device sync.
//
// Blob shape (under key `fujitravel.wizard.<slug>`):
//   {
//     v: 1,
//     payload: { ...all wizard fields },
//     currentStep: 0..6,
//     submissionId: "uuid" | null,
//     files: { passport_internal, passport_foreign, ticket, voucher },
//     savedAt: <epoch ms>,
//   }
//
// Consent is intentionally NOT persisted — every restored session must
// re-tick the box for legal cleanliness.
//
// Storage failures (quota, Safari private mode) are swallowed silently:
// the form must keep working even if persistence is unavailable.

const KEY_PREFIX = 'fujitravel.wizard.';
const SCHEMA_VERSION = 1;

function keyFor(slug) {
  return `${KEY_PREFIX}${slug}`;
}

// Best-effort feature probe. Safari private mode throws on setItem; some
// embeds disable storage entirely. We check once per call so a tab that
// changes privacy state mid-session still degrades gracefully.
function storageAvailable() {
  try {
    const probe = '__fuji_probe__';
    window.localStorage.setItem(probe, '1');
    window.localStorage.removeItem(probe);
    return true;
  } catch {
    return false;
  }
}

export function loadWizardBlob(slug) {
  if (!slug || !storageAvailable()) return null;
  try {
    const raw = window.localStorage.getItem(keyFor(slug));
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    // Schema-drift guard: ignore future versions we don't know how to read.
    // For v1 we just require an object with a payload field; missing keys
    // get filled by the wizard's defaults.
    if (!parsed || typeof parsed !== 'object') return null;
    if (parsed.v && parsed.v > SCHEMA_VERSION) return null;
    return {
      payload: parsed.payload && typeof parsed.payload === 'object' ? parsed.payload : {},
      currentStep: Number.isInteger(parsed.currentStep) ? parsed.currentStep : 0,
      submissionId: typeof parsed.submissionId === 'string' ? parsed.submissionId : null,
      files: parsed.files && typeof parsed.files === 'object' ? parsed.files : null,
      savedAt: typeof parsed.savedAt === 'number' ? parsed.savedAt : 0,
    };
  } catch {
    return null;
  }
}

export function saveWizardBlob(slug, blob) {
  if (!slug || !storageAvailable()) return;
  try {
    const out = {
      v: SCHEMA_VERSION,
      payload: blob.payload || {},
      currentStep: Number.isInteger(blob.currentStep) ? blob.currentStep : 0,
      submissionId: blob.submissionId || null,
      files: blob.files || null,
      savedAt: Date.now(),
    };
    window.localStorage.setItem(keyFor(slug), JSON.stringify(out));
  } catch {
    // Quota exceeded or storage disabled — silently skip.
  }
}

export function clearWizardBlob(slug) {
  if (!slug) return;
  try {
    window.localStorage.removeItem(keyFor(slug));
  } catch {
    // Ignore — clearing a non-existent key is a no-op anyway.
  }
}
