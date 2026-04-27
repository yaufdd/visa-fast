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
