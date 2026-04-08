import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getGroup,
  getHotels, getGroupHotels, saveGroupHotels,
  generateDocuments, getDownloadUrl, finalizeGroup, getFinalDownloadUrl,
  getTourists, addTouristFromSheet, deleteTourist,
  uploadFile, getUploads, parseGroup,
  getSheetRows,
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
  const row = tourist.matched_sheet_row || tourist.sheet_data || {};
  return (
    row['ФИО (латиницей, как в загранпаспорте)'] ||
    row['ФИО (латиницей)'] ||
    row['ФИО латиницей'] ||
    Object.values(row)[0] ||
    '—'
  );
}

function getTouristStatus(tourist, uploads) {
  const hasRawJson = tourist.raw_json && Object.keys(tourist.raw_json).length > 0;
  if (hasRawJson) return 'parsed';
  if (uploads && uploads.length > 0) return 'ready';
  return 'waiting';
}

const STATUS_CONFIG = {
  waiting: { label: 'Ожидает файлов', color: 'var(--white-dim)', bg: 'var(--gray)' },
  ready: { label: 'Готов к распознаванию', color: 'var(--warning)', bg: 'rgba(245,158,11,0.12)' },
  parsed: { label: 'Распознан', color: 'var(--success)', bg: 'rgba(34,197,94,0.12)' },
};

// ── AddFromSheetModal ─────────────────────────────────────────────────────────

function AddFromSheetModal({ groupId, onAdded, onClose }) {
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [selected, setSelected] = useState(new Set()); // Set of JSON-stringified rows
  const [adding, setAdding] = useState(false);
  const debounceRef = useRef(null);

  const loadRows = useCallback((q) => {
    setLoading(true);
    setError(null);
    getSheetRows(q).then(data => {
      const normalized = Array.isArray(data)
        ? data.map(item => (item.row ? item.row : item))
        : [];
      setRows(normalized);
    }).catch(e => {
      setError(e.message);
    }).finally(() => setLoading(false));
  }, []);

  useEffect(() => { loadRows(''); }, [loadRows]);

  useEffect(() => {
    if (!query.trim()) { loadRows(''); return; }
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => loadRows(query), 350);
    return () => clearTimeout(debounceRef.current);
  }, [query, loadRows]);

  const toggleRow = (key) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const handleAddSelected = async () => {
    if (selected.size === 0) return;
    setAdding(true);
    setError(null);
    try {
      const selectedRows = [...selected].map(k => JSON.parse(k));
      for (const row of selectedRows) {
        await addTouristFromSheet(groupId, row);
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
          <div className="spinner" />
          <span style={{ color: 'var(--white-dim)', fontSize: 13 }}>Загрузка...</span>
        </div>
      ) : rows.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '24px 0', color: 'var(--white-dim)', fontSize: 13 }}>
          Ничего не найдено
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6, maxHeight: 380, overflowY: 'auto', marginBottom: 16 }}>
          {rows.map((row, i) => {
            const name =
              row['ФИО (латиницей, как в загранпаспорте)'] ||
              row['ФИО (латиницей)'] ||
              row['ФИО латиницей'] ||
              Object.values(row)[0] ||
              '—';
            const dob = row['Дата рождения'] || '';
            const passport = row['З/паспорт'] || '';
            const key = JSON.stringify(row);
            const isChecked = selected.has(key);
            return (
              <label
                key={i}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 14px',
                  background: isChecked ? 'var(--accent-dim)' : 'var(--graphite)',
                  border: `1px solid ${isChecked ? 'var(--accent)' : 'var(--border)'}`,
                  borderRadius: 7,
                  cursor: 'pointer',
                  color: 'var(--white)',
                  transition: 'all 0.15s',
                }}
              >
                <input
                  type="checkbox"
                  checked={isChecked}
                  onChange={() => toggleRow(key)}
                  style={{ width: 16, height: 16, accentColor: 'var(--accent)', flexShrink: 0, cursor: 'pointer' }}
                />
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
          <button className="btn btn-secondary btn-sm" onClick={onClose} disabled={adding}>
            Отмена
          </button>
          <button
            className="btn btn-primary btn-sm"
            onClick={handleAddSelected}
            disabled={selected.size === 0 || adding}
          >
            {adding ? <><span className="spinner" /> Добавляем...</> : `Добавить (${selected.size})`}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── TouristCard ───────────────────────────────────────────────────────────────

function TouristCard({ tourist, onDeleted }) {
  const name = getTouristName(tourist);
  const isParsed = !!(tourist.raw_json && Object.keys(tourist.raw_json).length > 0);
  const statusCfg = STATUS_CONFIG[isParsed ? 'parsed' : 'waiting'];

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '12px 16px',
      background: 'var(--graphite)',
      border: '1px solid var(--border)',
      borderRadius: 8,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <div style={{
          width: 30,
          height: 30,
          borderRadius: '50%',
          background: 'var(--gray)',
          border: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 11,
          fontWeight: 700,
          color: 'var(--white-dim)',
          flexShrink: 0,
        }}>
          {name.charAt(0) || '?'}
        </div>
        <div>
          <div style={{ fontWeight: 500, fontSize: 13 }}>{name}</div>
          {tourist.matched_sheet_row?.['Дата рождения'] && (
            <div style={{ fontSize: 11, color: 'var(--white-dim)', fontFamily: 'var(--font-mono)', marginTop: 1 }}>
              {tourist.matched_sheet_row['Дата рождения']}
              {tourist.matched_sheet_row['З/паспорт'] && ` · ${tourist.matched_sheet_row['З/паспорт']}`}
            </div>
          )}
        </div>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{
          padding: '2px 9px',
          borderRadius: 100,
          background: statusCfg.bg,
          color: statusCfg.color,
          fontSize: 11,
          fontWeight: 500,
        }}>
          {isParsed ? 'Распознан \u2713' : 'Ожидает'}
        </span>
        <button
          className="btn btn-danger btn-sm"
          onClick={onDeleted}
          title="Удалить туриста"
        >
          \u2715
        </button>
      </div>
    </div>
  );
}

// ── TouristsSection ───────────────────────────────────────────────────────────

function TouristsSection({ groupId }) {
  const [tourists, setTourists] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [showAddModal, setShowAddModal] = useState(false);

  // Shared upload state
  const [uploads, setUploads] = useState([]);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef(null);

  // Notes for AI
  const [notes, setNotes] = useState('');

  // Parse-all state
  const [parsing, setParsing] = useState(false);
  const [parseResult, setParseResult] = useState(null);

  const loadTourists = useCallback(async () => {
    try {
      const data = await getTourists(groupId);
      setTourists(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [groupId]);

  const loadUploads = useCallback(async () => {
    try {
      const data = await getUploads(groupId);
      setUploads(Array.isArray(data) ? data : []);
    } catch {
      // silently ignore
    }
  }, [groupId]);

  useEffect(() => { loadTourists(); loadUploads(); }, [loadTourists, loadUploads]);

  const handleDelete = async (touristId) => {
    try {
      await deleteTourist(touristId);
      setTourists(prev => prev.filter(t => t.id !== touristId));
    } catch (e) {
      setError(e.message);
    }
  };

  const handleFiles = async (files) => {
    if (!files || files.length === 0) return;
    setUploading(true);
    setError(null);
    try {
      for (const file of Array.from(files)) {
        await uploadFile(groupId, file, 'document');
      }
      await loadUploads();
    } catch (e) {
      setError(e.message);
    } finally {
      setUploading(false);
    }
  };

  const handleDrop = (e) => {
    e.preventDefault();
    setDragOver(false);
    handleFiles(e.dataTransfer.files);
  };

  const handleParseAll = async () => {
    setParsing(true);
    setParseResult(null);
    setError(null);
    try {
      const result = await parseGroup(groupId, notes);
      setParseResult(result);
      await loadTourists();
    } catch (e) {
      setError(e.message);
    } finally {
      setParsing(false);
    }
  };

  return (
    <div>
      <div className="section-header">
        <div className="section-title">Туристы</div>
        <button className="btn btn-primary btn-sm" onClick={() => setShowAddModal(true)}>
          + Добавить из таблицы
        </button>
      </div>

      {error && <div className="error-message" style={{ marginBottom: 14 }}>{error}</div>}

      {/* Tourist list */}
      {loading ? (
        <div className="loading-center"><div className="spinner spinner-lg" /></div>
      ) : tourists.length === 0 ? (
        <div className="card" style={{ marginBottom: 20 }}>
          <div className="empty-state">
            <div className="empty-state-icon" style={{ fontSize: 32, opacity: 0.3, marginBottom: 10 }}>
              <svg width="40" height="40" viewBox="0 0 24 24" fill="none">
                <circle cx="12" cy="8" r="4" stroke="currentColor" strokeWidth="1.5"/>
                <path d="M4 20c0-4 3.58-7 8-7s8 3 8 7" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
              </svg>
            </div>
            <div className="empty-state-title">Нет туристов</div>
            <div className="empty-state-text">Добавьте туристов из таблицы Google Sheets</div>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 20 }}>
          {tourists.map(t => (
            <TouristCard
              key={t.id}
              tourist={t}
              onDeleted={() => handleDelete(t.id)}
            />
          ))}
        </div>
      )}

      {/* Shared file upload */}
      <div className="card" style={{ marginBottom: 16 }}>
        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 12, color: 'var(--white-dim)' }}>
          Документы (для всех туристов)
        </div>

        {/* Uploaded file chips */}
        {uploads.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 10 }}>
            {uploads.map((u, i) => (
              <span key={u.id || i} style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 5,
                padding: '3px 10px',
                background: 'var(--gray)',
                border: '1px solid var(--border)',
                borderRadius: 5,
                fontSize: 11,
                color: 'var(--white-dim)',
                fontFamily: 'var(--font-mono)',
              }}>
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
                  <path d="M9 1H3a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V6L9 1z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
                  <path d="M9 1v5h5" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/>
                </svg>
                {basename(u.file_path) || u.original_name || `файл ${i + 1}`}
              </span>
            ))}
          </div>
        )}

        {/* Drag-drop zone */}
        <div
          onDragOver={e => { e.preventDefault(); setDragOver(true); }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
          style={{
            border: `1px dashed ${dragOver ? 'var(--accent)' : 'var(--border)'}`,
            borderRadius: 6,
            padding: '12px 16px',
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            cursor: 'pointer',
            background: dragOver ? 'var(--accent-dim)' : 'transparent',
            transition: 'all 0.15s',
          }}
        >
          {uploading ? (
            <>
              <div className="spinner" style={{ flexShrink: 0 }} />
              <span style={{ fontSize: 12, color: 'var(--white-dim)' }}>Загрузка...</span>
            </>
          ) : (
            <>
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0, color: 'var(--white-dim)' }}>
                <path d="M8 11V3M8 3L5 6M8 3l3 3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                <path d="M2 13h12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
              </svg>
              <span style={{ fontSize: 12, color: 'var(--white-dim)' }}>
                {uploads.length === 0 ? 'Перетащите файлы или нажмите для выбора' : '+ Добавить ещё'}
              </span>
            </>
          )}
        </div>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept=".pdf,.jpg,.jpeg,.png"
          style={{ display: 'none' }}
          onChange={e => { handleFiles(e.target.files); e.target.value = ''; }}
        />
      </div>

      {/* Notes + parse */}
      <div className="card">
        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 10, color: 'var(--white-dim)' }}>
          Уточнения для ИИ (необязательно)
        </div>
        <textarea
          className="form-input"
          rows={3}
          placeholder="Например: паспорт и билет принадлежат разным людям, у первого туриста два билета..."
          value={notes}
          onChange={e => setNotes(e.target.value)}
          style={{ resize: 'vertical', fontFamily: 'inherit', fontSize: 13 }}
        />

        {parseResult && (
          <div className="success-message" style={{ marginTop: 10 }}>
            Распознано: {parseResult.count ?? parseResult.tourists?.length ?? '?'} туристов
          </div>
        )}

        <div style={{ marginTop: 12, display: 'flex', justifyContent: 'flex-end' }}>
          <button
            className="btn btn-primary"
            onClick={handleParseAll}
            disabled={parsing || uploads.length === 0}
            title={uploads.length === 0 ? 'Сначала загрузите файлы' : ''}
          >
            {parsing ? <><span className="spinner" /> Распознавание...</> : 'Распарсить всех'}
          </button>
        </div>
      </div>

      <Modal
        open={showAddModal}
        onClose={() => setShowAddModal(false)}
        title="Добавить туристов из таблицы"
        width={560}
      >
        <AddFromSheetModal
          groupId={groupId}
          onAdded={loadTourists}
          onClose={() => setShowAddModal(false)}
        />
      </Modal>
    </div>
  );
}

// ── HotelsSection ─────────────────────────────────────────────────────────────

function HotelsSection({ groupId }) {
  const [groupHotels, setGroupHotels] = useState([]);
  const [allHotels, setAllHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState(null);

  const [form, setForm] = useState({ hotel_id: '', check_in: '', check_out: '' });

  const loadAll = useCallback(async () => {
    try {
      const [hotels, current] = await Promise.all([getHotels(), getGroupHotels(groupId)]);
      setAllHotels(Array.isArray(hotels) ? hotels : []);
      setGroupHotels(Array.isArray(current) ? current : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [groupId]);

  useEffect(() => { loadAll(); }, [loadAll]);

  const selectedHotel = allHotels.find(h => String(h.id) === String(form.hotel_id));

  const handleAdd = () => {
    if (!form.hotel_id || !form.check_in || !form.check_out) return;
    const hotel = allHotels.find(h => String(h.id) === String(form.hotel_id));
    if (!hotel) return;
    setGroupHotels(prev => [
      ...prev,
      {
        hotel_id: hotel.id,
        hotel_name: hotel.name_en,
        hotel_name_ru: hotel.name_ru,
        city: hotel.city,
        address: hotel.address,
        phone: hotel.phone,
        check_in: form.check_in,
        check_out: form.check_out,
        sort_order: prev.length,
      },
    ]);
    setForm({ hotel_id: '', check_in: '', check_out: '' });
  };

  const handleRemove = (idx) => {
    setGroupHotels(prev => prev.filter((_, i) => i !== idx).map((h, i) => ({ ...h, sort_order: i })));
  };

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
      await saveGroupHotels(groupId, groupHotels);
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
      {/* Section divider */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 16,
        margin: '40px 0 24px',
      }}>
        <div style={{ height: 1, flex: 1, background: 'var(--border)' }} />
        <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--white-dim)', letterSpacing: '0.1em', textTransform: 'uppercase' }}>
          Отели группы
        </span>
        <div style={{ height: 1, flex: 1, background: 'var(--border)' }} />
      </div>

      {error && <div className="error-message">{error}</div>}
      {saveMsg && <div className="success-message">{saveMsg}</div>}

      {/* Add hotel form */}
      <div className="card" style={{ marginBottom: 20 }}>
        <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 14, color: 'var(--white-dim)' }}>
          Добавить отель
        </div>
        <div className="form-group" style={{ marginBottom: 12 }}>
          <label className="form-label">Отель</label>
          <select
            className="form-input"
            value={form.hotel_id}
            onChange={e => setForm(f => ({ ...f, hotel_id: e.target.value }))}
          >
            <option value="">— выберите отель —</option>
            {allHotels.map(h => (
              <option key={h.id} value={h.id}>{h.name_en} ({h.city})</option>
            ))}
          </select>
        </div>

        {selectedHotel && (
          <div style={{
            padding: '9px 12px',
            background: 'var(--graphite)',
            borderRadius: 6,
            marginBottom: 12,
            fontSize: 12,
            color: 'var(--white-dim)',
            display: 'flex',
            gap: 20,
            flexWrap: 'wrap',
          }}>
            <span>{selectedHotel.address || '—'}</span>
            <span>{selectedHotel.phone || '—'}</span>
          </div>
        )}

        <div className="grid-2">
          <div className="form-group" style={{ marginBottom: 0 }}>
            <label className="form-label">Check-in</label>
            <input
              className="form-input"
              type="date"
              value={form.check_in}
              onChange={e => setForm(f => ({ ...f, check_in: e.target.value }))}
            />
          </div>
          <div className="form-group" style={{ marginBottom: 0 }}>
            <label className="form-label">Check-out</label>
            <input
              className="form-input"
              type="date"
              value={form.check_out}
              onChange={e => setForm(f => ({ ...f, check_out: e.target.value }))}
            />
          </div>
        </div>

        <div style={{ marginTop: 14, display: 'flex', justifyContent: 'flex-end' }}>
          <button
            className="btn btn-secondary"
            onClick={handleAdd}
            disabled={!form.hotel_id || !form.check_in || !form.check_out}
          >
            + Добавить
          </button>
        </div>
      </div>

      {/* Hotels list */}
      {groupHotels.length === 0 ? (
        <div style={{ textAlign: 'center', padding: '24px 0', color: 'var(--white-dim)', fontSize: 13 }}>
          Нет отелей — добавьте через форму выше
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {groupHotels.map((h, idx) => (
            <div key={idx} style={{
              display: 'flex',
              alignItems: 'center',
              gap: 12,
              padding: '12px 16px',
              background: 'var(--gray-dark)',
              border: '1px solid var(--border)',
              borderRadius: 8,
            }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 24 }}>
                <button
                  style={{ background: 'none', border: 'none', color: idx === 0 ? 'var(--border)' : 'var(--white-dim)', cursor: idx === 0 ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }}
                  onClick={() => handleMoveUp(idx)}
                  disabled={idx === 0}
                >▲</button>
                <button
                  style={{ background: 'none', border: 'none', color: idx === groupHotels.length - 1 ? 'var(--border)' : 'var(--white-dim)', cursor: idx === groupHotels.length - 1 ? 'default' : 'pointer', fontSize: 13, lineHeight: 1, padding: 0 }}
                  onClick={() => handleMoveDown(idx)}
                  disabled={idx === groupHotels.length - 1}
                >▼</button>
              </div>

              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 500, marginBottom: 3, fontSize: 13 }}>
                  {h.hotel_name}
                  {h.hotel_name_ru && (
                    <span style={{ color: 'var(--white-dim)', marginLeft: 8, fontSize: 12 }}>/ {h.hotel_name_ru}</span>
                  )}
                  {h.city && (
                    <span style={{ color: 'var(--accent)', marginLeft: 8, fontSize: 11, fontWeight: 500 }}>{h.city}</span>
                  )}
                </div>
                <div style={{ fontSize: 12, color: 'var(--white-dim)', display: 'flex', gap: 14, flexWrap: 'wrap' }}>
                  <span style={{ fontFamily: 'var(--font-mono)' }}>{h.check_in} \u2192 {h.check_out}</span>
                  {h.address && <span>{h.address}</span>}
                  {h.phone && <span>{h.phone}</span>}
                </div>
              </div>

              <button className="btn btn-danger btn-sm" onClick={() => handleRemove(idx)}>
                Удалить
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Save button */}
      <div style={{ marginTop: 20, display: 'flex', justifyContent: 'flex-end' }}>
        <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
          {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить отели'}
        </button>
      </div>
    </div>
  );
}

// ── DocumentsTab ──────────────────────────────────────────────────────────────

const GENERATE_STEPS = ['idle', 'parsing', 'formatting', 'generating', 'done'];
const STEP_LABELS = {
  idle: 'Готов к генерации',
  parsing: 'Разбор данных...',
  formatting: 'Форматирование...',
  generating: 'Генерация документов...',
  done: 'Готово!',
};
const STEP_SHORT = { parsing: 'Разбор', formatting: 'Формат', generating: 'Генерация', done: 'Готово' };

function DocumentsTab({ groupId, group }) {
  const [step, setStep] = useState('idle');
  const [error, setError] = useState(null);
  const [genResult, setGenResult] = useState(null);
  const [finalStep, setFinalStep] = useState('idle'); // idle | loading | done
  const [finalError, setFinalError] = useState(null);

  const handleGenerate = async () => {
    setStep('parsing');
    setError(null);
    setGenResult(null);
    try {
      setStep('formatting');
      await new Promise(r => setTimeout(r, 400));
      setStep('generating');
      const result = await generateDocuments(groupId);
      setStep('done');
      setGenResult(result);
    } catch (e) {
      setError(e.message);
      setStep('idle');
    }
  };

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

  const stepIdx = GENERATE_STEPS.indexOf(step);

  return (
    <div>
      {/* ── Per-tourist documents ── */}
      <div className="section-header">
        <div className="section-title">Документы туристов</div>
      </div>
      <div style={{ fontSize: 12, color: 'var(--white-dim)', marginBottom: 16 }}>
        Программа, доверенность, анкета — по одному пакету на каждого туриста.
      </div>

      {error && <div className="error-message">{error}</div>}

      <div className="card" style={{ marginBottom: 32 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 28 }}>
          {GENERATE_STEPS.filter(s => s !== 'idle').map((s, i) => {
            const done = stepIdx > i + 1;
            const active = stepIdx === i + 1;
            return (
              <div key={s} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, flex: 1 }}>
                <div style={{
                  width: 28,
                  height: 28,
                  borderRadius: '50%',
                  border: `2px solid ${done ? 'var(--success)' : active ? 'var(--accent)' : 'var(--border)'}`,
                  background: done ? 'var(--success)' : active ? 'var(--accent)' : 'transparent',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 11,
                  fontWeight: 700,
                  color: done || active ? '#fff' : 'var(--white-dim)',
                  transition: 'all 0.3s',
                }}>
                  {done ? '\u2713' : i + 1}
                </div>
                <span style={{ fontSize: 11, color: active ? 'var(--white)' : 'var(--white-dim)', textAlign: 'center' }}>
                  {STEP_SHORT[s]}
                </span>
              </div>
            );
          })}
        </div>

        <div style={{ textAlign: 'center', padding: '16px 0 24px' }}>
          {step !== 'idle' && step !== 'done' && (
            <div className="spinner spinner-lg" style={{ margin: '0 auto 14px' }} />
          )}
          {step === 'done' && (
            <div style={{ fontSize: 32, marginBottom: 12 }}>&#10003;</div>
          )}
          <div style={{ fontSize: 15, fontWeight: 500, marginBottom: 6 }}>{STEP_LABELS[step]}</div>
          {genResult?.generated_at && (
            <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>
              Сгенерировано: {formatDate(genResult.generated_at)}
            </div>
          )}
        </div>

        <div style={{ display: 'flex', gap: 12, justifyContent: 'center', flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary"
            onClick={handleGenerate}
            disabled={step !== 'idle' && step !== 'done'}
          >
            {step !== 'idle' && step !== 'done'
              ? <><span className="spinner" /> Генерация...</>
              : 'Сгенерировать'}
          </button>

          {step === 'done' && (
            <a
              href={getDownloadUrl(groupId)}
              className="btn btn-secondary"
              target="_blank"
              rel="noreferrer"
              download
            >
              Скачать ZIP
            </a>
          )}
        </div>
      </div>

      {/* ── Final group documents ── */}
      <div className="section-header">
        <div className="section-title">Финальные документы</div>
      </div>
      <div style={{ fontSize: 12, color: 'var(--white-dim)', marginBottom: 16 }}>
        Для Инны в ВЦ, заявка ВЦ — формируются после оформления всей группы.
      </div>

      {finalError && <div className="error-message">{finalError}</div>}

      <div className="card">
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary"
            onClick={handleFinalize}
            disabled={finalStep === 'loading'}
          >
            {finalStep === 'loading'
              ? <><span className="spinner" /> Генерация...</>
              : 'Сформировать финальные документы'}
          </button>

          {finalStep === 'done' && (
            <a
              href={getFinalDownloadUrl(groupId)}
              className="btn btn-secondary"
              target="_blank"
              rel="noreferrer"
              download
            >
              Скачать final.zip
            </a>
          )}

          {finalStep === 'done' && (
            <span style={{ fontSize: 13, color: 'var(--success)' }}>&#10003; Готово</span>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Main Component ─────────────────────────────────────────────────────────────

const TABS = [
  { id: 'tourists', label: 'Туристы' },
  { id: 'documents', label: 'Документы' },
];

export default function GroupDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [group, setGroup] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState('tourists');

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
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 13,
                cursor: 'pointer',
                padding: 0,
                display: 'flex',
                alignItems: 'center',
                gap: 4,
              }}
            >
              \u2190 Группы
            </button>
            <span style={{ color: 'var(--border)' }}>/</span>
            <span style={{ color: 'var(--white-dim)', fontSize: 13 }}>{group?.name}</span>
          </div>
          <div className="page-title">{group?.name}</div>
          <div className="page-subtitle" style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 6 }}>
            <StatusBadge status={group?.status || 'draft'} />
            <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
              Создана: {formatDate(group?.created_at)}
            </span>
          </div>
        </div>
      </div>

      <div className="tabs">
        {TABS.map(t => (
          <button
            key={t.id}
            className={`tab-btn${activeTab === t.id ? ' active' : ''}`}
            onClick={() => setActiveTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {activeTab === 'tourists' && (
        <div>
          <TouristsSection groupId={id} />
          <HotelsSection groupId={id} />
        </div>
      )}
      {activeTab === 'documents' && <DocumentsTab groupId={id} group={group} />}
    </div>
  );
}
