import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getGroup,
  getHotels, getSubgroupHotels, saveSubgroupHotels,
  finalizeGroup, getFinalDownloadUrl,
  generateSubgroupDocuments, getSubgroupDownloadUrl,
  getTourists, addTouristFromSheet, deleteTourist,
  uploadFile, getUploads,
  getSheetRows,
  getSubgroups, createSubgroup, updateSubgroup, deleteSubgroup,
  assignTouristSubgroup, parseSubgroup, parseGroup,
} from '../api/client';
import StatusBadge from '../components/StatusBadge';
import Modal from '../components/Modal';

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short', year: 'numeric' });
}

function basename(filePath) {
  if (!filePath) return '';
  return filePath.split('/').pop().split('\\').pop();
}

function getTouristName(tourist) {
  const row = tourist.matched_sheet_row || {};
  return (
    row['ФИО (латиницей, как в загранпаспорте)'] ||
    row['ФИО (латиницей)'] ||
    row['ФИО латиницей'] ||
    tourist.raw_json?.name_lat ||
    Object.values(row)[0] ||
    '—'
  );
}

// ── AddFromSheetModal ─────────────────────────────────────────────────────────

function AddFromSheetModal({ groupId, subgroupId, onAdded, onClose }) {
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selected, setSelected] = useState(new Set());
  const [adding, setAdding] = useState(false);
  const debounceRef = useRef(null);

  const loadRows = useCallback((q) => {
    setLoading(true);
    setError(null);
    getSheetRows(q)
      .then(data => setRows(Array.isArray(data) ? data.map(i => i.row ?? i) : []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { loadRows(''); }, [loadRows]);

  useEffect(() => {
    if (!query.trim()) { loadRows(''); return; }
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => loadRows(query), 350);
    return () => clearTimeout(debounceRef.current);
  }, [query, loadRows]);

  const toggleRow = (key) => setSelected(prev => {
    const next = new Set(prev);
    next.has(key) ? next.delete(key) : next.add(key);
    return next;
  });

  const handleAdd = async () => {
    if (selected.size === 0) return;
    setAdding(true);
    setError(null);
    try {
      for (const k of selected) {
        const row = JSON.parse(k);
        await addTouristFromSheet(groupId, row, subgroupId);
      }
      onAdded();
      onClose();
    } catch (e) {
      setError(e.message);
      setAdding(false);
    }
  };

  return (
    <div>
      <div className="form-group" style={{ marginBottom: 16 }}>
        <input
          className="form-input"
          type="text"
          placeholder="Поиск по имени..."
          value={query}
          onChange={e => setQuery(e.target.value)}
          autoFocus
        />
      </div>
      {error && <div className="error-message" style={{ marginBottom: 12 }}>{error}</div>}
      {loading ? (
        <div className="loading-center" style={{ padding: 32 }}>
          <div className="spinner" /><span style={{ color: 'var(--white-dim)', fontSize: 13 }}>Загрузка...</span>
        </div>
      ) : rows.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '24px 0', color: 'var(--white-dim)', fontSize: 13 }}>Ничего не найдено</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, maxHeight: 380, overflowY: 'auto', marginBottom: 16 }}>
          {rows.map((row, i) => {
            const name = row['ФИО (латиницей, как в загранпаспорте)'] || row['ФИО (латиницей)'] || row['ФИО латиницей'] || Object.values(row)[0] || '—';
            const dob = row['Дата рождения'] || '';
            const passport = row['З/паспорт'] || '';
            const key = JSON.stringify(row);
            const checked = selected.has(key);
            return (
              <label key={i} style={{
                display: 'flex', alignItems: 'center', gap: 12, padding: '10px 14px',
                background: checked ? 'var(--accent-dim)' : 'var(--graphite)',
                border: `1px solid ${checked ? 'var(--accent)' : 'var(--border)'}`,
                borderRadius: 7, cursor: 'pointer', color: 'var(--white)', transition: 'all 0.15s',
              }}>
                <input type="checkbox" checked={checked} onChange={() => toggleRow(key)}
                  style={{ width: 16, height: 16, accentColor: 'var(--accent)', flexShrink: 0, cursor: 'pointer' }} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 13, fontWeight: 500 }}>{name}</div>
                  {(dob || passport) && (
                    <div style={{ fontSize: 11, color: 'var(--white-dim)', fontFamily: 'var(--font-mono)', marginTop: 2 }}>
                      {dob}{dob && passport ? ' · ' : ''}{passport}
                    </div>
                  )}
                </div>
              </label>
            );
          })}
        </div>
      )}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <span style={{ fontSize: 12, color: 'var(--white-dim)' }}>
          {selected.size > 0 ? `Выбрано: ${selected.size}` : 'Выберите туристов'}
        </span>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn-secondary btn-sm" onClick={onClose} disabled={adding}>Отмена</button>
          <button className="btn btn-primary btn-sm" onClick={handleAdd} disabled={selected.size === 0 || adding}>
            {adding ? <><span className="spinner" /> Добавляем...</> : `Добавить (${selected.size})`}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── TouristRow ────────────────────────────────────────────────────────────────

function TouristRow({ tourist, onDelete, subgroups, onAssign }) {
  const name = getTouristName(tourist);
  const isParsed = !!(tourist.raw_json && Object.keys(tourist.raw_json).length > 0);
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '8px 12px', background: 'var(--gray-dark)',
      border: '1px solid var(--border)', borderRadius: 7, gap: 8,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 9, minWidth: 0 }}>
        <div style={{
          width: 26, height: 26, borderRadius: '50%', background: 'var(--gray)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 10, fontWeight: 700, color: 'var(--white-dim)', flexShrink: 0,
        }}>
          {name.charAt(0) || '?'}
        </div>
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 13, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{name}</div>
          {tourist.matched_sheet_row?.['Дата рождения'] && (
            <div style={{ fontSize: 11, color: 'var(--white-dim)', fontFamily: 'var(--font-mono)' }}>
              {tourist.matched_sheet_row['Дата рождения']}
            </div>
          )}
        </div>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
        {subgroups && subgroups.length > 0 && onAssign && (
          <select
            value=""
            onChange={e => e.target.value && onAssign(tourist.id, e.target.value)}
            style={{
              background: 'var(--gray)', border: '1px solid var(--border)', borderRadius: 5,
              color: 'var(--white-dim)', fontSize: 11, padding: '3px 6px', cursor: 'pointer',
            }}
          >
            <option value="">→ в группу</option>
            {subgroups.map(sg => <option key={sg.id} value={sg.id}>{sg.name}</option>)}
          </select>
        )}
        <span style={{
          padding: '2px 8px', borderRadius: 100, fontSize: 11, fontWeight: 500, whiteSpace: 'nowrap',
          background: isParsed ? 'rgba(34,197,94,0.12)' : 'var(--gray)',
          color: isParsed ? 'var(--success)' : 'var(--white-dim)',
        }}>
          {isParsed ? 'Распознан ✓' : 'Ожидает'}
        </span>
        <button className="btn btn-danger btn-sm" onClick={onDelete} title="Удалить">✕</button>
      </div>
    </div>
  );
}

// ── GroupCard ─────────────────────────────────────────────────────────────────

function GroupCard({ group, groupId, allTourists, allUploads, onTouristAdded, onTouristDeleted, onRenamed, onDeleted }) {
  const [expanded, setExpanded] = useState(true);
  const [showAddModal, setShowAddModal] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(group.name);

  // Upload state
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef(null);

  // Parse state
  const [parsing, setParsing] = useState(false);
  const [parseResult, setParseResult] = useState(null);
  const [parseError, setParseError] = useState(null);
  const [notes, setNotes] = useState('');
  const [hotelsReloadKey, setHotelsReloadKey] = useState(0);

  const tourists = allTourists.filter(t => t.subgroup_id === group.id);
  const uploads = allUploads.filter(u => u.subgroup_id === group.id);

  const handleFiles = async (files) => {
    if (!files || files.length === 0) return;
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        await uploadFile(groupId, file, 'document', group.id);
      }
      onTouristAdded(); // reload uploads
    } catch (e) {
      setParseError(e.message);
    } finally {
      setUploading(false);
    }
  };

  const handleParse = async () => {
    setParsing(true);
    setParseResult(null);
    setParseError(null);
    try {
      const result = await parseSubgroup(group.id, notes);
      // Collect detected hotels from all tourists in the result
      const detectedHotels = [];
      const seen = new Set();
      for (const t of result.tourists || []) {
        for (const h of t.result?.hotels_from_vouchers || []) {
          const key = `${h.name}|${h.checkin}|${h.checkout}`;
          if (!seen.has(key)) { seen.add(key); detectedHotels.push(h); }
        }
      }
      setParseResult({ ...result, detectedHotels });
      onTouristAdded(); // reload tourists
      if (detectedHotels.length > 0) setHotelsReloadKey(k => k + 1);
    } catch (e) {
      setParseError(e.message);
    } finally {
      setParsing(false);
    }
  };

  const handleRename = async () => {
    if (!editName.trim() || editName === group.name) { setEditing(false); return; }
    try {
      await updateSubgroup(group.id, editName.trim());
      onRenamed(group.id, editName.trim());
      setEditing(false);
    } catch (e) {
      setParseError(e.message);
    }
  };

  const handleDelete = async () => {
    if (!confirm(`Удалить группу "${group.name}"? Туристы останутся в подаче без группы.`)) return;
    try {
      await deleteSubgroup(group.id);
      onDeleted(group.id);
    } catch (e) {
      setParseError(e.message);
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
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--danger)', fontSize: 14, padding: '2px 6px' }}
              onClick={e => { e.stopPropagation(); handleDelete(); }}
              title="Удалить группу"
            >✕</button>
          </>
        )}
      </div>

      {/* Body */}
      {expanded && (
        <div style={{ padding: '16px 18px', display: 'flex', flexDirection: 'column', gap: 14 }}>
          {parseError && <div className="error-message">{parseError}</div>}

          {/* Tourist list */}
          <div>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
              <span style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500 }}>Туристы</span>
              <button className="btn btn-secondary btn-sm" style={{ fontSize: 11 }} onClick={() => setShowAddModal(true)}>
                + Добавить из таблицы
              </button>
            </div>
            {tourists.length === 0 ? (
              <div style={{ padding: '10px 14px', border: '1px dashed var(--border)', borderRadius: 7, fontSize: 12, color: 'var(--white-dim)', textAlign: 'center' }}>
                Нет туристов — добавьте из таблицы
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                {tourists.map(t => (
                  <TouristRow
                    key={t.id}
                    tourist={t}
                    onDelete={async () => {
                      try { await deleteTourist(t.id); onTouristDeleted(t.id); }
                      catch (e) { setParseError(e.message); }
                    }}
                  />
                ))}
              </div>
            )}
          </div>

          {/* File upload */}
          <div>
            <div style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500, marginBottom: 8 }}>Файлы</div>
            {uploads.length > 0 && (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 5, marginBottom: 8 }}>
                {uploads.map((u, i) => (
                  <span key={u.id || i} style={{
                    display: 'inline-flex', alignItems: 'center', gap: 4, padding: '3px 9px',
                    background: 'var(--gray)', border: '1px solid var(--border)', borderRadius: 5,
                    fontSize: 11, color: 'var(--white-dim)', fontFamily: 'var(--font-mono)',
                  }}>
                    <svg width="10" height="10" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
                      <path d="M9 1H3a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V6L9 1z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
                      <path d="M9 1v5h5" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
                    </svg>
                    {basename(u.file_path) || `файл ${i + 1}`}
                  </span>
                ))}
              </div>
            )}
            <div
              onDragOver={e => { e.preventDefault(); setDragOver(true); }}
              onDragLeave={() => setDragOver(false)}
              onDrop={e => { e.preventDefault(); setDragOver(false); handleFiles(e.dataTransfer.files); }}
              onClick={() => fileInputRef.current?.click()}
              style={{
                border: `1px dashed ${dragOver ? 'var(--accent)' : 'var(--border)'}`, borderRadius: 6,
                padding: '10px 14px', display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer',
                background: dragOver ? 'var(--accent-dim)' : 'transparent', transition: 'all 0.15s',
              }}
            >
              {uploading
                ? <><div className="spinner" style={{ flexShrink: 0 }} /><span style={{ fontSize: 12, color: 'var(--white-dim)' }}>Загрузка...</span></>
                : <><svg width="12" height="12" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, color: 'var(--white-dim)' }}>
                    <path d="M8 11V3M8 3L5 6M8 3l3 3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                    <path d="M2 13h12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
                  </svg>
                  <span style={{ fontSize: 12, color: 'var(--white-dim)' }}>
                    {uploads.length === 0 ? 'Перетащите файлы или нажмите' : '+ Добавить ещё'}
                  </span></>
              }
            </div>
            <input ref={fileInputRef} type="file" multiple accept=".pdf,.jpg,.jpeg,.png"
              style={{ display: 'none' }} onChange={e => { handleFiles(e.target.files); e.target.value = ''; }} />
          </div>

          {/* Notes + parse */}
          <div>
            <textarea
              className="form-input"
              rows={2}
              placeholder="Уточнения для ИИ (необязательно)..."
              value={notes}
              onChange={e => setNotes(e.target.value)}
              style={{ resize: 'vertical', fontFamily: 'inherit', fontSize: 12, marginBottom: 8 }}
            />
            {parseResult && (
              <div className="success-message" style={{ marginBottom: 8 }}>
                <div>Распознано: {parseResult.count ?? parseResult.tourists?.length ?? '?'} туристов</div>
                {parseResult.detectedHotels?.length > 0 && (
                  <div style={{ marginTop: 6, fontSize: 12, opacity: 0.85 }}>
                    Обнаружено отелей из ваучеров: {parseResult.detectedHotels.length} ↓
                  </div>
                )}
              </div>
            )}
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                className="btn btn-primary btn-sm"
                onClick={handleParse}
                disabled={parsing || uploads.length === 0}
                title={uploads.length === 0 ? 'Сначала загрузите файлы' : ''}
              >
                {parsing ? <><span className="spinner" /> Распознавание...</> : 'Распарсить группу'}
              </button>
            </div>
          </div>

          {/* Hotels for this subgroup */}
          <div style={{ borderTop: '1px solid var(--border)', paddingTop: 14 }}>
            <div style={{ fontSize: 12, color: 'var(--white-dim)', fontWeight: 500, marginBottom: 10, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Отели группы
            </div>
            <SubgroupHotelsSection subgroupId={group.id} reloadKey={hotelsReloadKey} />
          </div>
        </div>
      )}

      <Modal open={showAddModal} onClose={() => setShowAddModal(false)} title={`Добавить в "${group.name}"`} width={560}>
        <AddFromSheetModal
          groupId={groupId}
          subgroupId={group.id}
          onAdded={onTouristAdded}
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
  const [uploads, setUploads] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showNewForm, setShowNewForm] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    try {
      const [sgs, ts, ups] = await Promise.all([
        getSubgroups(groupId),
        getTourists(groupId),
        getUploads(groupId),
      ]);
      setSubgroups(Array.isArray(sgs) ? sgs : []);
      setTourists(Array.isArray(ts) ? ts : []);
      setUploads(Array.isArray(ups) ? ups : []);
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
              allUploads={uploads}
              onTouristAdded={load}
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
              <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                {unassigned.map(t => (
                  <TouristRow
                    key={t.id}
                    tourist={t}
                    subgroups={subgroups}
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

function SubgroupHotelsSection({ subgroupId, reloadKey }) {
  const [groupHotels, setGroupHotels] = useState([]);
  const [allHotels, setAllHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState(null);
  const [form, setForm] = useState({ hotel_id: '', check_in: '', check_out: '' });
  const [showAddCard, setShowAddCard] = useState(false);

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

  useEffect(() => { loadAll(); }, [loadAll, reloadKey]);

  const selectedHotel = allHotels.find(h => String(h.id) === String(form.hotel_id));

  const handleAdd = () => {
    if (!form.hotel_id || !form.check_in || !form.check_out) return;
    const hotel = allHotels.find(h => String(h.id) === String(form.hotel_id));
    if (!hotel) return;
    setGroupHotels(prev => [...prev, {
      hotel_id: hotel.id, hotel_name: hotel.name_en, hotel_name_ru: hotel.name_ru,
      city: hotel.city, address: hotel.address, phone: hotel.phone,
      check_in: form.check_in, check_out: form.check_out, sort_order: prev.length,
    }]);
    setForm({ hotel_id: '', check_in: '', check_out: '' });
    setShowAddCard(false);
  };

  const handleRemove = (idx) =>
    setGroupHotels(prev => prev.filter((_, i) => i !== idx).map((h, i) => ({ ...h, sort_order: i })));

  const handleMoveUp = (idx) => {
    if (idx === 0) return;
    setGroupHotels(prev => {
      const arr = [...prev];
      [arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]];
      return arr.map((h, i) => ({ ...h, sort_order: i }));
    });
  };

  const handleMoveDown = (idx) => {
    setGroupHotels(prev => {
      if (idx >= prev.length - 1) return prev;
      const arr = [...prev];
      [arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]];
      return arr.map((h, i) => ({ ...h, sort_order: i }));
    });
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveMsg(null);
    setError(null);
    try {
      await saveSubgroupHotels(subgroupId, groupHotels);
      setSaveMsg('Отели сохранены');
      setTimeout(() => setSaveMsg(null), 3000);
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <div className="loading-center"><div className="spinner" /></div>;

  return (
    <div>
      {error && <div className="error-message">{error}</div>}
      {saveMsg && <div className="success-message">{saveMsg}</div>}

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
                <button style={{ background: 'none', border: 'none', color: idx === 0 ? 'var(--border)' : 'var(--white-dim)', cursor: idx === 0 ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }} onClick={() => handleMoveUp(idx)} disabled={idx === 0}>▲</button>
                <button style={{ background: 'none', border: 'none', color: idx === groupHotels.length - 1 ? 'var(--border)' : 'var(--white-dim)', cursor: idx === groupHotels.length - 1 ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }} onClick={() => handleMoveDown(idx)} disabled={idx === groupHotels.length - 1}>▼</button>
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 500, marginBottom: 3, fontSize: 13 }}>
                  {h.hotel_name}
                  {h.hotel_name_ru && <span style={{ color: 'var(--white-dim)', marginLeft: 8, fontSize: 12 }}>/ {h.hotel_name_ru}</span>}
                  {h.city && <span style={{ color: 'var(--accent)', marginLeft: 8, fontSize: 11, fontWeight: 500 }}>{h.city}</span>}
                </div>
                <div style={{ fontSize: 12, color: 'var(--white-dim)', display: 'flex', gap: 14, flexWrap: 'wrap' }}>
                  <span style={{ fontFamily: 'var(--font-mono)' }}>{h.check_in} → {h.check_out}</span>
                  {h.address && <span>{h.address}</span>}
                  {h.phone && <span>{h.phone}</span>}
                </div>
              </div>
              <button className="btn btn-danger btn-sm" onClick={() => handleRemove(idx)}>Удалить</button>
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
            <button className="btn btn-secondary" onClick={handleAdd} disabled={!form.hotel_id || !form.check_in || !form.check_out}>
              + Добавить
            </button>
          </div>
        </div>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving}>
          {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить отели'}
        </button>
      </div>
    </div>
  );
}

// ── DocumentsTab ──────────────────────────────────────────────────────────────

function SubgroupDocsRow({ subgroup }) {
  const [state, setState] = useState('idle'); // idle | generating | done | error
  const [error, setError] = useState(null);
  const [generatedAt, setGeneratedAt] = useState(null);

  const handleGenerate = async () => {
    setState('generating');
    setError(null);
    try {
      const res = await generateSubgroupDocuments(subgroup.id);
      setGeneratedAt(res.generated_at);
      setState('done');
    } catch (e) {
      setError(e.message);
      setState('error');
    }
  };

  return (
    <div className="card" style={{ marginBottom: 12, padding: '14px 18px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 14, flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--white)' }}>{subgroup.name}</div>
          {state === 'done' && generatedAt && (
            <div style={{ fontSize: 11, color: 'var(--white-dim)', marginTop: 3 }}>
              Сгенерировано: {formatDate(generatedAt)}
            </div>
          )}
          {state === 'error' && error && (
            <div style={{ fontSize: 11, color: 'var(--danger)', marginTop: 3 }}>{error}</div>
          )}
        </div>
        <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleGenerate}
            disabled={state === 'generating'}
          >
            {state === 'generating'
              ? <><span className="spinner" /> Генерация...</>
              : state === 'done' ? 'Перегенерировать' : 'Сгенерировать'}
          </button>
          {state === 'done' && (
            <a href={getSubgroupDownloadUrl(subgroup.id)} className="btn btn-secondary btn-sm" target="_blank" rel="noreferrer" download>
              Скачать ZIP
            </a>
          )}
        </div>
      </div>
    </div>
  );
}

function DocumentsTab({ groupId }) {
  const [subgroups, setSubgroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [finalStep, setFinalStep] = useState('idle');
  const [finalError, setFinalError] = useState(null);

  useEffect(() => {
    (async () => {
      try {
        const sgs = await getSubgroups(groupId);
        setSubgroups(Array.isArray(sgs) ? sgs : []);
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, [groupId]);

  const handleFinalize = async () => {
    setFinalStep('loading');
    setFinalError(null);
    try {
      await finalizeGroup(groupId);
      setFinalStep('done');
    } catch (e) {
      setFinalError(e.message);
      setFinalStep('idle');
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
        Для Инны в ВЦ, заявка ВЦ — формируются после оформления всей подачи.
      </div>

      {finalError && <div className="error-message">{finalError}</div>}

      <div className="card">
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn btn-primary" onClick={handleFinalize} disabled={finalStep === 'loading'}>
            {finalStep === 'loading' ? <><span className="spinner" /> Генерация...</> : 'Сформировать финальные документы'}
          </button>
          {finalStep === 'done' && (
            <a href={getFinalDownloadUrl(groupId)} className="btn btn-secondary" target="_blank" rel="noreferrer" download>
              Скачать final.zip
            </a>
          )}
          {finalStep === 'done' && <span style={{ fontSize: 13, color: 'var(--success)' }}>✓ Готово</span>}
        </div>
      </div>
    </div>
  );
}

// ── Main Component ────────────────────────────────────────────────────────────

const TABS = [
  { id: 'groups', label: 'Группы' },
  { id: 'documents', label: 'Документы' },
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
        setGroup(data);
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

      {activeTab === 'groups' && <GroupsTab groupId={id} />}
      {activeTab === 'documents' && <DocumentsTab groupId={id} />}
    </div>
  );
}
