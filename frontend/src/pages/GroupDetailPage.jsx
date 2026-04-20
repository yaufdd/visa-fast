import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getGroup,
  getHotels, getSubgroupHotels, saveSubgroupHotels,
  finalizeGroup, getFinalDownloadUrl, getFinalStatus,
  generateSubgroupDocuments, getSubgroupDownloadUrl,
  getTourists, deleteTourist,
  uploadTouristFile, getTouristUploads,
  getSubgroups, createSubgroup, updateSubgroup, deleteSubgroup,
  assignTouristSubgroup,
  updateGroupStatus, deleteGroup,
} from '../api/client';
import StatusSection from '../components/StatusSection';
import AddFromDBModal from '../components/AddFromDBModal';
import FlightDataCard from '../components/FlightDataCard';

// Folder-download icon.
const FolderIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
    <path d="M1.5 3.5a1 1 0 0 1 1-1h3.586a1 1 0 0 1 .707.293l1.414 1.414a1 1 0 0 0 .707.293H13.5a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V3.5Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
  </svg>
);
import StatusBadge from '../components/StatusBadge';
import Modal from '../components/Modal';

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short', year: 'numeric' });
}

// Read a field from the tourist's submission_snapshot (raw JSON blob from DB).
function snapshotOf(tourist) {
  const raw = tourist?.submission_snapshot;
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

function getTouristName(tourist) {
  const snap = snapshotOf(tourist);
  return snap.name_lat || snap.name_cyr || '—';
}

// ── TouristUploadsBar — per-tourist ticket/voucher uploads ───────────────────

function TouristUploadsBar({ touristId, onChanged }) {
  const [uploads, setUploads] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [uploadingType, setUploadingType] = useState(null);
  const ticketRef = useRef(null);
  const voucherRef = useRef(null);

  const load = useCallback(async () => {
    try {
      const data = await getTouristUploads(touristId);
      setUploads(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [touristId]);

  useEffect(() => { load(); }, [load]);

  const handleUpload = async (file, fileType) => {
    if (!file) return;
    setUploadingType(fileType);
    setError(null);
    try {
      const res = await uploadTouristFile(touristId, file, fileType);
      if (res?.parse_error) {
        setError(`Парсинг не удался: ${res.parse_error}`);
      }
      await load();
      onChanged?.();
    } catch (e) {
      setError(e.message);
    } finally {
      setUploadingType(null);
    }
  };

  const counts = uploads.reduce((acc, u) => {
    acc[u.file_type] = (acc[u.file_type] || 0) + 1;
    return acc;
  }, {});

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        flexWrap: 'wrap',
        gap: 8,
        marginTop: 10,
      }}
    >
      <span
        style={{
          fontSize: 11,
          color: 'var(--white-dim)',
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
        }}
      >
        Сканы:
      </span>
      <button
        type="button"
        className="btn btn-secondary btn-sm"
        onClick={() => ticketRef.current?.click()}
        disabled={loading || uploadingType === 'ticket'}
      >
        {uploadingType === 'ticket'
          ? <><span className="spinner" /> Билет...</>
          : `+ Билет${counts.ticket ? ` (${counts.ticket})` : ''}`}
      </button>
      <button
        type="button"
        className="btn btn-secondary btn-sm"
        onClick={() => voucherRef.current?.click()}
        disabled={loading || uploadingType === 'voucher'}
      >
        {uploadingType === 'voucher'
          ? <><span className="spinner" /> Ваучер...</>
          : `+ Ваучер${counts.voucher ? ` (${counts.voucher})` : ''}`}
      </button>
      <input
        ref={ticketRef}
        type="file"
        accept=".pdf,.jpg,.jpeg,.png"
        style={{ display: 'none' }}
        onChange={(e) => { handleUpload(e.target.files?.[0], 'ticket'); e.target.value = ''; }}
      />
      <input
        ref={voucherRef}
        type="file"
        accept=".pdf,.jpg,.jpeg,.png"
        style={{ display: 'none' }}
        onChange={(e) => { handleUpload(e.target.files?.[0], 'voucher'); e.target.value = ''; }}
      />
      {error && (
        <span style={{ fontSize: 11, color: 'var(--danger)', width: '100%' }}>
          {error}
        </span>
      )}
    </div>
  );
}

// ── TouristCard — full per-tourist card (name + flight + uploads) ────────────

function TouristCard({ tourist, onDelete, subgroups, onAssign, onUpdated }) {
  const name = getTouristName(tourist);
  const snap = snapshotOf(tourist);
  const dob = snap.birth_date || snap.date_of_birth;

  return (
    <div
      style={{
        padding: '12px 14px',
        background: 'var(--gray-dark)',
        border: '1px solid var(--border)',
        borderRadius: 8,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 9, minWidth: 0 }}>
          <div
            style={{
              width: 28, height: 28, borderRadius: '50%', background: 'var(--gray)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 11, fontWeight: 700, color: 'var(--white-dim)', flexShrink: 0,
            }}
          >
            {name.charAt(0) || '?'}
          </div>
          <div style={{ minWidth: 0 }}>
            <div
              style={{
                fontSize: 13,
                fontWeight: 500,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {name}
            </div>
            {dob && (
              <div
                style={{
                  fontSize: 11,
                  color: 'var(--white-dim)',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {dob}
              </div>
            )}
          </div>
        </div>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            flexShrink: 0,
          }}
        >
          {subgroups && subgroups.length > 0 && onAssign && (
            <select
              value=""
              onChange={(e) => e.target.value && onAssign(tourist.id, e.target.value)}
              style={{
                background: 'var(--gray)',
                border: '1px solid var(--border)',
                borderRadius: 5,
                color: 'var(--white-dim)',
                fontSize: 11,
                padding: '3px 6px',
                cursor: 'pointer',
              }}
            >
              <option value="">→ в группу</option>
              {subgroups.map((sg) => (
                <option key={sg.id} value={sg.id}>{sg.name}</option>
              ))}
            </select>
          )}
          <button
            type="button"
            onClick={onDelete}
            title="Удалить"
            aria-label="Удалить"
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              color: 'var(--white-dim)',
              fontSize: 14,
              lineHeight: 1,
              padding: '4px 6px',
              borderRadius: 4,
            }}
          >✕</button>
        </div>
      </div>

      <FlightDataCard tourist={tourist} onUpdated={onUpdated} />

      <TouristUploadsBar touristId={tourist.id} onChanged={onUpdated} />
    </div>
  );
}

// ── GroupCard ─────────────────────────────────────────────────────────────────

function GroupCard({ group, groupId, allTourists, onReload, onTouristDeleted, onRenamed, onDeleted }) {
  const [expanded, setExpanded] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(group.name);
  const [cardError, setCardError] = useState(null);
  const [hotelsExpanded, setHotelsExpanded] = useState(false);

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

  const handleDelete = async () => {
    if (!confirm(`Удалить группу "${group.name}"? Туристы останутся в подаче без группы.`)) return;
    try {
      await deleteSubgroup(group.id);
      onDeleted(group.id);
    } catch (e) {
      setCardError(e.message);
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

        <span style={{ fontSize: 12, color: 'var(--white-dim)', background: 'var(--gray)', padding: '2px 8px', borderRadius: 100 }}>
          {tourists.length} туристов
        </span>

        {editing ? (
          <>
            <button className="btn btn-primary btn-sm" onClick={e => { e.stopPropagation(); handleRename(); }}>OK</button>
            <button className="btn btn-secondary btn-sm" onClick={e => { e.stopPropagation(); setEditing(false); setEditName(group.name); }}>Отмена</button>
          </>
        ) : (
          <>
            <button
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--white-dim)', fontSize: 14, padding: '2px 6px' }}
              onClick={e => { e.stopPropagation(); setEditing(true); setEditName(group.name); }}
              title="Переименовать"
            >✎</button>
            <button
              type="button"
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                color: 'var(--white-dim)',
                fontSize: 14,
                lineHeight: 1,
                padding: '4px 6px',
                borderRadius: 4,
                transition: 'color 0.15s, background 0.15s',
              }}
              onClick={e => { e.stopPropagation(); handleDelete(); }}
              onMouseEnter={e => { e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
              onMouseLeave={e => { e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
              title="Удалить группу"
              aria-label="Удалить группу"
            >✕</button>
          </>
        )}
      </div>

      {/* Body */}
      {expanded && (
        <div style={{ padding: '16px 18px', display: 'flex', flexDirection: 'column', gap: 14 }}>
          {cardError && <div className="error-message">{cardError}</div>}

          {/* Tourist list */}
          <div>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: 8,
              }}
            >
              <span style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500 }}>
                Туристы
              </span>
              <button
                className="btn btn-secondary btn-sm"
                style={{ fontSize: 11 }}
                onClick={() => setShowAddModal(true)}
              >
                + Добавить туриста
              </button>
            </div>
            {tourists.length === 0 ? (
              <div
                style={{
                  padding: '10px 14px',
                  border: '1px dashed var(--border)',
                  borderRadius: 7,
                  fontSize: 12,
                  color: 'var(--white-dim)',
                  textAlign: 'center',
                }}
              >
                Нет туристов — добавьте из пула анкет
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {tourists.map((t) => (
                  <TouristCard
                    key={t.id}
                    tourist={t}
                    onUpdated={onReload}
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
              <SubgroupHotelsSection subgroupId={group.id} />
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
    </div>
  );
}

// ── GroupsTab ─────────────────────────────────────────────────────────────────

function GroupsTab({ groupId }) {
  const [subgroups, setSubgroups] = useState([]);
  const [tourists, setTourists] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showNewForm, setShowNewForm] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    try {
      const [sgs, ts] = await Promise.all([
        getSubgroups(groupId),
        getTourists(groupId),
      ]);
      setSubgroups(Array.isArray(sgs) ? sgs : []);
      setTourists(Array.isArray(ts) ? ts : []);
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
  const handleDeleted = (sgId) => setSubgroups(prev => prev.filter(sg => sg.id !== sgId));
  const handleTouristDeleted = (tid) => setTourists(prev => prev.filter(t => t.id !== tid));

  // Unassigned tourists
  const unassigned = tourists.filter(t => !t.subgroup_id);

  if (loading) return <div className="loading-center"><div className="spinner spinner-lg" /></div>;

  return (
    <div>
      {error && <div className="error-message" style={{ marginBottom: 14 }}>{error}</div>}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
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
              onReload={load}
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

function SubgroupHotelsSection({ subgroupId }) {
  const [groupHotels, setGroupHotels] = useState([]);
  const [allHotels, setAllHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ hotel_id: '', check_in: '', check_out: '' });
  const [showAddCard, setShowAddCard] = useState(false);
  // Remembers the server-confirmed value of a date field at the moment the user
  // focused it, so onBlur can decide whether to persist and what to roll back to.
  const dateEditStartRef = useRef({});

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
      hotel_id: hotel.id, hotel_name: hotel.name_en, hotel_name_ru: hotel.name_ru,
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

  if (loading) return <div className="loading-center"><div className="spinner" /></div>;

  return (
    <div>
      {error && <div className="error-message">{error}</div>}

      {/* Hotel list (from AI / manually added) */}
      {groupHotels.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '16px 0', color: 'var(--white-dim)', fontSize: 12 }}>
          Нет отелей — добавьте кнопкой ниже
        </div>
      ) : (
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
                  {h.hotel_name_ru && <span style={{ color: 'var(--white-dim)', marginLeft: 8, fontSize: 12 }}>/ {h.hotel_name_ru}</span>}
                  {h.city && <span style={{ color: 'var(--accent)', marginLeft: 8, fontSize: 11, fontWeight: 500 }}>{h.city}</span>}
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
                  fontSize: 14,
                  lineHeight: 1,
                  padding: '4px 6px',
                  borderRadius: 4,
                  transition: 'color 0.15s, background 0.15s',
                }}
                onMouseEnter={e => { if (saving) return; e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                onMouseLeave={e => { if (saving) return; e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
              >✕</button>
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
              {allHotels.map(h => <option key={h.id} value={h.id}>{h.name_en} ({h.city})</option>)}
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
}

// ── DocumentsTab ──────────────────────────────────────────────────────────────

function SubgroupDocsRow({ subgroup }) {
  // Start with server-persisted state: if a ZIP exists on disk, show it.
  const [hasZip, setHasZip] = useState(!!subgroup.has_zip);
  const [generatedAt, setGeneratedAt] = useState(subgroup.generated_at || null);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState(null);

  const handleGenerate = async () => {
    setGenerating(true);
    setError(null);
    try {
      const res = await generateSubgroupDocuments(subgroup.id);
      setGeneratedAt(res.generated_at);
      setHasZip(true);
    } catch (e) {
      setError(e.message);
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="card" style={{ marginBottom: 12, padding: '14px 18px' }}>
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
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleGenerate}
            disabled={generating}
          >
            {generating
              ? <><span className="spinner" /> Генерация...</>
              : hasZip ? 'Перегенерировать' : 'Сгенерировать'}
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
    </div>
  );
}

const LOCKED_STATUSES = ['docs_ready', 'submitted', 'visa_issued'];

function DocumentsTab({ groupId, group, onGroupUpdated }) {
  const [subgroups, setSubgroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [finalHasZip, setFinalHasZip] = useState(false);
  const [finalizing, setFinalizing] = useState(false);
  const [finalError, setFinalError] = useState(null);
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
      await finalizeGroup(groupId, submissionDate);
      setFinalHasZip(true);
    } catch (e) {
      setFinalError(e.message);
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

      <div className="card" style={{ marginBottom: 12, padding: '14px 18px' }}>
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
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button
              className="btn btn-primary btn-sm"
              onClick={handleFinalize}
              disabled={finalizing}
            >
              {finalizing
                ? <><span className="spinner" /> Генерация...</>
                : finalHasZip ? 'Перегенерировать' : 'Сгенерировать'}
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

function SettingsTab({ group, onDeleted }) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState(null);

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

  return (
    <div>
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
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState('groups');

  useEffect(() => {
    (async () => {
      try {
        const data = await getGroup(id);
        // getGroup returns { group, tourists, hotels } — unwrap to the flat group object
        // so downstream components (StatusSection, DocumentsTab) see group.id directly.
        setGroup(data?.group ?? data);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, [id]);

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
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
            <button
              onClick={() => navigate('/')}
              style={{ background: 'none', border: 'none', color: 'var(--white-dim)', fontSize: 13, cursor: 'pointer', padding: 0, display: 'flex', alignItems: 'center', gap: 4 }}
            >
              ← Подачи
            </button>
            <span style={{ color: 'var(--border)' }}>/</span>
            <span style={{ color: 'var(--white-dim)', fontSize: 13 }}>{group?.name}</span>
          </div>
          <div className="page-title">{group?.name}</div>
          <div className="page-subtitle" style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 6 }}>
            <StatusBadge status={group?.status || 'draft'} />
            <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>Создана: {formatDate(group?.created_at)}</span>
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
        <SettingsTab group={group} onDeleted={() => navigate('/')} />
      )}
    </div>
  );
}
