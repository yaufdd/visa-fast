const API = '/api'

async function apiFetch(path, opts = {}) {
  const hasBody = opts.body !== undefined
  const res = await fetch(API + path, {
    credentials: 'include',
    ...opts,
    headers: {
      ...(hasBody && typeof opts.body === 'string' ? { 'Content-Type': 'application/json' } : {}),
      ...(opts.headers || {}),
    },
  })
  if (res.status === 401 && !path.startsWith('/auth/') && !path.startsWith('/public/')) {
    window.location.href = '/login'
    throw new Error('unauthenticated')
  }
  return res
}

async function errFromRes(res) {
  try {
    const data = await res.json()
    return new Error(data.error || 'request failed')
  } catch {
    return new Error(`${res.status} ${res.statusText}`)
  }
}

// Groups
export async function getGroups() {
  const res = await apiFetch('/groups');
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function createGroup(name) {
  const res = await apiFetch('/groups', {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getGroup(id) {
  const res = await apiFetch(`/groups/${id}`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateGroupStatus(groupId, status) {
  const res = await apiFetch(`/groups/${groupId}/status`, {
    method: 'PUT',
    body: JSON.stringify({ status }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateGroupNotes(groupId, notes) {
  const res = await apiFetch(`/groups/${groupId}/notes`, {
    method: 'PUT',
    body: JSON.stringify({ notes }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function deleteGroup(id) {
  const res = await apiFetch(`/groups/${id}`, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
}

// Tourists
export async function getTourists(groupId) {
  const res = await apiFetch(`/groups/${groupId}/tourists`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function deleteTourist(touristId) {
  const res = await apiFetch(`/tourists/${touristId}`, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
}

export async function getTourist(touristId) {
  const res = await apiFetch(`/tourists/${touristId}`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Per-tourist uploads (ticket | voucher). Two-step flow:
//   1. uploadTouristFile — saves + redacts the scan, does NOT run AI.
//   2. parseTouristUpload — runs the AI parser on a previously uploaded row.
export async function getTouristUploads(touristId) {
  const res = await apiFetch(`/tourists/${touristId}/uploads`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function uploadTouristFile(touristId, file, fileType) {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('file_type', fileType);
  const res = await apiFetch(`/tourists/${touristId}/uploads`, {
    method: 'POST',
    body: formData,
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function parseTouristUpload(touristId, uploadId) {
  const res = await apiFetch(`/tourists/${touristId}/uploads/${uploadId}/parse`, {
    method: 'POST',
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function deleteTouristUpload(touristId, uploadId) {
  const res = await apiFetch(`/tourists/${touristId}/uploads/${uploadId}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Hotels
export async function getHotels() {
  const res = await apiFetch('/hotels');
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function createHotel(data) {
  const res = await apiFetch('/hotels', {
    method: 'POST',
    body: JSON.stringify(data),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getHotel(id) {
  const res = await apiFetch(`/hotels/${id}`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateHotel(id, data) {
  const res = await apiFetch(`/hotels/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getGroupHotels(groupId) {
  const res = await apiFetch(`/groups/${groupId}/hotels`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function saveGroupHotels(groupId, hotels) {
  const res = await apiFetch(`/groups/${groupId}/hotels`, {
    method: 'POST',
    body: JSON.stringify(hotels),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Subgroups
export async function getSubgroups(groupId) {
  const res = await apiFetch(`/groups/${groupId}/subgroups`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function createSubgroup(groupId, name) {
  const res = await apiFetch(`/groups/${groupId}/subgroups`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateSubgroup(subgroupId, name) {
  const res = await apiFetch(`/subgroups/${subgroupId}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function deleteSubgroup(subgroupId) {
  const res = await apiFetch(`/subgroups/${subgroupId}`, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
  // Backend returns 204 No Content — no body to parse.
}

export async function assignTouristSubgroup(touristId, subgroupId) {
  const res = await apiFetch(`/tourists/${touristId}/subgroup`, {
    method: 'PUT',
    body: JSON.stringify({ subgroup_id: subgroupId || null }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getSubgroupHotels(subgroupId) {
  const res = await apiFetch(`/subgroups/${subgroupId}/hotels`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function saveSubgroupHotels(subgroupId, hotels) {
  const res = await apiFetch(`/subgroups/${subgroupId}/hotels`, {
    method: 'POST',
    body: JSON.stringify(hotels),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function generateSubgroupDocuments(subgroupId) {
  const res = await apiFetch(`/subgroups/${subgroupId}/generate`, { method: 'POST' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export function getSubgroupDownloadUrl(subgroupId) {
  return `${API}/subgroups/${subgroupId}/download`;
}

// Documents
export async function generateDocuments(groupId, guidePhone = '') {
  const qs = guidePhone ? `?guide_phone=${encodeURIComponent(guidePhone)}` : '';
  const res = await apiFetch(`/groups/${groupId}/generate${qs}`, { method: 'POST' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export function getDownloadUrl(groupId) {
  return `${API}/groups/${groupId}/download`;
}

export async function finalizeGroup(groupId, submissionDate = '') {
  const qs = submissionDate ? `?submission_date=${encodeURIComponent(submissionDate)}` : '';
  const res = await apiFetch(`/groups/${groupId}/finalize${qs}`, { method: 'POST' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export function getFinalDownloadUrl(groupId) {
  return `${API}/groups/${groupId}/download/final`;
}

export async function getFinalStatus(groupId) {
  const res = await apiFetch(`/groups/${groupId}/final/status`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Submissions (form-workflow)
export async function apiCreateSubmission(payload, consentAccepted) {
  const res = await apiFetch('/submissions', {
    method: 'POST',
    body: JSON.stringify({ payload, consent_accepted: consentAccepted, source: 'manager' }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function listSubmissions(q = '', status = '') {
  const params = new URLSearchParams();
  if (q) params.set('q', q);
  if (status) params.set('status', status);
  const qs = params.toString();
  const res = await apiFetch(qs ? `/submissions?${qs}` : '/submissions');
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getSubmission(id) {
  const res = await apiFetch(`/submissions/${id}`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateSubmission(id, payload) {
  const res = await apiFetch(`/submissions/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ payload }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function archiveSubmission(id) {
  const res = await apiFetch(`/submissions/${id}`, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function eraseSubmission(id) {
  const res = await apiFetch(`/submissions/${id}/erase`, { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function attachSubmission(id, groupId, subgroupId = null) {
  const res = await apiFetch(`/submissions/${id}/attach`, {
    method: 'POST',
    body: JSON.stringify({ group_id: groupId, subgroup_id: subgroupId }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function getConsentText() {
  const res = await apiFetch('/consent/text');
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateFlightData(touristId, data) {
  const res = await apiFetch(`/tourists/${touristId}/flight_data`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function applyFlightDataToSubgroup(touristId) {
  const res = await apiFetch(
    `/tourists/${touristId}/flight_data/apply_to_subgroup`,
    { method: 'POST' },
  );
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateGroupName(groupId, name) {
  const res = await apiFetch(`/groups/${groupId}/name`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateGroupProgrammeNotes(groupId, notes) {
  const res = await apiFetch(`/groups/${groupId}/programme_notes`, {
    method: 'PUT',
    body: JSON.stringify({ notes }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function updateSubgroupProgrammeNotes(subgroupId, notes) {
  const res = await apiFetch(`/subgroups/${subgroupId}/programme_notes`, {
    method: 'PUT',
    body: JSON.stringify({ notes }),
  });
  if (!res.ok) throw await errFromRes(res);
}

// ── Document templates ────────────────────────────────────────────────────
export async function getDoverenostTemplateStatus() {
  const res = await apiFetch('/templates/doverenost');
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function uploadDoverenostTemplate(file) {
  const fd = new FormData();
  fd.append('file', file);
  const res = await apiFetch('/templates/doverenost', {
    method: 'POST',
    body: fd,
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

export async function deleteDoverenostTemplate() {
  const res = await apiFetch('/templates/doverenost', { method: 'DELETE' });
  if (!res.ok) throw await errFromRes(res);
}

export function getDoverenostTemplateDownloadUrl() {
  return `${API}/templates/doverenost/download`;
}

// ── Submission files (uploaded by tourist via the public wizard) ──────────
// Backend: GET/DELETE /api/submissions/{id}/files[/file_id[/download]]
// Auth: session cookie. Returns metadata only (no file_path / org_id).
export async function listSubmissionFilesAdmin(submissionId) {
  const res = await apiFetch(`/submissions/${encodeURIComponent(submissionId)}/files`);
  if (res.status === 404) {
    const err = new Error('not found');
    err.notFound = true;
    throw err;
  }
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Synchronous URL builder so an <a href download> can stream the file
// directly — the browser then handles Content-Disposition (incl. RFC-5987
// filename* for Cyrillic names) without us copying bytes through JS.
export function submissionFileDownloadUrl(submissionId, fileId) {
  return `${API}/submissions/${encodeURIComponent(submissionId)}/files/${encodeURIComponent(fileId)}/download`;
}

export async function deleteSubmissionFileAdmin(submissionId, fileId) {
  const res = await apiFetch(
    `/submissions/${encodeURIComponent(submissionId)}/files/${encodeURIComponent(fileId)}`,
    { method: 'DELETE' },
  );
  if (!res.ok) throw await errFromRes(res);
}

// createDraftSubmission allocates an empty draft row owned by the
// authenticated manager's org so the dashboard wizard has something to
// attach scans to before the manager finalises the payload. Mirrors the
// public /start endpoint. Returns { submission_id }.
export async function createDraftSubmission() {
  const res = await apiFetch('/submissions/draft', {
    method: 'POST',
    body: '{}',
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// uploadSubmissionFileAdmin attaches a file (passport / ticket / voucher)
// to a submission via the session-authenticated endpoint. If onProgress
// is provided we use XHR so the upload-progress event fires (fetch can't
// surface upload progress in browsers); otherwise plain fetch.
//
// Same friendly status-code mapping as the public sibling — keeps the
// wizard's error UX identical regardless of which adapter it's wired to.
export async function uploadSubmissionFileAdmin(submissionId, fileType, file, onProgress) {
  const url = `${API}/submissions/${encodeURIComponent(submissionId)}/files/${encodeURIComponent(fileType)}`;
  const fd = new FormData();
  fd.append('file', file);

  const friendly = (status, fallback) => {
    if (status === 413) return new Error('Файл слишком большой (>50 МБ)');
    if (status === 429) return new Error('Слишком много загрузок, подождите минуту');
    if (status === 401) {
      // Mirror apiFetch's redirect-on-401 so an expired session boots the
      // manager back to /login instead of leaving them with a cryptic
      // "request failed" toast inside the wizard.
      window.location.href = '/login';
    }
    return fallback;
  };

  if (typeof onProgress === 'function') {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', url);
      // Session cookie carries auth — XHR includes it on same-origin by default.
      xhr.withCredentials = true;
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
          } catch {
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
        reject(friendly(xhr.status, err));
      };
      xhr.onerror = () => reject(new Error('Сетевая ошибка при загрузке'));
      xhr.onabort = () => reject(new Error('Загрузка отменена'));
      xhr.send(fd);
    });
  }

  const res = await apiFetch(`/submissions/${encodeURIComponent(submissionId)}/files/${encodeURIComponent(fileType)}`, {
    method: 'POST',
    body: fd,
  });
  if (!res.ok) {
    const baseErr = await errFromRes(res);
    throw friendly(res.status, baseErr);
  }
  return res.json();
}

// parseSubmissionPassportAdmin runs the Yandex Vision + YandexGPT pipeline
// on a previously-uploaded passport scan and returns the structured fields
// the wizard merges into its payload. `type` is "internal" or "foreign".
export async function parseSubmissionPassportAdmin(submissionId, fileId, type) {
  const res = await apiFetch(`/submissions/${encodeURIComponent(submissionId)}/parse-passport`, {
    method: 'POST',
    body: JSON.stringify({ file_id: fileId, type }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// parseSubmissionTicketAdmin recognises a single ticket scan attached to
// the submission. Returns { arrival, departure } — same shape as the
// per-tourist parser, so the wizard's payload.flight_data slot accepts it
// directly.
export async function parseSubmissionTicketAdmin(submissionId, fileId) {
  const res = await apiFetch(`/submissions/${encodeURIComponent(submissionId)}/parse-ticket`, {
    method: 'POST',
    body: JSON.stringify({ file_id: fileId }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// parseSubmissionVoucherAdmin recognises a single voucher scan and returns
// the array of hotel stays found inside. Wizard appends to payload.hotels.
export async function parseSubmissionVoucherAdmin(submissionId, fileId) {
  const res = await apiFetch(`/submissions/${encodeURIComponent(submissionId)}/parse-voucher`, {
    method: 'POST',
    body: JSON.stringify({ file_id: fileId }),
  });
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// Returns { "<submission_id>": <count>, ... } for every tourist in the
// group whose submission has at least one file attached. Tourists with
// no submission_id (manager-created manually) and submissions with zero
// files are omitted — frontend should treat absence as zero.
export async function getGroupTouristFileCounts(groupId) {
  const res = await apiFetch(`/groups/${encodeURIComponent(groupId)}/tourist_file_counts`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// ── AI audit log ──
// Returns the latest 500 AI (Claude) calls made on behalf of this group,
// newest first. Image/PDF bytes inside request JSONs are redacted server-side.
export async function getGroupAILogs(groupId) {
  const res = await apiFetch(`/groups/${encodeURIComponent(groupId)}/ai_logs`);
  if (!res.ok) throw await errFromRes(res);
  return res.json();
}

// ── Auth ──
export async function apiRegister(orgName, email, password) {
  const res = await apiFetch('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ org_name: orgName, email, password }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiLogin(email, password) {
  const res = await apiFetch('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiLogout() {
  const res = await apiFetch('/auth/logout', { method: 'POST' })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiMe() {
  const res = await apiFetch('/auth/me')
  if (res.status === 401) return null
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

// ── Public (no session) ──
export async function publicGetOrg(slug) {
  const res = await fetch(`${API}/public/org/${slug}`)
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

// publicCreateSubmission posts the final form payload. When `submissionId`
// is provided (Phase 3 flow), the backend finalises an existing draft row
// — the same id that was returned by /start and used to attach scans. Old
// callers that omit it get the legacy "create new pending row" behaviour.
export async function publicCreateSubmission(slug, payload, consentAccepted, submissionId) {
  const body = { payload, consent_accepted: consentAccepted }
  if (submissionId) body.submission_id = submissionId
  const res = await fetch(`${API}/public/submissions/${slug}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}
