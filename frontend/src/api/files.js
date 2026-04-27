// Public-form file upload helpers.
//
// These wrap the four Phase 2 endpoints used by the tourist-facing
// /form/<slug> page:
//   POST   /api/public/submissions/{slug}/start
//   POST   /api/public/submissions/{slug}/files/{type}     (multipart)
//   GET    /api/public/submissions/{slug}/files?submission_id=...
//   DELETE /api/public/submissions/{slug}/files/{id}?submission_id=...
//   POST   /api/public/submissions/{slug}/parse-passport
//
// All endpoints are unauthenticated (no session cookie required), so we
// use raw fetch + a small inline error helper rather than the dashboard
// `apiFetch` (which redirects on 401, which we don't want here).

const API = '/api';

async function errFromRes(res) {
  // Mirror the error shape used by client.js so the UI handlers can
  // rely on err.message being populated.
  try {
    const data = await res.json();
    return new Error(data?.error || `request failed (${res.status})`);
  } catch {
    return new Error(`${res.status} ${res.statusText}`);
  }
}

// Friendly mapping for the public-form-specific status codes the upload
// endpoint can return. The backend caps each file at 50 MB and applies
// per-IP rate limiting.
function friendlyUploadError(status, fallback) {
  if (status === 413) return new Error('Файл слишком большой (>50 МБ)');
  if (status === 429) return new Error('Слишком много загрузок, подождите минуту');
  return fallback;
}

// startSubmission creates a draft submission row so the tourist can attach
// scans before they finish typing the form. Returns { submission_id }.
export async function startSubmission(slug) {
  const res = await fetch(`${API}/public/submissions/${encodeURIComponent(slug)}/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// uploadSubmissionFile attaches a file (passport / ticket / voucher) to
// the draft. If `onProgress` is provided, the call is dispatched via
// XMLHttpRequest so we can hook the upload-progress event (fetch can't
// report it in browsers). Without `onProgress` we use plain fetch.
//
// Returns the file metadata as returned by the backend (id, original_name,
// size_bytes, mime_type, created_at, file_type).
export async function uploadSubmissionFile(slug, submissionId, fileType, file, onProgress) {
  const url = `${API}/public/submissions/${encodeURIComponent(slug)}/files/${encodeURIComponent(fileType)}`;
  const fd = new FormData();
  fd.append('submission_id', submissionId);
  fd.append('file', file);

  if (typeof onProgress === 'function') {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', url);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          const pct = Math.round((e.loaded / e.total) * 100);
          onProgress(pct);
        }
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            resolve(JSON.parse(xhr.responseText));
          } catch (err) {
            reject(new Error('invalid JSON response'));
          }
          return;
        }
        let err;
        try {
          const data = JSON.parse(xhr.responseText);
          err = new Error(data?.error || `request failed (${xhr.status})`);
        } catch {
          err = new Error(`${xhr.status} ${xhr.statusText}`);
        }
        reject(friendlyUploadError(xhr.status, err));
      };
      xhr.onerror = () => reject(new Error('Сетевая ошибка при загрузке'));
      xhr.onabort = () => reject(new Error('Загрузка отменена'));
      xhr.send(fd);
    });
  }

  const res = await fetch(url, { method: 'POST', body: fd });
  if (!res.ok) {
    const baseErr = await errFromRes(res);
    throw friendlyUploadError(res.status, baseErr);
  }
  return res.json();
}

// listSubmissionFiles returns the metadata rows currently attached to the
// draft so the form can re-render badges after a page reload.
export async function listSubmissionFiles(slug, submissionId) {
  const url = `${API}/public/submissions/${encodeURIComponent(slug)}/files?submission_id=${encodeURIComponent(submissionId)}`;
  const res = await fetch(url);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// deleteSubmissionFile removes both the file row and the on-disk bytes.
export async function deleteSubmissionFile(slug, submissionId, fileId) {
  const url = `${API}/public/submissions/${encodeURIComponent(slug)}/files/${encodeURIComponent(fileId)}?submission_id=${encodeURIComponent(submissionId)}`;
  const res = await fetch(url, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// parsePassport runs the Yandex Vision + YandexGPT pipeline on a
// previously-uploaded passport scan and returns the structured fields
// (see backend ai.PassportFields). `type` is "internal" or "foreign".
export async function parsePassport(slug, submissionId, fileId, type) {
  const res = await fetch(`${API}/public/submissions/${encodeURIComponent(slug)}/parse-passport`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      submission_id: submissionId,
      file_id: fileId,
      type,
    }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}
