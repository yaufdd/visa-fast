// wizardPersistence — small wrapper around localStorage for the public
// form wizard. Per-slug blobs survive accidental refresh / tab close so
// the tourist doesn't lose typed text.
//
// Blob shape (under key `fujitravel.wizard.<slug>`):
//   {
//     v: 2,
//     payload: { ...all wizard fields },
//     currentStep: 0..6,
//     savedAt: <epoch ms>,
//   }
//
// What we deliberately DO NOT persist:
//   - consent — every restored session must re-tick the box for legal
//     cleanliness.
//   - submissionId — we no longer create draft submissions on the public
//     side; everything is sent in one multipart POST on final submit.
//   - files — File objects can't be serialised into localStorage by
//     browsers. On a reload the tourist re-picks the files; the typed
//     text is preserved so the form is still mostly intact.
//
// Schema migration: blobs written under v1 carried `submissionId` and
// `files` keys. We tolerate them on read (just ignore the extras) so
// users with a stale v1 blob don't lose the typed text — only the
// stale-and-now-invalid file references silently drop.
//
// Storage failures (quota, Safari private mode) are swallowed silently:
// the form must keep working even if persistence is unavailable.

const KEY_PREFIX = 'fujitravel.wizard.';
const SCHEMA_VERSION = 2;

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
    // v1 carried submissionId / files; we accept v1 blobs, just drop those
    // fields silently — text-only restore is the contract going forward.
    if (!parsed || typeof parsed !== 'object') return null;
    if (parsed.v && parsed.v > SCHEMA_VERSION) return null;
    return {
      payload: parsed.payload && typeof parsed.payload === 'object' ? parsed.payload : {},
      currentStep: Number.isInteger(parsed.currentStep) ? parsed.currentStep : 0,
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
