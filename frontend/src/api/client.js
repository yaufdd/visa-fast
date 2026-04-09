const API = import.meta.env.VITE_API_URL || 'http://localhost:8081';

// Groups
export async function getGroups() {
  const res = await fetch(`${API}/api/groups`);
  if (!res.ok) throw new Error('Failed to fetch groups');
  return res.json();
}

export async function createGroup(name) {
  const res = await fetch(`${API}/api/groups`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error('Failed to create group');
  return res.json();
}

export async function getGroup(id) {
  const res = await fetch(`${API}/api/groups/${id}`);
  if (!res.ok) throw new Error('Failed to fetch group');
  return res.json();
}

export async function deleteGroup(id) {
  const res = await fetch(`${API}/api/groups/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete group');
}

// Uploads
export async function uploadFile(groupId, file, fileType, subgroupId = '') {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('file_type', fileType || 'document');
  if (subgroupId) formData.append('subgroup_id', subgroupId);
  const res = await fetch(`${API}/api/groups/${groupId}/uploads`, {
    method: 'POST',
    body: formData,
  });
  if (!res.ok) throw new Error('Failed to upload file');
  return res.json();
}

export async function getUploads(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/uploads`);
  if (!res.ok) throw new Error('Failed to fetch uploads');
  return res.json();
}

export async function parseDocuments(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/parse`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to start parsing');
  return res.json();
}

export async function parseGroup(groupId, notes = '') {
  const url = new URL(`${API}/api/groups/${groupId}/parse`);
  if (notes.trim()) url.searchParams.set('notes', notes.trim());
  const res = await fetch(url.toString(), { method: 'POST' });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || 'Failed to parse group');
  }
  return res.json();
}

// Tourists (legacy helpers kept for compatibility)
export async function getTourists(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/tourists`);
  if (!res.ok) throw new Error('Failed to fetch tourists');
  return res.json();
}

export async function searchSheets(query) {
  const res = await fetch(`${API}/api/sheets/search?q=${encodeURIComponent(query)}`);
  if (!res.ok) throw new Error('Failed to search sheets');
  return res.json();
}

export async function matchTourist(touristId, sheetRow) {
  const res = await fetch(`${API}/api/tourists/${touristId}/match`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sheet_row: sheetRow }),
  });
  if (!res.ok) throw new Error('Failed to match tourist');
  return res.json();
}

// Sheets rows
export async function getSheetRows(q = '') {
  const url = q.trim()
    ? `${API}/api/sheets/rows?q=${encodeURIComponent(q.trim())}`
    : `${API}/api/sheets/rows`;
  const res = await fetch(url);
  if (!res.ok) throw new Error('Failed to fetch sheet rows');
  return res.json();
}

// Tourists — new workflow
export async function addTouristFromSheet(groupId, sheetRow, subgroupId = null) {
  const res = await fetch(`${API}/api/groups/${groupId}/tourists`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sheet_row: sheetRow, subgroup_id: subgroupId || undefined }),
  });
  if (!res.ok) throw new Error('Failed to add tourist');
  return res.json();
}

export async function deleteTourist(touristId) {
  const res = await fetch(`${API}/api/tourists/${touristId}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete tourist');
}

// Per-tourist uploads
export async function getTouristUploads(touristId) {
  const res = await fetch(`${API}/api/tourists/${touristId}/uploads`);
  if (!res.ok) throw new Error('Failed to fetch tourist uploads');
  return res.json();
}

export async function uploadTouristFile(touristId, file) {
  const formData = new FormData();
  formData.append('file', file);
  const res = await fetch(`${API}/api/tourists/${touristId}/uploads`, {
    method: 'POST',
    body: formData,
  });
  if (!res.ok) throw new Error('Failed to upload tourist file');
  return res.json();
}

export async function parseTourist(touristId) {
  const res = await fetch(`${API}/api/tourists/${touristId}/parse`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to start parsing');
  return res.json();
}

// Hotels
export async function getHotels() {
  const res = await fetch(`${API}/api/hotels`);
  if (!res.ok) throw new Error('Failed to fetch hotels');
  return res.json();
}

export async function createHotel(data) {
  const res = await fetch(`${API}/api/hotels`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error('Failed to create hotel');
  return res.json();
}

export async function getGroupHotels(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/hotels`);
  if (!res.ok) throw new Error('Failed to fetch group hotels');
  return res.json();
}

export async function saveGroupHotels(groupId, hotels) {
  const res = await fetch(`${API}/api/groups/${groupId}/hotels`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(hotels),
  });
  if (!res.ok) throw new Error('Failed to save group hotels');
  return res.json();
}

// Subgroups
export async function getSubgroups(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/subgroups`);
  if (!res.ok) throw new Error('Failed to fetch subgroups');
  return res.json();
}

export async function createSubgroup(groupId, name) {
  const res = await fetch(`${API}/api/groups/${groupId}/subgroups`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error('Failed to create subgroup');
  return res.json();
}

export async function updateSubgroup(subgroupId, name) {
  const res = await fetch(`${API}/api/subgroups/${subgroupId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error('Failed to update subgroup');
  return res.json();
}

export async function deleteSubgroup(subgroupId) {
  const res = await fetch(`${API}/api/subgroups/${subgroupId}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete subgroup');
}

export async function assignTouristSubgroup(touristId, subgroupId) {
  const res = await fetch(`${API}/api/tourists/${touristId}/subgroup`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ subgroup_id: subgroupId || null }),
  });
  if (!res.ok) throw new Error('Failed to assign subgroup');
  return res.json();
}

export async function getSubgroupHotels(subgroupId) {
  const res = await fetch(`${API}/api/subgroups/${subgroupId}/hotels`);
  if (!res.ok) throw new Error('Failed to fetch subgroup hotels');
  return res.json();
}

export async function saveSubgroupHotels(subgroupId, hotels) {
  const res = await fetch(`${API}/api/subgroups/${subgroupId}/hotels`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(hotels),
  });
  if (!res.ok) throw new Error('Failed to save subgroup hotels');
  return res.json();
}

export async function generateSubgroupDocuments(subgroupId) {
  const res = await fetch(`${API}/api/subgroups/${subgroupId}/generate`, { method: 'POST' });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || 'Failed to generate subgroup documents');
  }
  return res.json();
}

export function getSubgroupDownloadUrl(subgroupId) {
  return `${API}/api/subgroups/${subgroupId}/download`;
}

export async function parseSubgroup(subgroupId, notes = '') {
  const url = new URL(`${API}/api/subgroups/${subgroupId}/parse`);
  if (notes.trim()) url.searchParams.set('notes', notes.trim());
  const res = await fetch(url.toString(), { method: 'POST' });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || 'Failed to parse subgroup');
  }
  return res.json();
}

// Documents
export async function generateDocuments(groupId, guidePhone = '') {
  const url = new URL(`${API}/api/groups/${groupId}/generate`);
  if (guidePhone) url.searchParams.set('guide_phone', guidePhone);
  const res = await fetch(url.toString(), { method: 'POST' });
  if (!res.ok) throw new Error('Failed to generate documents');
  return res.json();
}

export function getDownloadUrl(groupId) {
  return `${API}/api/groups/${groupId}/download`;
}

export async function finalizeGroup(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/finalize`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to finalize group');
  return res.json();
}

export function getFinalDownloadUrl(groupId) {
  return `${API}/api/groups/${groupId}/download/final`;
}

export async function getFinalStatus(groupId) {
  const res = await fetch(`${API}/api/groups/${groupId}/final/status`);
  if (!res.ok) throw new Error('Failed to fetch final status');
  return res.json();
}
