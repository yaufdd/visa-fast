import { forwardRef, useImperativeHandle, useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getGroup,
  getHotels, getSubgroupHotels, saveSubgroupHotels,
  finalizeGroup, getFinalDownloadUrl, getFinalStatus,
  generateSubgroupDocuments, getSubgroupDownloadUrl,
  getTourists, deleteTourist,
  getSubgroups, createSubgroup, updateSubgroup, deleteSubgroup,
  assignTouristSubgroup,
  updateGroupStatus, deleteGroup, updateGroupName,
  updateSubgroupProgrammeNotes,
  getGroupTouristFileCounts,
  uploadTouristFile, parseTouristUpload,
} from '../api/client';
import StatusSection from '../components/StatusSection';
import AddFromDBModal from '../components/AddFromDBModal';
import TouristCard from '../components/TouristCard';
import { normalizeCity } from '../constants/cities';
import { formatAIError } from '../utils/aiError';

// Folder-download icon.
const FolderIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
    <path d="M1.5 3.5a1 1 0 0 1 1-1h3.586a1 1 0 0 1 .707.293l1.414 1.414a1 1 0 0 0 .707.293H13.5a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V3.5Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
  </svg>
);

// Trash (delete) icon — used in place of the old ✕ button.
const TrashIcon = ({ size = 14 }) => (
  <svg width={size} height={size} viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, display: 'block' }}>
    <path d="M3 4h10M6.5 4V2.5a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1V4M4 4l.5 8.5a1.5 1.5 0 0 0 1.5 1.4h4a1.5 1.5 0 0 0 1.5-1.4L12 4M6.5 7v4M9.5 7v4"
      stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
  </svg>
);

// "X" icon — used in the Groups view as a quieter alternative to the trash
// silhouette (deletion in this view is a quick one-click action).
const CrossIcon = ({ size = 13 }) => (
  <svg width={size} height={size} viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, display: 'block' }}>
    <path d="M4 4l8 8M12 4l-8 8"
      stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
  </svg>
);

// "Add person" icon — silhouette with a plus sign.
const UserPlusIcon = ({ size = 14 }) => (
  <svg width={size} height={size} viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, display: 'block' }}>
    <circle cx="6" cy="5" r="2.4" stroke="currentColor" strokeWidth="1.3"/>
    <path d="M2 13.5c0-2.2 1.8-4 4-4s4 1.8 4 4" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/>
    <path d="M12 6.5v4M10 8.5h4" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
  </svg>
);

// Sparkle / 4-point star — used for "auto-fill from a scan".
const MagicIcon = ({ size = 14 }) => (
  <svg width={size} height={size} viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, display: 'block' }}>
    <path d="M8 1.5L9.2 6.8L14.5 8L9.2 9.2L8 14.5L6.8 9.2L1.5 8L6.8 6.8Z"
      fill="currentColor"/>
  </svg>
);
import StatusBadge from '../components/StatusBadge';
import Modal from '../components/Modal';

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short', year: 'numeric' });
}

// ── GroupCard ─────────────────────────────────────────────────────────────────

function GroupCard({ group, groupId, allTourists, fileCounts, onReload, onFilesChanged, onTouristDeleted, onRenamed, onDeleted }) {
  const [expanded, setExpanded] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(group.name);
  const [cardError, setCardError] = useState(null);
  const [hotelsExpanded, setHotelsExpanded] = useState(false);
  const [touristsExpanded, setTouristsExpanded] = useState(true);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState(null);

  // Wiring for "magic button on a tourist's voucher in the docs modal" → run
  // the parse inside SubgroupHotelsSection so the spinner appears there.
  // pendingVoucher is queued from the modal callback; the effect below fires
  // it once the section has actually mounted (after auto-expand).
  const hotelsRef = useRef(null);
  const [pendingVoucher, setPendingVoucher] = useState(null);
  useEffect(() => {
    if (!pendingVoucher) return;
    if (!hotelsExpanded) return;
    if (!hotelsRef.current) return;
    const { touristId, uploadIds } = pendingVoucher;
    hotelsRef.current.parseExisting(touristId, uploadIds);
    setPendingVoucher(null);
  }, [pendingVoucher, hotelsExpanded]);
  const handleVoucherParseRequest = useCallback((touristId, uploadIds) => {
    setHotelsExpanded(true);
    setPendingVoucher({ touristId, uploadIds });
  }, []);

  const tourists = allTourists.filter(t => t.subgroup_id === group.id);

  const handleRename = async () => {
    if (!editName.trim() || editName === group.name) { setEditing(false); return; }
    try {
      await updateSubgroup(group.id, editName.trim());
      onRenamed(group.id, editName.trim());
      setEditing(false);
    } catch (e) {
      setCardError(e.message);
    }
  };

  // Triggered by the trash button — opens the confirmation modal instead of
  // the native browser confirm() so the UX matches the rest of the app.
  const requestDelete = () => {
    setDeleteError(null);
    setConfirmDeleteOpen(true);
  };

  const handleDeleteConfirmed = async () => {
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteSubgroup(group.id);
      setConfirmDeleteOpen(false);
      onDeleted(group.id);
    } catch (e) {
      setDeleteError(e.message);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div style={{
      border: '1px solid var(--border)', borderRadius: 10,
      background: 'var(--graphite)', marginBottom: 16, overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, padding: '14px 18px',
        cursor: 'pointer', userSelect: 'none',
        borderBottom: expanded ? '1px solid var(--border)' : 'none',
      }} onClick={() => !editing && setExpanded(e => !e)}>
        <span style={{ fontSize: 13, color: 'var(--white-dim)', transition: 'transform 0.2s', display: 'inline-block', transform: expanded ? 'rotate(90deg)' : 'none' }}>▶</span>

        {editing ? (
          <input
            className="form-input"
            style={{ flex: 1, fontSize: 14, padding: '4px 8px' }}
            value={editName}
            onChange={e => setEditName(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleRename(); if (e.key === 'Escape') setEditing(false); }}
            onClick={e => e.stopPropagation()}
            autoFocus
          />
        ) : (
          <span style={{ flex: 1, fontSize: 14, fontWeight: 600, color: 'var(--white)' }}>{group.name}</span>
        )}

        {editing && (
          <>
            <button className="btn btn-primary btn-sm" onClick={e => { e.stopPropagation(); handleRename(); }}>OK</button>
            <button className="btn btn-secondary btn-sm" onClick={e => { e.stopPropagation(); setEditing(false); setEditName(group.name); }}>Отмена</button>
          </>
        )}
      </div>

      {/* Body */}
      {expanded && (
        <div style={{ padding: '16px 18px', display: 'flex', flexDirection: 'column', gap: 14 }}>
          {cardError && <div className="error-message">{cardError}</div>}

          {/* Tourist list — collapsible */}
          <div>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: touristsExpanded ? 8 : 0,
              }}
            >
              <div
                onClick={() => setTouristsExpanded(e => !e)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  cursor: 'pointer',
                  userSelect: 'none',
                  flex: 1,
                }}
              >
                <span
                  style={{
                    fontSize: 10,
                    color: 'var(--white-dim)',
                    transition: 'transform 0.2s',
                    display: 'inline-block',
                    transform: touristsExpanded ? 'rotate(90deg)' : 'none',
                  }}
                >▶</span>
                <span
                  style={{
                    fontSize: 12,
                    color: 'var(--white-dim)',
                    fontWeight: 500,
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                  }}
                >
                  Туристы{tourists.length > 0 ? ` (${tourists.length})` : ''}
                </span>
              </div>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => setShowAddModal(true)}
                title="Добавить туриста"
                aria-label="Добавить туриста"
                style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
              >
                <UserPlusIcon />
              </button>
            </div>
            {touristsExpanded && tourists.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {tourists.map((t) => (
                  <TouristCard
                    key={t.id}
                    tourist={t}
                    fileCount={(fileCounts && t.submission_id) ? (fileCounts[t.submission_id] || 0) : 0}
                    onFilesChanged={onFilesChanged}
                    onUpdated={onReload}
                    onVoucherParseRequest={handleVoucherParseRequest}
                    onDelete={async () => {
                      try { await deleteTourist(t.id); onTouristDeleted(t.id); }
                      catch (e) { setCardError(e.message); }
                    }}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Hotels for this subgroup — collapsible, collapsed by default */}
          <div style={{ borderTop: '1px solid var(--border)', paddingTop: 14 }}>
            <div
              onClick={() => setHotelsExpanded((e) => !e)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                cursor: 'pointer',
                userSelect: 'none',
                marginBottom: hotelsExpanded ? 10 : 0,
              }}
            >
              <span
                style={{
                  fontSize: 10,
                  color: 'var(--white-dim)',
                  transition: 'transform 0.2s',
                  display: 'inline-block',
                  transform: hotelsExpanded ? 'rotate(90deg)' : 'none',
                }}
              >▶</span>
              <span
                style={{
                  fontSize: 12,
                  color: 'var(--white-dim)',
                  fontWeight: 500,
                  textTransform: 'uppercase',
                  letterSpacing: '0.05em',
                }}
              >
                Отели группы
              </span>
            </div>
            {hotelsExpanded && (
              <SubgroupHotelsSection
                ref={hotelsRef}
                subgroupId={group.id}
                tourists={tourists}
              />
            )}
          </div>
        </div>
      )}

      <Modal
        open={showAddModal}
        onClose={() => setShowAddModal(false)}
        title={`Добавить в "${group.name}"`}
        width={560}
      >
        <AddFromDBModal
          groupId={groupId}
          subgroupId={group.id}
          onAdded={onReload}
          onClose={() => setShowAddModal(false)}
        />
      </Modal>

      <Modal
        open={confirmDeleteOpen}
        onClose={() => !deleting && setConfirmDeleteOpen(false)}
        title="Удалить группу?"
        width={440}
      >
        <div style={{ fontSize: 13, color: 'var(--white)', marginBottom: 16, lineHeight: 1.5 }}>
          Удалить группу <strong>«{group.name}»</strong>? Туристы останутся в подаче без группы.
        </div>
        {deleteError && <div className="error-message" style={{ marginBottom: 12 }}>{deleteError}</div>}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => setConfirmDeleteOpen(false)}
            disabled={deleting}
          >
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={handleDeleteConfirmed}
            disabled={deleting}
          >
            {deleting ? <><span className="spinner" /> Удаление…</> : 'Удалить'}
          </button>
        </div>
      </Modal>
    </div>
  );
}

// ── GroupsTab ─────────────────────────────────────────────────────────────────

function GroupsTab({ groupId }) {
  const [subgroups, setSubgroups] = useState([]);
  const [tourists, setTourists] = useState([]);
  const [fileCounts, setFileCounts] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showNewForm, setShowNewForm] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);

  // Reloads only the file-count map. Used after the tourist files
  // modal is closed (a delete inside the modal could have changed
  // the count). Failures are swallowed — the badge is non-critical UI.
  const reloadFileCounts = useCallback(async () => {
    try {
      const counts = await getGroupTouristFileCounts(groupId);
      setFileCounts(counts && typeof counts === 'object' ? counts : {});
    } catch {
      /* ignore — keep stale counts on error */
    }
  }, [groupId]);

  const load = useCallback(async () => {
    try {
      const [sgs, ts, counts] = await Promise.all([
        getSubgroups(groupId),
        getTourists(groupId),
        getGroupTouristFileCounts(groupId).catch(() => ({})),
      ]);
      setSubgroups(Array.isArray(sgs) ? sgs : []);
      setTourists(Array.isArray(ts) ? ts : []);
      setFileCounts(counts && typeof counts === 'object' ? counts : {});
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [groupId]);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const sg = await createSubgroup(groupId, newName.trim());
      setSubgroups(prev => [...prev, sg]);
      setNewName('');
      setShowNewForm(false);
    } catch (e) {
      setError(e.message);
    } finally {
      setCreating(false);
    }
  };

  const handleRenamed = (sgId, name) => setSubgroups(prev => prev.map(sg => sg.id === sgId ? { ...sg, name } : sg));
  const handleDeleted = (sgId) => {
    // Backend FK is ON DELETE SET NULL — tourists survive but lose their
    // subgroup_id. Mirror that in local state so they re-appear in "Без группы".
    setSubgroups(prev => prev.filter(sg => sg.id !== sgId));
    setTourists(prev => prev.map(t => t.subgroup_id === sgId ? { ...t, subgroup_id: null } : t));
  };
  const handleTouristDeleted = (tid) => setTourists(prev => prev.filter(t => t.id !== tid));

  // Unassigned tourists
  const unassigned = tourists.filter(t => !t.subgroup_id);

  if (loading) return <div className="loading-center"><div className="spinner spinner-lg" /></div>;

  return (
    <div>
      {error && <div className="error-message" style={{ marginBottom: 14 }}>{error}</div>}

      <div style={{ display: 'flex', justifyContent: 'flex-end', alignItems: 'center', marginBottom: 16, gap: 12 }}>
        <button className="btn btn-primary btn-sm" onClick={() => setShowNewForm(true)}>
          + Добавить группу
        </button>
      </div>

      {/* New group form */}
      {showNewForm && (
        <div style={{ display: 'flex', gap: 8, marginBottom: 16, alignItems: 'center' }}>
          <input
            className="form-input"
            style={{ flex: 1, fontSize: 13 }}
            placeholder="Название группы (напр. Кузнецовы)"
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleCreate()}
            autoFocus
          />
          <button className="btn btn-primary btn-sm" onClick={handleCreate} disabled={creating || !newName.trim()}>
            {creating ? <span className="spinner" /> : 'Создать'}
          </button>
          <button className="btn btn-secondary btn-sm" onClick={() => { setShowNewForm(false); setNewName(''); }}>
            Отмена
          </button>
        </div>
      )}

      {subgroups.length === 0 && !showNewForm ? (
        <div className="card">
          <div className="empty-state">
            <div className="empty-state-title">Нет групп</div>
            <div className="empty-state-text">Добавьте первую группу кнопкой выше</div>
          </div>
        </div>
      ) : (
        <>
          {subgroups.map(sg => (
            <GroupCard
              key={sg.id}
              group={sg}
              groupId={groupId}
              allTourists={tourists}
              fileCounts={fileCounts}
              onReload={load}
              onFilesChanged={reloadFileCounts}
              onTouristDeleted={handleTouristDeleted}
              onRenamed={handleRenamed}
              onDeleted={handleDeleted}
            />
          ))}

          {/* Unassigned tourists */}
          {unassigned.length > 0 && (
            <div style={{ marginTop: 8 }}>
              <div style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500, marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Без группы ({unassigned.length})
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {unassigned.map(t => (
                  <TouristCard
                    key={t.id}
                    tourist={t}
                    subgroups={subgroups}
                    fileCount={t.submission_id ? (fileCounts[t.submission_id] || 0) : 0}
                    onFilesChanged={reloadFileCounts}
                    onUpdated={load}
                    onAssign={async (touristId, sgId) => {
                      try {
                        await assignTouristSubgroup(touristId, sgId);
                        setTourists(prev => prev.map(x => x.id === touristId ? { ...x, subgroup_id: sgId } : x));
                      } catch (e) { setError(e.message); }
                    }}
                    onDelete={async () => {
                      try { await deleteTourist(t.id); handleTouristDeleted(t.id); }
                      catch (e) { setError(e.message); }
                    }}
                  />
                ))}
              </div>
            </div>
          )}
        </>
      )}

    </div>
  );
}

// ── SubgroupHotelsSection ─────────────────────────────────────────────────────

const SubgroupHotelsSection = forwardRef(function SubgroupHotelsSection({ subgroupId, tourists = [] }, ref) {
  const [groupHotels, setGroupHotels] = useState([]);
  const [allHotels, setAllHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ hotel_id: '', check_in: '', check_out: '' });
  const [showAddCard, setShowAddCard] = useState(false);
  const [uploadingVoucher, setUploadingVoucher] = useState(false);
  const [voucherProgress, setVoucherProgress] = useState(null);
  const [voucherError, setVoucherError] = useState(null);
  const voucherInputRef = useRef(null);
  // Remembers the server-confirmed value of a date field at the moment the user
  // focused it, so onBlur can decide whether to persist and what to roll back to.
  const dateEditStartRef = useRef({});

  // Voucher parser writes group_hotels for the subgroup using a tourist as the
  // upload target (their group_id + subgroup_id scope the inserted rows).
  // Pick any tourist in this subgroup; if there are none, the button is hidden.
  const uploadAnchor = tourists[0]?.id || null;

  const loadAll = useCallback(async () => {
    try {
      const [hotels, current] = await Promise.all([getHotels(), getSubgroupHotels(subgroupId)]);
      setAllHotels(Array.isArray(hotels) ? hotels : []);
      setGroupHotels(Array.isArray(current) ? current : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [subgroupId]);

  useEffect(() => { loadAll(); }, [loadAll]);

  const selectedHotel = allHotels.find(h => String(h.id) === String(form.hotel_id));

  // Persist the given hotel array to the backend immediately.
  // On success: re-sync local state from the server response.
  // On failure: roll back to the previous state and surface the error.
  const persist = useCallback(async (nextHotels, prevHotels) => {
    setSaving(true);
    setError(null);
    try {
      await saveSubgroupHotels(subgroupId, nextHotels);
      // Re-fetch so we pick up any normalization (e.g. hotel name/address from DB).
      const current = await getSubgroupHotels(subgroupId);
      setGroupHotels(Array.isArray(current) ? current : []);
    } catch (e) {
      setError(e.message);
      // Roll back optimistic update.
      setGroupHotels(prevHotels);
    } finally {
      setSaving(false);
    }
  }, [subgroupId]);

  const handleAdd = () => {
    if (saving) return;
    if (!form.hotel_id || !form.check_in || !form.check_out) return;
    const hotel = allHotels.find(h => String(h.id) === String(form.hotel_id));
    if (!hotel) return;
    const prev = groupHotels;
    const next = [...prev, {
      hotel_id: hotel.id, hotel_name: hotel.name_en,
      city: hotel.city, address: hotel.address, phone: hotel.phone,
      check_in: form.check_in, check_out: form.check_out, sort_order: prev.length,
    }];
    setGroupHotels(next);                                    // optimistic
    setForm({ hotel_id: '', check_in: '', check_out: '' });
    setShowAddCard(false);
    persist(next, prev);
  };

  const handleRemove = (idx) => {
    if (saving) return;
    const prev = groupHotels;
    const next = prev.filter((_, i) => i !== idx).map((h, i) => ({ ...h, sort_order: i }));
    setGroupHotels(next);
    persist(next, prev);
  };

  const handleMoveUp = (idx) => {
    if (saving || idx === 0) return;
    const prev = groupHotels;
    const arr = [...prev];
    [arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]];
    const next = arr.map((h, i) => ({ ...h, sort_order: i }));
    setGroupHotels(next);
    persist(next, prev);
  };

  // Remember server value when the user starts editing.
  const handleDateFocus = (idx, field) => {
    dateEditStartRef.current[`${idx}:${field}`] = groupHotels[idx]?.[field] || '';
  };

  // Update local state only — do NOT persist on every keystroke, or the input
  // gets disabled mid-edit and loses focus.
  const handleDateChange = (idx, field, value) => {
    setGroupHotels(prev => prev.map((h, i) => i === idx ? { ...h, [field]: value } : h));
  };

  // Persist on blur, but only if the value actually changed vs. what the server had.
  const handleDateBlur = (idx, field) => {
    const key = `${idx}:${field}`;
    const original = dateEditStartRef.current[key] || '';
    delete dateEditStartRef.current[key];
    const current = groupHotels[idx]?.[field] || '';
    if (!current || current === original) return;
    const prevState = groupHotels.map((h, i) => i === idx ? { ...h, [field]: original } : h);
    persist(groupHotels, prevState);
  };

  const handleMoveDown = (idx) => {
    if (saving || idx >= groupHotels.length - 1) return;
    const prev = groupHotels;
    const arr = [...prev];
    [arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]];
    const next = arr.map((h, i) => ({ ...h, sort_order: i }));
    setGroupHotels(next);
    persist(next, prev);
  };

  const handleVoucherUpload = async (files) => {
    const list = files ? Array.from(files) : [];
    if (list.length === 0 || !uploadAnchor) return;
    setUploadingVoucher(true);
    setVoucherError(null);
    setVoucherProgress({ done: 0, total: list.length });
    try {
      for (let i = 0; i < list.length; i++) {
        const up = await uploadTouristFile(uploadAnchor, list[i], 'voucher');
        const res = await parseTouristUpload(uploadAnchor, up.id);
        if (res?.parse_error) setVoucherError(formatAIError({ message: res.parse_error }));
        setVoucherProgress({ done: i + 1, total: list.length });
      }
      // Reload subgroup hotels — voucher parser inserts rows into group_hotels.
      await loadAll();
    } catch (e) {
      setVoucherError(formatAIError(e));
    } finally {
      setUploadingVoucher(false);
      setVoucherProgress(null);
    }
  };

  // Triggered from elsewhere (e.g. tourist's documents modal) — parses
  // already-uploaded tourist_uploads and refreshes the hotel list.
  const parseExistingVouchers = async (anchorTouristId, uploadIds) => {
    if (!anchorTouristId || !Array.isArray(uploadIds) || uploadIds.length === 0) return;
    setUploadingVoucher(true);
    setVoucherError(null);
    setVoucherProgress({ done: 0, total: uploadIds.length });
    try {
      for (let i = 0; i < uploadIds.length; i++) {
        const res = await parseTouristUpload(anchorTouristId, uploadIds[i]);
        if (res?.parse_error) setVoucherError(formatAIError({ message: res.parse_error }));
        setVoucherProgress({ done: i + 1, total: uploadIds.length });
      }
      await loadAll();
    } catch (e) {
      setVoucherError(formatAIError(e));
    } finally {
      setUploadingVoucher(false);
      setVoucherProgress(null);
    }
  };

  useImperativeHandle(ref, () => ({
    parseExisting: parseExistingVouchers,
  }));

  if (loading) return <div className="loading-center"><div className="spinner" /></div>;

  return (
    <div>
      {error && <div className="error-message">{error}</div>}

      {/* Voucher upload — parses and auto-fills hotels */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
        <button
          type="button"
          className="btn btn-ghost btn-sm btn-magic"
          onClick={() => voucherInputRef.current?.click()}
          disabled={uploadingVoucher || !uploadAnchor}
          title={uploadAnchor
            ? 'Загрузить скан ваучера — отели распознаются автоматически'
            : 'Сначала добавьте туриста в группу'}
          aria-label="Заполнить отели из скана ваучера"
          style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
        >
          {uploadingVoucher ? (
            <>
              <span className="spinner" />
              {voucherProgress && voucherProgress.total > 1 && (
                <span style={{ fontSize: 10, marginLeft: 4, fontVariantNumeric: 'tabular-nums' }}>
                  {voucherProgress.done}/{voucherProgress.total}
                </span>
              )}
            </>
          ) : <MagicIcon />}
        </button>
        {voucherError && (
          <span style={{ fontSize: 11, color: 'var(--danger)', whiteSpace: 'pre-wrap' }}>{voucherError}</span>
        )}
        <input
          ref={voucherInputRef}
          type="file"
          accept=".pdf,.jpg,.jpeg,.png"
          multiple
          style={{ display: 'none' }}
          onChange={(e) => { handleVoucherUpload(e.target.files); e.target.value = ''; }}
        />
      </div>

      {/* Hotel list (from AI / manually added) */}
      {groupHotels.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 14 }}>
          {groupHotels.map((h, idx) => (
            <div key={idx} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', background: 'var(--gray-dark)', border: '1px solid var(--border)', borderRadius: 8 }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 24 }}>
                <button style={{ background: 'none', border: 'none', color: idx === 0 || saving ? 'var(--border)' : 'var(--white-dim)', cursor: idx === 0 || saving ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }} onClick={() => handleMoveUp(idx)} disabled={idx === 0 || saving}>▲</button>
                <button style={{ background: 'none', border: 'none', color: idx === groupHotels.length - 1 || saving ? 'var(--border)' : 'var(--white-dim)', cursor: idx === groupHotels.length - 1 || saving ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }} onClick={() => handleMoveDown(idx)} disabled={idx === groupHotels.length - 1 || saving}>▼</button>
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 500, marginBottom: 3, fontSize: 13 }}>
                  {h.hotel_name}
                  {h.city && <span style={{ color: 'var(--accent)', marginLeft: 8, fontSize: 11, fontWeight: 500 }}>{normalizeCity(h.city)}</span>}
                </div>
                <div style={{ fontSize: 12, color: 'var(--white-dim)', display: 'flex', gap: 14, flexWrap: 'wrap', alignItems: 'center' }}>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: 'var(--font-mono)' }}>
                    <input
                      type="date"
                      value={h.check_in || ''}
                      onFocus={() => handleDateFocus(idx, 'check_in')}
                      onChange={e => handleDateChange(idx, 'check_in', e.target.value)}
                      onBlur={() => handleDateBlur(idx, 'check_in')}
                      className="hotel-date-input"
                    />
                    →
                    <input
                      type="date"
                      value={h.check_out || ''}
                      onFocus={() => handleDateFocus(idx, 'check_out')}
                      onChange={e => handleDateChange(idx, 'check_out', e.target.value)}
                      onBlur={() => handleDateBlur(idx, 'check_out')}
                      className="hotel-date-input"
                    />
                  </span>
                  {h.address && <span>{h.address}</span>}
                  {h.phone && <span>{h.phone}</span>}
                </div>
              </div>
              <button
                type="button"
                onClick={() => handleRemove(idx)}
                disabled={saving}
                title="Удалить"
                aria-label="Удалить"
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: saving ? 'default' : 'pointer',
                  color: saving ? 'var(--border)' : 'var(--white-dim)',
                  lineHeight: 1,
                  padding: '4px 6px',
                  borderRadius: 4,
                  transition: 'color 0.15s, background 0.15s',
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
                onMouseEnter={e => { if (saving) return; e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                onMouseLeave={e => { if (saving) return; e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
              ><CrossIcon /></button>
            </div>
          ))}
        </div>
      )}

      {/* Add hotel — collapsible */}
      {!showAddCard ? (
        <div style={{ display: 'flex', justifyContent: 'flex-start', marginBottom: 14 }}>
          <button className="btn btn-secondary btn-sm" onClick={() => setShowAddCard(true)}>
            + Добавить отель
          </button>
        </div>
      ) : (
        <div className="card" style={{ marginBottom: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 14 }}>
            <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--white-dim)' }}>Добавить отель</div>
            <button
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--white-dim)', fontSize: 14, padding: '2px 6px' }}
              onClick={() => { setShowAddCard(false); setForm({ hotel_id: '', check_in: '', check_out: '' }); }}
              title="Закрыть"
            >✕</button>
          </div>
          <div className="form-group" style={{ marginBottom: 12 }}>
            <label className="form-label">Отель</label>
            <select className="form-input" value={form.hotel_id} onChange={e => setForm(f => ({ ...f, hotel_id: e.target.value }))}>
              <option value="">— выберите отель —</option>
              {allHotels.map(h => <option key={h.id} value={h.id}>{h.name_en} ({normalizeCity(h.city)})</option>)}
            </select>
          </div>

          {selectedHotel && (
            <div style={{ padding: '9px 12px', background: 'var(--graphite)', borderRadius: 6, marginBottom: 12, fontSize: 12, color: 'var(--white-dim)', display: 'flex', gap: 20, flexWrap: 'wrap' }}>
              <span>{selectedHotel.address || '—'}</span>
              <span>{selectedHotel.phone || '—'}</span>
            </div>
          )}

          <div className="grid-2">
            <div className="form-group" style={{ marginBottom: 0 }}>
              <label className="form-label">Check-in</label>
              <input className="form-input" type="date" value={form.check_in} onChange={e => setForm(f => ({ ...f, check_in: e.target.value }))} />
            </div>
            <div className="form-group" style={{ marginBottom: 0 }}>
              <label className="form-label">Check-out</label>
              <input className="form-input" type="date" value={form.check_out} onChange={e => setForm(f => ({ ...f, check_out: e.target.value }))} />
            </div>
          </div>

          <div style={{ marginTop: 14, display: 'flex', justifyContent: 'flex-end' }}>
            <button className="btn btn-secondary" onClick={handleAdd} disabled={saving || !form.hotel_id || !form.check_in || !form.check_out}>
              + Добавить
            </button>
          </div>
        </div>
      )}

      {saving && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 8, fontSize: 11, color: 'var(--white-dim)' }}>
          <span className="spinner" /> Сохранение...
        </div>
      )}
    </div>
  );
});

// ── DocumentsTab ──────────────────────────────────────────────────────────────

// Per-subgroup free-text hints that feed into programme generation.
// Autosaves on blur. Lives inside SubgroupDocsRow so each subgroup can
// describe its own itinerary preferences independently.
function SubgroupProgrammeNotes({ subgroupId, initial }) {
  const [value, setValue] = useState(initial || '');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState(null);
  const [lastSaved, setLastSaved] = useState(initial || '');

  const handleBlur = async () => {
    if (lastSaved === value) return;
    setSaving(true);
    setError(null);
    try {
      await updateSubgroupProgrammeNotes(subgroupId, value);
      setLastSaved(value);
      setSaved(true);
      setTimeout(() => setSaved(false), 1800);
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div style={{ marginTop: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
        <div style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500 }}>
          Пожелания по программе
        </div>
        <div style={{ fontSize: 11, color: 'var(--white-dim)', minHeight: 14 }}>
          {saving && 'Сохранение…'}
          {!saving && saved && 'Сохранено'}
          {!saving && error && <span style={{ color: 'var(--danger)' }}>{error}</span>}
        </div>
      </div>
      <textarea
        className="form-input"
        rows={3}
        value={value}
        onChange={e => setValue(e.target.value)}
        onBlur={handleBlur}
        placeholder="Например: 3 день — чайная церемония, без экскурсий в трансферные дни"
        style={{ width: '100%', resize: 'vertical', minHeight: 64, fontFamily: 'var(--font-body)' }}
      />
    </div>
  );
}

// JustRegeneratedBadge — small accent chip placed next to the timestamp.
// Self-fades via the parent's setTimeout; this component is purely visual.
function JustRegeneratedBadge() {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        padding: '2px 8px',
        borderRadius: 10,
        background: 'rgba(127, 183, 126, 0.18)',
        color: 'var(--accent)',
        border: '1px solid var(--accent)',
        fontSize: 10,
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: '0.05em',
      }}
    >
      new
    </span>
  );
}

function SubgroupDocsRow({ subgroup }) {
  // Start with server-persisted state: if a ZIP exists on disk, show it.
  const [hasZip, setHasZip] = useState(!!subgroup.has_zip);
  const [generatedAt, setGeneratedAt] = useState(subgroup.generated_at || null);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState(null);
  // justRegenerated drives the "только что обновлено" badge + accent
  // border highlight after a successful regeneration. Auto-clears after
  // 30 s via the effect below so the visual cue fades on its own.
  const [justRegenerated, setJustRegenerated] = useState(false);

  useEffect(() => {
    if (!justRegenerated) return undefined;
    const t = setTimeout(() => setJustRegenerated(false), 30000);
    return () => clearTimeout(t);
  }, [justRegenerated]);

  const handleGenerate = async () => {
    setGenerating(true);
    setError(null);
    try {
      const res = await generateSubgroupDocuments(subgroup.id);
      setGeneratedAt(res.generated_at);
      setHasZip(true);
      setJustRegenerated(true);
    } catch (e) {
      setError(formatAIError(e));
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div
      className="card"
      style={{
        marginBottom: 12,
        padding: '14px 18px',
        border: justRegenerated ? '1px solid var(--accent)' : undefined,
        boxShadow: justRegenerated ? '0 0 0 2px rgba(127, 183, 126, 0.15)' : undefined,
        transition: 'border-color 0.3s ease, box-shadow 0.3s ease',
      }}
    >
      <div className="doc-card-row">
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--white)' }}>{subgroup.name}</div>
          {hasZip && generatedAt && (
            <div style={{ fontSize: 11, color: 'var(--white-dim)', marginTop: 3 }}>
              Сгенерировано: {formatDate(generatedAt)}
            </div>
          )}
          {error && (
            <div style={{ fontSize: 11, color: 'var(--danger)', marginTop: 3 }}>{error}</div>
          )}
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          {justRegenerated && <JustRegeneratedBadge />}
          <button
            className="btn btn-sm btn-magic"
            onClick={handleGenerate}
            disabled={generating}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
          >
            {generating
              ? <><span className="spinner" /> Генерация...</>
              : <><MagicIcon /> {hasZip ? 'Перегенерировать' : 'Сгенерировать'}</>}
          </button>
          {hasZip && (
            <a
              href={getSubgroupDownloadUrl(subgroup.id)}
              className="btn btn-secondary btn-sm"
              target="_blank"
              rel="noreferrer"
              download
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
            >
              <FolderIcon /> Скачать ZIP
            </a>
          )}
        </div>
      </div>
      <SubgroupProgrammeNotes
        subgroupId={subgroup.id}
        initial={subgroup.programme_notes || ''}
      />
    </div>
  );
}

const LOCKED_STATUSES = ['docs_ready', 'submitted', 'visa_issued'];

function DocumentsTab({ groupId, group, onGroupUpdated }) {
  const [subgroups, setSubgroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [finalHasZip, setFinalHasZip] = useState(false);
  const [finalGeneratedAt, setFinalGeneratedAt] = useState(null);
  const [finalJustRegenerated, setFinalJustRegenerated] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [finalError, setFinalError] = useState(null);

  useEffect(() => {
    if (!finalJustRegenerated) return undefined;
    const t = setTimeout(() => setFinalJustRegenerated(false), 30000);
    return () => clearTimeout(t);
  }, [finalJustRegenerated]);
  // Default submission date = tomorrow (YYYY-MM-DD for <input type="date">).
  const [submissionDate, setSubmissionDate] = useState(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return d.toISOString().slice(0, 10);
  });
  const [markingReady, setMarkingReady] = useState(false);
  const [markReadyError, setMarkReadyError] = useState(null);

  const currentStatus = group?.status || 'draft';
  const alreadyReady = LOCKED_STATUSES.includes(currentStatus);

  const handleMarkReady = async () => {
    if (alreadyReady) return;
    setMarkingReady(true);
    setMarkReadyError(null);
    try {
      const updated = await updateGroupStatus(groupId, 'docs_ready');
      onGroupUpdated?.(updated);
    } catch (e) {
      setMarkReadyError(e.message);
    } finally {
      setMarkingReady(false);
    }
  };

  useEffect(() => {
    (async () => {
      try {
        const [sgs, fstat] = await Promise.all([
          getSubgroups(groupId),
          getFinalStatus(groupId).catch(() => ({ has_zip: false })),
        ]);
        setSubgroups(Array.isArray(sgs) ? sgs : []);
        setFinalHasZip(!!fstat.has_zip);
        setFinalGeneratedAt(fstat.generated_at || null);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, [groupId]);

  const handleFinalize = async () => {
    setFinalizing(true);
    setFinalError(null);
    try {
      const res = await finalizeGroup(groupId, submissionDate);
      setFinalHasZip(true);
      setFinalGeneratedAt(res?.generated_at || new Date().toISOString());
      setFinalJustRegenerated(true);
    } catch (e) {
      setFinalError(formatAIError(e));
    } finally {
      setFinalizing(false);
    }
  };

  if (loading) return <div className="loading-center"><div className="spinner spinner-lg" /></div>;

  return (
    <div>
      <div className="section-header">
        <div className="section-title">Документы по группам</div>
      </div>
      <div style={{ fontSize: 12, color: 'var(--white-dim)', marginBottom: 16 }}>
        Программа, доверенность, анкета — отдельный ZIP для каждой группы.
      </div>

      {error && <div className="error-message">{error}</div>}

      {subgroups.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="empty-state-title">Нет групп</div>
            <div className="empty-state-text">Создайте группу во вкладке «Группы»</div>
          </div>
        </div>
      ) : (
        <div style={{ marginBottom: 32 }}>
          {subgroups.map(sg => (
            <SubgroupDocsRow key={sg.id} subgroup={sg} />
          ))}
        </div>
      )}

      <div className="section-header">
        <div className="section-title">Финальные документы</div>
      </div>
      <div style={{ fontSize: 12, color: 'var(--white-dim)', marginBottom: 16 }}>
        Приложение на оплату и списки.
      </div>

      {finalError && <div className="error-message">{finalError}</div>}

      <div
        className="card"
        style={{
          marginBottom: 12,
          padding: '14px 18px',
          border: finalJustRegenerated ? '1px solid var(--accent)' : undefined,
          boxShadow: finalJustRegenerated ? '0 0 0 2px rgba(127, 183, 126, 0.15)' : undefined,
          transition: 'border-color 0.3s ease, box-shadow 0.3s ease',
        }}
      >
        <div className="doc-card-row">
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: 10, fontSize: 13, color: 'var(--white)', fontWeight: 500, flexWrap: 'wrap' }}>
            Дата подачи:
            <input
              type="date"
              className="form-input"
              value={submissionDate}
              onChange={e => setSubmissionDate(e.target.value)}
              aria-label="Дата подачи"
              style={{ fontSize: 13, padding: '6px 10px', height: 32 }}
            />
          </label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            {finalJustRegenerated && <JustRegeneratedBadge />}
            <button
              className="btn btn-sm btn-magic"
              onClick={handleFinalize}
              disabled={finalizing}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
            >
              {finalizing
                ? <><span className="spinner" /> Генерация...</>
                : <><MagicIcon /> {finalHasZip ? 'Перегенерировать' : 'Сгенерировать'}</>}
            </button>
            {finalHasZip && (
              <a
                href={getFinalDownloadUrl(groupId)}
                className="btn btn-secondary btn-sm"
                target="_blank"
                rel="noreferrer"
                download
                style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
              >
                <FolderIcon /> Скачать ZIP
              </a>
            )}
          </div>
        </div>
        {finalHasZip && finalGeneratedAt && (
          <div style={{ fontSize: 11, color: 'var(--white-dim)', marginTop: 8 }}>
            Сгенерировано: {formatDate(finalGeneratedAt)}
          </div>
        )}
      </div>

      {/* Final confirmation — compact toggle */}
      {(() => {
        const isReady = group?.status === 'docs_ready'
          || group?.status === 'submitted'
          || group?.status === 'visa_issued';
        const toggle = async (nextStatus) => {
          if (markingReady) return;
          if ((nextStatus === 'docs_ready' && isReady) ||
              (nextStatus === 'in_progress' && group?.status === 'in_progress')) return;
          setMarkingReady(true);
          setMarkReadyError(null);
          try {
            const updated = await updateGroupStatus(groupId, nextStatus);
            onGroupUpdated?.(updated);
          } catch (e) {
            setMarkReadyError(e.message);
          } finally {
            setMarkingReady(false);
          }
        };
        const iconBtn = (active, activeColor, onClick, children, title) => (
          <button
            type="button"
            onClick={onClick}
            disabled={markingReady}
            title={title}
            aria-label={title}
            style={{
              width: 32,
              height: 32,
              borderRadius: '50%',
              border: `1px solid ${active ? activeColor : 'var(--border)'}`,
              background: active ? `${activeColor}1f` : 'transparent',
              color: active ? activeColor : 'var(--white-dim)',
              cursor: markingReady ? 'default' : 'pointer',
              fontSize: 15,
              fontWeight: 700,
              lineHeight: 1,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              opacity: active ? 1 : 0.5,
              transition: 'opacity 0.15s, background 0.15s, color 0.15s, border-color 0.15s',
              padding: 0,
            }}
            onMouseEnter={e => { if (!markingReady) e.currentTarget.style.opacity = '1'; }}
            onMouseLeave={e => { if (!markingReady) e.currentTarget.style.opacity = active ? '1' : '0.5'; }}
          >{children}</button>
        );
        return (
          <div style={{
            marginTop: 20,
            padding: '12px 16px',
            background: 'var(--graphite)',
            border: '1px solid var(--border)',
            borderRadius: 8,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 14,
            flexWrap: 'wrap',
          }}>
            <span style={{ fontSize: 13, fontWeight: 500, color: 'var(--white)' }}>
              Все сгенерировано правильно?
            </span>
            <div style={{ display: 'flex', gap: 8 }}>
              {iconBtn(isReady, '#3b82f6', () => toggle('docs_ready'), '✓', 'Документы готовы')}
              {iconBtn(group?.status === 'in_progress', '#f59e0b', () => toggle('in_progress'), '✕', 'В процессе')}
            </div>
          </div>
        );
      })()}
      {markReadyError && (
        <div className="error-message" style={{ marginTop: 8 }}>{markReadyError}</div>
      )}
    </div>
  );
}

// ── Main Component ────────────────────────────────────────────────────────────

// ── SettingsTab ───────────────────────────────────────────────────────────────

// SubgroupsManager — list of all subgroups inside the current submission
// with inline rename + delete. Lives on the Settings tab so that destructive
// actions are tucked away from the main "Группы" working view.
function SubgroupsManager({ groupId }) {
  const [subgroups, setSubgroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [editingId, setEditingId] = useState(null);
  const [editName, setEditName] = useState('');
  const [busyId, setBusyId] = useState(null);
  const [confirmTarget, setConfirmTarget] = useState(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getSubgroups(groupId);
      setSubgroups(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [groupId]);

  useEffect(() => { load(); }, [load]);

  const startEdit = (sg) => {
    setEditingId(sg.id);
    setEditName(sg.name);
  };
  const cancelEdit = () => {
    setEditingId(null);
    setEditName('');
  };
  const saveEdit = async (sg) => {
    const trimmed = editName.trim();
    if (!trimmed || trimmed === sg.name) { cancelEdit(); return; }
    setBusyId(sg.id);
    setError(null);
    try {
      await updateSubgroup(sg.id, trimmed);
      setSubgroups(prev => prev.map(s => s.id === sg.id ? { ...s, name: trimmed } : s));
      cancelEdit();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusyId(null);
    }
  };

  const requestDelete = (sg) => setConfirmTarget(sg);
  const confirmDelete = async () => {
    if (!confirmTarget) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteSubgroup(confirmTarget.id);
      setSubgroups(prev => prev.filter(s => s.id !== confirmTarget.id));
      setConfirmTarget(null);
    } catch (e) {
      setError(e.message);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div style={{
      marginBottom: 18,
      padding: '14px 16px',
      background: 'var(--graphite)',
      border: '1px solid var(--border)',
      borderRadius: 8,
    }}>
      <div style={{
        fontFamily: 'var(--font-mono)',
        fontSize: 10,
        letterSpacing: '0.05em',
        textTransform: 'uppercase',
        color: 'var(--white-dim)',
        marginBottom: 10,
      }}>
        Группы внутри подачи
      </div>

      {error && <div className="error-message" style={{ marginBottom: 10 }}>{error}</div>}

      {loading ? (
        <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>Загрузка…</div>
      ) : subgroups.length === 0 ? (
        <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>В этой подаче пока нет групп.</div>
      ) : (
        <div style={{
          border: '1px solid var(--border)',
          borderRadius: 6,
          overflow: 'hidden',
          background: 'var(--gray-dark)',
        }}>
          {subgroups.map((sg, i) => (
            <div
              key={sg.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '10px 12px',
                borderTop: i === 0 ? 'none' : '1px solid var(--border)',
              }}
            >
              {editingId === sg.id ? (
                <>
                  <input
                    className="form-input"
                    style={{ flex: 1, fontSize: 13, padding: '4px 8px' }}
                    value={editName}
                    onChange={e => setEditName(e.target.value)}
                    onKeyDown={e => {
                      if (e.key === 'Enter') saveEdit(sg);
                      if (e.key === 'Escape') cancelEdit();
                    }}
                    autoFocus
                  />
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    onClick={() => saveEdit(sg)}
                    disabled={busyId === sg.id || !editName.trim()}
                  >
                    {busyId === sg.id ? <span className="spinner" /> : 'OK'}
                  </button>
                  <button
                    type="button"
                    className="btn btn-secondary btn-sm"
                    onClick={cancelEdit}
                    disabled={busyId === sg.id}
                  >
                    Отмена
                  </button>
                </>
              ) : (
                <>
                  <span style={{ flex: 1, fontSize: 13, color: 'var(--white)' }}>{sg.name}</span>
                  <button
                    type="button"
                    onClick={() => startEdit(sg)}
                    title="Переименовать"
                    aria-label="Переименовать"
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      color: 'var(--white-dim)',
                      fontSize: 14,
                      padding: '4px 6px',
                      borderRadius: 4,
                      transition: 'color 0.15s, background 0.15s',
                    }}
                    onMouseEnter={e => { e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                    onMouseLeave={e => { e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
                  >✎</button>
                  <button
                    type="button"
                    onClick={() => requestDelete(sg)}
                    title="Удалить группу"
                    aria-label="Удалить группу"
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      color: 'var(--white-dim)',
                      lineHeight: 1,
                      padding: '4px 6px',
                      borderRadius: 4,
                      transition: 'color 0.15s, background 0.15s',
                      display: 'inline-flex',
                      alignItems: 'center',
                    }}
                    onMouseEnter={e => { e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                    onMouseLeave={e => { e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
                  ><TrashIcon /></button>
                </>
              )}
            </div>
          ))}
        </div>
      )}

      <Modal
        open={!!confirmTarget}
        onClose={() => !deleting && setConfirmTarget(null)}
        title="Удалить группу?"
        width={440}
      >
        <div style={{ fontSize: 13, color: 'var(--white)', marginBottom: 16, lineHeight: 1.5 }}>
          Удалить группу <strong>«{confirmTarget?.name}»</strong>? Туристы останутся в подаче без группы.
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => setConfirmTarget(null)}
            disabled={deleting}
          >
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={confirmDelete}
            disabled={deleting}
          >
            {deleting ? <><span className="spinner" /> Удаление…</> : 'Удалить'}
          </button>
        </div>
      </Modal>
    </div>
  );
}

function SettingsTab({ group, groupId, onDeleted, onGroupUpdated }) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState(null);

  // Rename state
  const [name, setName] = useState(group?.name || '');
  const [renaming, setRenaming] = useState(false);
  const [renameError, setRenameError] = useState(null);
  const [renameSaved, setRenameSaved] = useState(false);

  useEffect(() => { setName(group?.name || ''); }, [group?.id, group?.name]);

  const handleRename = async () => {
    const trimmed = name.trim();
    if (!trimmed || trimmed === group?.name) return;
    setRenaming(true);
    setRenameError(null);
    setRenameSaved(false);
    try {
      const updated = await updateGroupName(group.id, trimmed);
      onGroupUpdated?.(updated);
      setRenameSaved(true);
      setTimeout(() => setRenameSaved(false), 1800);
    } catch (e) {
      setRenameError(e.message);
    } finally {
      setRenaming(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    setError(null);
    try {
      await deleteGroup(group.id);
      setConfirmOpen(false);
      onDeleted();
    } catch (e) {
      setError(e.message);
      setDeleting(false);
    }
  };

  const nameDirty = name.trim() !== (group?.name || '');

  return (
    <div>
      <div style={{
        marginBottom: 18,
        padding: '14px 16px',
        background: 'var(--graphite)',
        border: '1px solid var(--border)',
        borderRadius: 8,
      }}>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: 8,
        }}>
          <div style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            letterSpacing: '0.05em',
            textTransform: 'uppercase',
            color: 'var(--white-dim)',
          }}>
            Название подачи
          </div>
          <div style={{ fontSize: 11, color: 'var(--white-dim)', minHeight: 14 }}>
            {renaming && 'Сохранение…'}
            {!renaming && renameSaved && 'Сохранено'}
            {!renaming && renameError && (
              <span style={{ color: 'var(--danger)' }}>{renameError}</span>
            )}
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input
            className="form-input"
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleRename(); }}
            disabled={renaming}
            style={{ flex: 1, fontSize: 13 }}
          />
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={handleRename}
            disabled={renaming || !nameDirty || !name.trim()}
          >
            {renaming ? <span className="spinner" /> : 'Сохранить'}
          </button>
        </div>
      </div>

      <div style={{
        marginBottom: 18,
        padding: '12px 14px',
        background: 'var(--graphite)',
        border: '1px solid var(--border)',
        borderRadius: 8,
        fontSize: 12,
        color: 'var(--white-dim)',
        display: 'flex',
        gap: 8,
        alignItems: 'baseline',
      }}>
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 10,
          letterSpacing: '0.05em',
          textTransform: 'uppercase',
        }}>
          Создана:
        </span>
        <span style={{ color: 'var(--white)', fontFamily: 'var(--font-mono)' }}>
          {formatDate(group?.created_at)}
        </span>
      </div>

      {groupId && <SubgroupsManager groupId={groupId} />}

      <button
        type="button"
        onClick={() => { setConfirmOpen(true); setError(null); }}
        style={{
          background: 'none',
          border: '1px solid rgba(239, 68, 68, 0.4)',
          color: '#ef4444',
          fontSize: 13,
          fontWeight: 600,
          padding: '8px 16px',
          borderRadius: 6,
          cursor: 'pointer',
          transition: 'background 0.15s, border-color 0.15s',
        }}
        onMouseEnter={e => { e.currentTarget.style.background = 'rgba(239, 68, 68, 0.08)'; e.currentTarget.style.borderColor = '#ef4444'; }}
        onMouseLeave={e => { e.currentTarget.style.background = 'none'; e.currentTarget.style.borderColor = 'rgba(239, 68, 68, 0.4)'; }}
      >
        Удалить подачу
      </button>

      <Modal open={confirmOpen} onClose={() => !deleting && setConfirmOpen(false)} title="Удалить подачу?" width={440}>
        <div style={{ fontSize: 13, color: 'var(--white)', marginBottom: 16, lineHeight: 1.5 }}>
          Вы собираетесь удалить подачу <strong>«{group?.name}»</strong>. Это действие необратимо.
        </div>
        {error && <div className="error-message" style={{ marginBottom: 12 }}>{error}</div>}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => setConfirmOpen(false)}
            disabled={deleting}
          >
            Отмена
          </button>
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            style={{
              background: '#ef4444',
              border: 'none',
              color: '#fff',
              fontSize: 12,
              fontWeight: 600,
              padding: '7px 14px',
              borderRadius: 5,
              cursor: deleting ? 'default' : 'pointer',
              opacity: deleting ? 0.6 : 1,
              transition: 'opacity 0.15s',
            }}
          >
            {deleting ? <><span className="spinner" /> Удаление...</> : 'Удалить'}
          </button>
        </div>
      </Modal>
    </div>
  );
}

const TABS = [
  { id: 'groups', label: 'Группы' },
  { id: 'documents', label: 'Документы' },
  { id: 'status', label: 'Статус' },
  { id: 'settings', label: 'Настройки' },
];

export default function GroupDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [group, setGroup] = useState(null);
  const [tourists, setTourists] = useState([]);
  const [hotels, setHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState('groups');

  // getGroup returns { group, tourists, hotels } — we need tourists + hotels
  // at the top level to feed the workflow stepper.
  const loadGroup = useCallback(async () => {
    try {
      const data = await getGroup(id);
      setGroup(data?.group ?? data);
      setTourists(Array.isArray(data?.tourists) ? data.tourists : []);
      setHotels(Array.isArray(data?.hotels) ? data.hotels : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [id]);

  // Re-fetch when the user switches tabs so the stepper reflects changes
  // (tourists added, scans uploaded, documents generated) without a hard reload.
  useEffect(() => { loadGroup(); }, [loadGroup, activeTab]);

  if (loading) return (
    <div className="page-container">
      <div className="loading-center"><div className="spinner spinner-lg" /> Загрузка...</div>
    </div>
  );

  if (error) return (
    <div className="page-container">
      <div className="error-message">{error}</div>
      <button className="btn btn-ghost" onClick={() => navigate('/')}>← Назад</button>
    </div>
  );

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">{group?.name}</div>
          <div className="page-subtitle" style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 6 }}>
            <StatusBadge status={group?.status || 'draft'} />
          </div>
        </div>
      </div>

      <div className="tabs">
        {TABS.map(t => (
          <button key={t.id} className={`tab-btn${activeTab === t.id ? ' active' : ''}`} onClick={() => setActiveTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>

      {activeTab === 'status' && <StatusSection group={group} onGroupUpdated={setGroup} />}
      {activeTab === 'groups' && <GroupsTab groupId={id} />}
      {activeTab === 'documents' && (
        <DocumentsTab groupId={id} group={group} onGroupUpdated={setGroup} />
      )}
      {activeTab === 'settings' && (
        <SettingsTab
          group={group}
          groupId={id}
          onDeleted={() => navigate('/')}
          onGroupUpdated={setGroup}
        />
      )}
    </div>
  );
}
