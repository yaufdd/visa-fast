import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  uploadTouristFile,
  parseTouristUpload,
  getTouristUploads,
  deleteTouristUpload,
  updateFlightData,
  applyFlightDataToSubgroup,
} from '../api/client';
import FlightDataForm from './FlightDataForm';
import ConfirmModal from './ConfirmModal';

// ── Shared helpers ───────────────────────────────────────────────────────────

const FILE_TYPE_LABEL = { ticket: 'Билет', voucher: 'Ваучер' };

function snapshotOf(t) {
  const raw = t?.submission_snapshot;
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

function getTouristName(t) {
  const s = snapshotOf(t);
  return s.name_lat || s.name_cyr || '—';
}

function safeParse(raw) {
  if (!raw) return null;
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return null; }
}

function uploadDisplayName(u) {
  const raw = u.file_path || '';
  const base = raw.split(/[\\/]/).pop() || '';
  const prefix = `${u.file_type}_`;
  return base.startsWith(prefix) ? base.slice(prefix.length) : base;
}

function formatUploadDate(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit' });
}

function formatLeg(leg) {
  if (!leg) return null;
  const parts = [];
  if (leg.flight_number) parts.push(leg.flight_number);
  if (leg.airport) parts.push(leg.airport);
  const when = [leg.date, leg.time].filter(Boolean).join(' ');
  if (when) parts.push(when);
  return parts.length ? parts.join(' · ') : null;
}

function isLegEmpty(leg) {
  return !leg || (!leg.flight_number && !leg.date && !leg.time && !leg.airport);
}

// ── Plain trash icon (gray) ──────────────────────────────────────────────────

function TrashIcon({ size = 13 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M3 4h10M6.5 4V2.5a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1V4M4 4l.5 8.5a1.5 1.5 0 0 0 1.5 1.4h4a1.5 1.5 0 0 0 1.5-1.4L12 4M6.5 7v4M9.5 7v4"
        stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}

// ── Hook: all uploads/parsing/delete state for one tourist ───────────────────

function useTouristUploads(touristId, onChanged) {
  const [uploads, setUploads] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [uploadingType, setUploadingType] = useState(null);
  const [uploadProgress, setUploadProgress] = useState(null);
  const [bulkParsing, setBulkParsing] = useState(false);
  const [bulkProgress, setBulkProgress] = useState(null);
  const [parsingId, setParsingId] = useState(null);
  const [parseErrorById, setParseErrorById] = useState({});
  const [deletingId, setDeletingId] = useState(null);
  const [confirmDeleteTarget, setConfirmDeleteTarget] = useState(null);
  const [confirmDeleteError, setConfirmDeleteError] = useState(null);
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

  const handleUpload = async (files, fileType) => {
    const list = files ? Array.from(files) : [];
    if (list.length === 0) return;
    setUploadingType(fileType);
    setError(null);
    setUploadProgress({ done: 0, total: list.length });
    const errors = [];
    for (let i = 0; i < list.length; i++) {
      const file = list[i];
      try {
        const res = await uploadTouristFile(touristId, file, fileType);
        if (res?.redact_error) errors.push(`${file.name}: ${res.redact_error}`);
      } catch (e) {
        errors.push(`${file.name}: ${e.message}`);
      }
      setUploadProgress({ done: i + 1, total: list.length });
    }
    if (errors.length) setError(errors.join('\n'));
    await load();
    onChanged?.();
    setUploadingType(null);
    setUploadProgress(null);
  };

  const handleParseAll = async () => {
    const targets = uploads.filter(u => !u.parsed_at);
    if (targets.length === 0) return;
    setBulkParsing(true);
    setBulkProgress({ done: 0, total: targets.length });
    setParseErrorById(m => {
      const next = { ...m };
      targets.forEach(u => { next[u.id] = null; });
      return next;
    });
    for (let i = 0; i < targets.length; i++) {
      const u = targets[i];
      setParsingId(u.id);
      try {
        const res = await parseTouristUpload(touristId, u.id);
        if (res?.parse_error) setParseErrorById(m => ({ ...m, [u.id]: res.parse_error }));
        else if (res?.redact_error) setParseErrorById(m => ({ ...m, [u.id]: res.redact_error }));
      } catch (e) {
        setParseErrorById(m => ({ ...m, [u.id]: e.message }));
      }
      setBulkProgress({ done: i + 1, total: targets.length });
    }
    setParsingId(null);
    await load();
    onChanged?.();
    setBulkParsing(false);
    setBulkProgress(null);
  };

  const requestDelete = (u) => {
    setConfirmDeleteError(null);
    setConfirmDeleteTarget(u);
  };

  const confirmDelete = async () => {
    const u = confirmDeleteTarget;
    if (!u) return;
    setDeletingId(u.id);
    setConfirmDeleteError(null);
    try {
      await deleteTouristUpload(touristId, u.id);
      setConfirmDeleteTarget(null);
      await load();
      onChanged?.();
    } catch (e) {
      setConfirmDeleteError(e.message);
    } finally {
      setDeletingId(null);
    }
  };

  const closeConfirmDelete = () => {
    setConfirmDeleteTarget(null);
    setConfirmDeleteError(null);
  };

  const tickets = useMemo(() => uploads.filter(u => u.file_type === 'ticket'), [uploads]);
  const vouchers = useMemo(() => uploads.filter(u => u.file_type === 'voucher'), [uploads]);
  const unparsedCount = useMemo(() => uploads.filter(u => !u.parsed_at).length, [uploads]);
  const busy = !!uploadingType || bulkParsing || !!deletingId;

  return {
    loading, error, uploads, tickets, vouchers, unparsedCount, busy,
    uploadingType, uploadProgress,
    bulkParsing, bulkProgress,
    parsingId, parseErrorById,
    deletingId, confirmDeleteTarget, confirmDeleteError,
    handleUpload, handleParseAll, requestDelete, confirmDelete, closeConfirmDelete,
    ticketRef, voucherRef,
  };
}

// ── Shared sub-components ────────────────────────────────────────────────────

function GrayDeleteButton({ onClick, disabled, busy }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title="Удалить"
      style={{
        background: 'none',
        border: 'none',
        cursor: disabled ? 'default' : 'pointer',
        color: 'var(--white-dim)',
        padding: '2px 6px',
        opacity: disabled ? 0.4 : 1,
        flexShrink: 0,
        display: 'inline-flex',
        alignItems: 'center',
      }}
    >
      {busy ? <span className="spinner" /> : <TrashIcon size={13} />}
    </button>
  );
}

function UploadRow({ u, parseError, isParsing, onDelete, deleting, busy }) {
  const name = uploadDisplayName(u);
  const date = formatUploadDate(u.created_at);
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '4px 8px',
          fontSize: 12,
          background: 'var(--graphite)',
          borderRadius: 4,
          opacity: isParsing ? 0.65 : 1,
        }}
      >
        <span
          style={{
            flex: 1,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            color: 'var(--white)',
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
          }}
          title={name}
        >
          {name || '—'}
        </span>
        {isParsing && <span className="spinner" />}
        {!isParsing && u.parsed_at && (
          <span style={{ fontSize: 10, color: 'var(--accent, #7fb77e)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            ✓
          </span>
        )}
        {date && (
          <span style={{ color: 'var(--white-dim)', fontSize: 10, fontVariantNumeric: 'tabular-nums' }}>
            {date}
          </span>
        )}
        <GrayDeleteButton onClick={onDelete} disabled={busy} busy={deleting} />
      </div>
      {parseError && (
        <div style={{ fontSize: 11, color: 'var(--danger)', paddingLeft: 8 }}>{parseError}</div>
      )}
    </div>
  );
}

function FilesGroup({ title, items, onUploadClick, uploading, uploadProgress, busy, hook }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          paddingBottom: 4,
          borderBottom: '1px solid var(--border)',
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
          {title}
        </span>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={onUploadClick}
          disabled={busy}
          style={{ fontSize: 11 }}
        >
          {uploading
            ? <><span className="spinner" /> {uploadProgress ? `${uploadProgress.done}/${uploadProgress.total}` : ''}</>
            : '+ загрузить'}
        </button>
      </div>
      {items.length === 0 ? (
        <div style={{ fontSize: 11, color: 'var(--white-dim)', paddingLeft: 4 }}>—</div>
      ) : (
        items.map(u => (
          <UploadRow
            key={u.id}
            u={u}
            parseError={hook.parseErrorById[u.id]}
            isParsing={hook.parsingId === u.id}
            deleting={hook.deletingId === u.id}
            onDelete={() => hook.requestDelete(u)}
            busy={hook.busy}
          />
        ))
      )}
    </div>
  );
}

function ParseAllButton({ hook, count, fullWidth }) {
  if (count === 0) return null;
  return (
    <button
      type="button"
      className="btn btn-primary btn-sm"
      onClick={hook.handleParseAll}
      disabled={hook.busy}
      style={fullWidth ? { width: '100%' } : undefined}
    >
      {hook.bulkParsing
        ? <><span className="spinner" /> Распознавание {hook.bulkProgress ? `${hook.bulkProgress.done}/${hook.bulkProgress.total}` : ''}…</>
        : `Распознать все (${count})`}
    </button>
  );
}

// ── Flight area (no card border, rows or single-line summary) ────────────────

function FlightLine({ label, value }) {
  return (
    <div style={{ display: 'flex', gap: 12, fontSize: 12, lineHeight: 1.5 }}>
      <span
        style={{
          width: 60,
          flexShrink: 0,
          color: 'var(--white-dim)',
          textTransform: 'uppercase',
          fontSize: 10,
          letterSpacing: '0.05em',
          paddingTop: 2,
        }}
      >
        {label}
      </span>
      <span style={{ color: 'var(--white)', fontFamily: 'var(--font-mono)' }}>{value}</span>
    </div>
  );
}

function FlightBlock({ tourist, onUpdated, layout = 'rows' }) {
  const [open, setOpen] = useState(false);
  const flight = useMemo(() => safeParse(tourist.flight_data) || {}, [tourist]);
  const arrivalStr = formatLeg(flight.arrival);
  const departureStr = formatLeg(flight.departure);
  const has = !isLegEmpty(flight.arrival) || !isLegEmpty(flight.departure);

  const handleSave = async (data) => {
    await updateFlightData(tourist.id, data);
    onUpdated?.();
  };
  const handleApplyToSubgroup = async (data) => {
    await updateFlightData(tourist.id, data);
    await applyFlightDataToSubgroup(tourist.id);
    onUpdated?.();
  };

  const form = (
    <FlightDataForm
      open={open}
      initial={flight}
      onClose={() => setOpen(false)}
      onSave={handleSave}
      canApplyToSubgroup={!!tourist.subgroup_id}
      onApplyToSubgroup={handleApplyToSubgroup}
    />
  );

  if (layout === 'summary') {
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 12, lineHeight: 1.4 }}>
        <span
          style={{
            width: 60,
            flexShrink: 0,
            color: 'var(--white-dim)',
            textTransform: 'uppercase',
            fontSize: 10,
            letterSpacing: '0.05em',
          }}
        >
          Рейсы
        </span>
        <span style={{ flex: 1, color: has ? 'var(--white)' : 'var(--white-dim)', fontFamily: 'var(--font-mono)' }}>
          {has ? `${arrivalStr || '—'}${departureStr ? `   →   ${departureStr}` : ''}` : 'не заданы'}
        </span>
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => setOpen(true)}>
          {has ? 'Изменить' : 'Добавить'}
        </button>
        {form}
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {has ? (
        <>
          {arrivalStr && <FlightLine label="Прилёт" value={arrivalStr} />}
          {departureStr && <FlightLine label="Обратно" value={departureStr} />}
          {!arrivalStr && !departureStr && (
            <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>Данные заполнены частично</div>
          )}
        </>
      ) : (
        <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>Нет данных о рейсах</div>
      )}
      <div>
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => setOpen(true)} style={{ marginTop: 4 }}>
          {has ? 'Изменить' : 'Добавить'}
        </button>
      </div>
      {form}
    </div>
  );
}

// ── Card header: name, DOB, subgroup picker, gray delete ─────────────────────

function CardHeader({ tourist, onDelete, subgroups, onAssign }) {
  const name = getTouristName(tourist);
  const snap = snapshotOf(tourist);
  const dob = snap.birth_date || snap.date_of_birth;
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
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
          <div style={{ fontSize: 11, color: 'var(--white-dim)', fontFamily: 'var(--font-mono)' }}>
            {dob}
          </div>
        )}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
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
        <GrayDeleteButton onClick={onDelete} />
      </div>
    </div>
  );
}

// ── Variant A: flat (everything visible) ─────────────────────────────────────

function VariantA({ tourist, onUpdated, hook }) {
  return (
    <>
      <FlightBlock tourist={tourist} onUpdated={onUpdated} layout="rows" />
      <FilesGroup
        title="Билеты"
        items={hook.tickets}
        onUploadClick={() => hook.ticketRef.current?.click()}
        uploading={hook.uploadingType === 'ticket'}
        uploadProgress={hook.uploadProgress}
        busy={hook.busy}
        hook={hook}
      />
      <FilesGroup
        title="Ваучеры"
        items={hook.vouchers}
        onUploadClick={() => hook.voucherRef.current?.click()}
        uploading={hook.uploadingType === 'voucher'}
        uploadProgress={hook.uploadProgress}
        busy={hook.busy}
        hook={hook}
      />
      {hook.error && <div style={{ fontSize: 11, color: 'var(--danger)', whiteSpace: 'pre-wrap' }}>{hook.error}</div>}
      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <ParseAllButton hook={hook} count={hook.unparsedCount} />
      </div>
    </>
  );
}

// ── Variant B: collapsible documents ─────────────────────────────────────────

function VariantB({ tourist, onUpdated, hook }) {
  const [open, setOpen] = useState(hook.unparsedCount > 0);
  const totalDocs = hook.uploads.length;
  return (
    <>
      <FlightBlock tourist={tourist} onUpdated={onUpdated} layout="summary" />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <button
          type="button"
          onClick={() => setOpen(o => !o)}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'var(--white-dim)',
            padding: 0,
            textAlign: 'left',
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            fontSize: 12,
          }}
        >
          <span
            style={{
              display: 'inline-block',
              transition: 'transform 0.15s',
              transform: open ? 'rotate(90deg)' : 'none',
              fontSize: 10,
            }}
          >
            ▶
          </span>
          <span style={{ textTransform: 'uppercase', letterSpacing: '0.05em', fontSize: 10 }}>
            Документы
          </span>
          <span style={{ fontSize: 11 }}>
            {totalDocs === 0
              ? '— ничего не загружено'
              : (hook.unparsedCount > 0
                  ? `${totalDocs}, не распознано: ${hook.unparsedCount}`
                  : `${totalDocs}, всё распознано`)}
          </span>
          {hook.unparsedCount > 0 && !open && (
            <span
              style={{
                background: 'var(--accent-dim)',
                color: 'var(--accent)',
                padding: '0 6px',
                borderRadius: 8,
                fontSize: 10,
                marginLeft: 'auto',
              }}
            >
              требует распознавания
            </span>
          )}
        </button>

        {open && (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 12,
              padding: '10px 12px',
              background: 'var(--graphite)',
              borderRadius: 6,
            }}
          >
            <FilesGroup
              title="Билеты"
              items={hook.tickets}
              onUploadClick={() => hook.ticketRef.current?.click()}
              uploading={hook.uploadingType === 'ticket'}
              uploadProgress={hook.uploadProgress}
              busy={hook.busy}
              hook={hook}
            />
            <FilesGroup
              title="Ваучеры"
              items={hook.vouchers}
              onUploadClick={() => hook.voucherRef.current?.click()}
              uploading={hook.uploadingType === 'voucher'}
              uploadProgress={hook.uploadProgress}
              busy={hook.busy}
              hook={hook}
            />
            {hook.error && <div style={{ fontSize: 11, color: 'var(--danger)', whiteSpace: 'pre-wrap' }}>{hook.error}</div>}
            {hook.unparsedCount > 0 && (
              <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <ParseAllButton hook={hook} count={hook.unparsedCount} />
              </div>
            )}
          </div>
        )}
      </div>
    </>
  );
}

// ── Variant C: two columns ───────────────────────────────────────────────────

function VariantC({ tourist, onUpdated, hook }) {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'minmax(220px, 1fr) minmax(280px, 1.4fr)',
        gap: 18,
      }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div style={{
          fontSize: 10, color: 'var(--white-dim)', textTransform: 'uppercase', letterSpacing: '0.05em',
          paddingBottom: 4, borderBottom: '1px solid var(--border)',
        }}>
          Рейсы
        </div>
        <FlightBlock tourist={tourist} onUpdated={onUpdated} layout="rows" />
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <FilesGroup
          title="Билеты"
          items={hook.tickets}
          onUploadClick={() => hook.ticketRef.current?.click()}
          uploading={hook.uploadingType === 'ticket'}
          uploadProgress={hook.uploadProgress}
          busy={hook.busy}
          hook={hook}
        />
        <FilesGroup
          title="Ваучеры"
          items={hook.vouchers}
          onUploadClick={() => hook.voucherRef.current?.click()}
          uploading={hook.uploadingType === 'voucher'}
          uploadProgress={hook.uploadProgress}
          busy={hook.busy}
          hook={hook}
        />
        {hook.error && <div style={{ fontSize: 11, color: 'var(--danger)', whiteSpace: 'pre-wrap' }}>{hook.error}</div>}
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <ParseAllButton hook={hook} count={hook.unparsedCount} />
        </div>
      </div>
    </div>
  );
}

// ── Public API ───────────────────────────────────────────────────────────────

export default function TouristCard({
  tourist, onDelete, subgroups, onAssign, onUpdated, variant = 'B',
}) {
  const hook = useTouristUploads(tourist.id, onUpdated);

  const Body = variant === 'A' ? VariantA : variant === 'C' ? VariantC : VariantB;

  return (
    <div
      style={{
        padding: '12px 14px',
        background: 'var(--gray-dark)',
        border: '1px solid var(--border)',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
      }}
    >
      <CardHeader tourist={tourist} onDelete={onDelete} subgroups={subgroups} onAssign={onAssign} />
      <Body tourist={tourist} onUpdated={onUpdated} hook={hook} />

      {/* Hidden file inputs — shared across variants */}
      <input
        ref={hook.ticketRef}
        type="file"
        accept=".pdf,.jpg,.jpeg,.png"
        multiple
        style={{ display: 'none' }}
        onChange={(e) => { hook.handleUpload(e.target.files, 'ticket'); e.target.value = ''; }}
      />
      <input
        ref={hook.voucherRef}
        type="file"
        accept=".pdf,.jpg,.jpeg,.png"
        multiple
        style={{ display: 'none' }}
        onChange={(e) => { hook.handleUpload(e.target.files, 'voucher'); e.target.value = ''; }}
      />

      <ConfirmModal
        open={!!hook.confirmDeleteTarget}
        title="Удалить файл?"
        message={hook.confirmDeleteTarget
          ? `Удалить «${uploadDisplayName(hook.confirmDeleteTarget) || FILE_TYPE_LABEL[hook.confirmDeleteTarget.file_type] || 'файл'}»?`
          : ''}
        confirmText="Удалить"
        cancelText="Отмена"
        variant="danger"
        busy={!!hook.deletingId}
        error={hook.confirmDeleteError}
        onConfirm={hook.confirmDelete}
        onCancel={hook.closeConfirmDelete}
      />
    </div>
  );
}
