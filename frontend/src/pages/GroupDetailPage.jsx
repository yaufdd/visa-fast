import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getGroup,
  getHotels, getSubgroupHotels, saveSubgroupHotels,
  finalizeGroup, getFinalDownloadUrl, getFinalStatus,
  generateSubgroupDocuments, getSubgroupDownloadUrl,
  getTourists, addTouristFromSheet, deleteTourist,
  uploadFile, getUploads,
  getSheetRows,
  getSubgroups, createSubgroup, updateSubgroup, deleteSubgroup,
  assignTouristSubgroup, parseSubgroup, parseGroup,
  updateGroupStatus, deleteGroup,
} from '../api/client';
import StatusSection from '../components/StatusSection';

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
    let added = 0;
    let failErr = null;
    try {
      for (const k of selected) {
        const row = JSON.parse(k);
        try {
          await addTouristFromSheet(groupId, row, subgroupId);
          added += 1;
        } catch (e) {
          failErr = e;
          break;
        }
      }
    } finally {
      // Always refresh so partial progress is visible even on mid-loop error.
      if (added > 0) onAdded();
      if (failErr) {
        setError(`${failErr.message} (добавлено: ${added} из ${selected.size})`);
        setAdding(false);
      } else {
        onClose();
      }
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
            transition: 'color 0.15s, background 0.15s',
          }}
          onMouseEnter={e => { e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
          onMouseLeave={e => { e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
        >✕</button>
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
  const [hotelsExpanded, setHotelsExpanded] = useState(false);

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
              placeholder="Уточнение (необязательно)..."
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
                style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
              >
                {parsing
                  ? <><span className="spinner" /> Распознавание...</>
                  : <>
                  Распознать
                      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
                        <path d="M8 1.5l1.1 3.4 3.4 1.1-3.4 1.1L8 10.5 6.9 7.1 3.5 6l3.4-1.1L8 1.5z" fill="currentColor"/>
                        <path d="M12.5 10l0.5 1.5L14.5 12l-1.5 0.5L12.5 14l-0.5-1.5L10.5 12l1.5-0.5L12.5 10z" fill="currentColor"/>
                        <path d="M3 11.5l0.3 1 1 0.3-1 0.3L3 14.1l-0.3-1-1-0.3 1-0.3L3 11.5z" fill="currentColor"/>
                      </svg>
                      
                    </>
                }
              </button>
            </div>
          </div>

          {/* Hotels for this subgroup — collapsible, collapsed by default */}
          <div style={{ borderTop: '1px solid var(--border)', paddingTop: 14 }}>
            <div
              onClick={() => setHotelsExpanded(e => !e)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                cursor: 'pointer',
                userSelect: 'none',
                marginBottom: hotelsExpanded ? 10 : 0,
              }}
            >
              <span style={{
                fontSize: 10,
                color: 'var(--white-dim)',
                transition: 'transform 0.2s',
                display: 'inline-block',
                transform: hotelsExpanded ? 'rotate(90deg)' : 'none',
              }}>▶</span>
              <span style={{
                fontSize: 12,
                color: 'var(--white-dim)',
                fontWeight: 500,
                textTransform: 'uppercase',
                letterSpacing: '0.05em',
              }}>
                Отели группы
              </span>
            </div>
            {hotelsExpanded && (
              <SubgroupHotelsSection subgroupId={group.id} reloadKey={hotelsReloadKey} />
            )}
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
  const [dirty, setDirty] = useState(false);

  const loadAll = useCallback(async () => {
    try {
      const [hotels, current] = await Promise.all([getHotels(), getSubgroupHotels(subgroupId)]);
      setAllHotels(Array.isArray(hotels) ? hotels : []);
      setGroupHotels(Array.isArray(current) ? current : []);
      setDirty(false);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [subgroupId]);

  useEffect(() => { loadAll(); }, [loadAll, reloadKey]);

  // Warn on accidental tab close while there are unsaved local changes.
  useEffect(() => {
    if (!dirty) return;
    const handler = (e) => { e.preventDefault(); e.returnValue = ''; };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);

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
    setDirty(true);
  };

  const handleRemove = (idx) => {
    setGroupHotels(prev => prev.filter((_, i) => i !== idx).map((h, i) => ({ ...h, sort_order: i })));
    setDirty(true);
  };

  const handleMoveUp = (idx) => {
    if (idx === 0) return;
    setGroupHotels(prev => {
      const arr = [...prev];
      [arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]];
      return arr.map((h, i) => ({ ...h, sort_order: i }));
    });
    setDirty(true);
  };

  const handleMoveDown = (idx) => {
    setGroupHotels(prev => {
      if (idx >= prev.length - 1) return prev;
      const arr = [...prev];
      [arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]];
      return arr.map((h, i) => ({ ...h, sort_order: i }));
    });
    setDirty(true);
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveMsg(null);
    setError(null);
    try {
      await saveSubgroupHotels(subgroupId, groupHotels);
      // Re-sync from server so the UI reflects any normalization the backend did.
      await loadAll();
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
              <button
                type="button"
                onClick={() => handleRemove(idx)}
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
                  transition: 'color 0.15s, background 0.15s',
                }}
                onMouseEnter={e => { e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                onMouseLeave={e => { e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
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
            <button className="btn btn-secondary" onClick={handleAdd} disabled={!form.hotel_id || !form.check_in || !form.check_out}>
              + Добавить
            </button>
          </div>
        </div>
      )}

      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 10 }}>
        {dirty && !saving && (
          <span style={{ fontSize: 11, color: 'var(--warning, #e0a82e)' }}>● Есть несохранённые изменения</span>
        )}
        <button className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving || !dirty}>
          {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить отели'}
        </button>
      </div>
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
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 14, flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0, flex: 1 }}>
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
        <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
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
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 14, flexWrap: 'wrap' }}>
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: 10, fontSize: 13, color: 'var(--white)', fontWeight: 500 }}>
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
          <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
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
