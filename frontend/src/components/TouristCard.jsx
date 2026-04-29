import { forwardRef, useImperativeHandle, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  uploadTouristFile,
  parseTouristUpload,
  updateFlightData,
  applyFlightDataToSubgroup,
} from '../api/client';
import FlightDataForm from './FlightDataForm';
import ConfirmModal from './ConfirmModal';
import TouristFilesModal from './TouristFilesModal';
import { formatAIError } from '../utils/aiError';

// ── Shared helpers ───────────────────────────────────────────────────────────

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

function isLegEmpty(leg) {
  return !leg || (!leg.flight_number && !leg.date && !leg.time && !leg.airport);
}

// ── Icons ────────────────────────────────────────────────────────────────────

function CrossIcon({ size = 13 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M4 4l8 8M12 4l-8 8"
        stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  );
}

function UploadIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M8 10.5V3M8 3l-3 3M8 3l3 3M3 13h10"
        stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}

// Pencil — used for the "edit" / "change" action.
function EditIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M11 2.5l2.5 2.5L5 13.5H2.5V11L11 2.5z"
        stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round"/>
      <path d="M9.5 4l2.5 2.5" stroke="currentColor" strokeWidth="1.3"/>
    </svg>
  );
}

// Sparkle / 4-point star — used for "auto-fill from a scan" actions.
function MagicIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M8 1.5L9.2 6.8L14.5 8L9.2 9.2L8 14.5L6.8 9.2L1.5 8L6.8 6.8Z"
        fill="currentColor"/>
    </svg>
  );
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
      {busy ? <span className="spinner" /> : <CrossIcon size={13} />}
    </button>
  );
}

// ── Flight area ─────────────────────────────────────────────────────────────

function FlightLegRow({ label, leg }) {
  const datetime = [leg?.date, leg?.time].filter(Boolean).join(' ');
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'baseline',
        gap: 16,
        fontSize: 12,
        lineHeight: 1.55,
        minWidth: 0,
      }}
    >
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
        {label}
      </span>
      <span
        style={{
          width: 70,
          flexShrink: 0,
          color: 'var(--white)',
          fontFamily: 'var(--font-mono)',
        }}
      >
        {leg?.flight_number || ''}
      </span>
      <span
        style={{
          color: 'var(--white)',
          fontFamily: 'var(--font-mono)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          minWidth: 0,
        }}
        title={leg?.airport || ''}
      >
        {leg?.airport || ''}
      </span>
      <span
        className="tc-flight-datetime"
        style={{
          color: 'var(--white)',
          fontFamily: 'var(--font-mono)',
          fontVariantNumeric: 'tabular-nums',
          flexShrink: 0,
        }}
      >
        {datetime}
      </span>
    </div>
  );
}

// FlightSection: collapsible area with flight info + edit / apply / upload-ticket
// actions. The ticket upload runs the upload+parse pair under the hood and
// updates the tourist's flight_data — the file itself is not surfaced here.
//
// Imperative handle: parent can call `ref.current.parseExisting(uploadIds)` to
// run the same parse loop against tourist_uploads that already exist (e.g.
// triggered from the documents modal). The shared spinner / progress / error
// state inside the section is reused so the visual feedback matches the
// regular upload flow.
const FlightSection = forwardRef(function FlightSection({ tourist, onUpdated }, ref) {
  const flight = useMemo(() => safeParse(tourist.flight_data) || {}, [tourist]);
  const hasArrival = !isLegEmpty(flight.arrival);
  const hasDeparture = !isLegEmpty(flight.departure);
  const has = hasArrival || hasDeparture;
  const canApply = has && !!tourist.subgroup_id;

  const [expanded, setExpanded] = useState(true);
  const [editOpen, setEditOpen] = useState(false);
  const [confirmApplyOpen, setConfirmApplyOpen] = useState(false);
  const [applying, setApplying] = useState(false);
  const [actionError, setActionError] = useState(null);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(null);
  const fileInputRef = useRef(null);

  const handleSave = async (data) => {
    await updateFlightData(tourist.id, data);
    onUpdated?.();
  };

  const handleApplyToSubgroup = async () => {
    setApplying(true);
    setActionError(null);
    try {
      await applyFlightDataToSubgroup(tourist.id);
      setConfirmApplyOpen(false);
      onUpdated?.();
    } catch (e) {
      setActionError(e.message);
    } finally {
      setApplying(false);
    }
  };

  const handleUpload = async (files) => {
    const list = files ? Array.from(files) : [];
    if (list.length === 0) return;
    setUploading(true);
    setActionError(null);
    setExpanded(true);
    setProgress({ done: 0, total: list.length });
    try {
      // Each file gets uploaded, then immediately parsed so the flight_data
      // on the tourist is filled in. The file row stays server-side (audit
      // log + reupload safety) but is not surfaced anywhere on this screen.
      for (let i = 0; i < list.length; i++) {
        const up = await uploadTouristFile(tourist.id, list[i], 'ticket');
        const res = await parseTouristUpload(tourist.id, up.id);
        if (res?.parse_error) {
          setActionError(formatAIError({ message: res.parse_error }));
        }
        setProgress({ done: i + 1, total: list.length });
      }
      onUpdated?.();
    } catch (e) {
      setActionError(formatAIError(e));
    } finally {
      setUploading(false);
      setProgress(null);
    }
  };

  // Same parse loop, but against existing tourist_uploads (e.g. triggered
  // from the modal's magic button). No upload step.
  const parseExisting = async (uploadIds) => {
    if (!Array.isArray(uploadIds) || uploadIds.length === 0) return;
    setUploading(true);
    setActionError(null);
    setExpanded(true);
    setProgress({ done: 0, total: uploadIds.length });
    try {
      for (let i = 0; i < uploadIds.length; i++) {
        const res = await parseTouristUpload(tourist.id, uploadIds[i]);
        if (res?.parse_error) {
          setActionError(formatAIError({ message: res.parse_error }));
        }
        setProgress({ done: i + 1, total: uploadIds.length });
      }
      onUpdated?.();
    } catch (e) {
      setActionError(formatAIError(e));
    } finally {
      setUploading(false);
      setProgress(null);
    }
  };

  useImperativeHandle(ref, () => ({ parseExisting }));

  const summary = has
    ? [
        flight.arrival?.date,
        flight.departure?.date && `→ ${flight.departure.date}`,
      ].filter(Boolean).join(' ') || 'есть данные'
    : '';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <button
        type="button"
        onClick={() => setExpanded(o => !o)}
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
            transform: expanded ? 'rotate(90deg)' : 'none',
            fontSize: 10,
          }}
        >
          ▶
        </span>
        <span style={{ textTransform: 'uppercase', letterSpacing: '0.05em', fontSize: 10 }}>
          Авиабилеты
        </span>
        <span style={{ fontSize: 11 }}>{summary}</span>
      </button>

      {expanded && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
            padding: '10px 12px',
            background: 'var(--graphite)',
            borderRadius: 6,
          }}
        >
          {has ? (
            <div
              className="tc-flight-row"
              style={{
                display: 'flex',
                gap: 12,
                alignItems: 'flex-start',
                justifyContent: 'space-between',
              }}
            >
              <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
                {hasArrival && <FlightLegRow label="Прилёт" leg={flight.arrival} />}
                {hasDeparture && <FlightLegRow label="Вылет" leg={flight.departure} />}
              </div>
              <div
                className="tc-flight-actions"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'flex-end',
                  gap: 6,
                  flexShrink: 0,
                }}
              >
                <button
                  type="button"
                  className="btn btn-secondary btn-sm"
                  onClick={() => setEditOpen(true)}
                  title="Изменить рейс"
                  aria-label="Изменить рейс"
                  style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
                >
                  <EditIcon />
                </button>
                <button
                  type="button"
                  className="btn btn-ghost btn-sm btn-magic"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploading}
                  title="Загрузить скан билета — данные распознаются автоматически"
                  aria-label="Заполнить из скана билета"
                  style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
                >
                  {uploading ? (
                    <>
                      <span className="spinner" />
                      {progress && progress.total > 1 && (
                        <span style={{ fontSize: 10, marginLeft: 4, fontVariantNumeric: 'tabular-nums' }}>
                          {progress.done}/{progress.total}
                        </span>
                      )}
                    </>
                  ) : <MagicIcon />}
                </button>
                {canApply && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => setConfirmApplyOpen(true)}
                    disabled={applying}
                    title="Скопировать этот перелёт всем туристам в этой группе"
                    style={{ fontSize: 11, whiteSpace: 'nowrap' }}
                  >
                    {applying
                      ? <><span className="spinner" /> Применение…</>
                      : '↯ Применить ко всей группе'}
                  </button>
                )}
              </div>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 8 }}>
              <button
                type="button"
                className="btn btn-ghost btn-sm btn-magic"
                onClick={() => fileInputRef.current?.click()}
                disabled={uploading}
                title="Загрузить скан билета — данные распознаются автоматически"
                aria-label="Заполнить из скана билета"
                style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
              >
                {uploading ? (
                  <>
                    <span className="spinner" />
                    {progress && progress.total > 1 && (
                      <span style={{ fontSize: 10, marginLeft: 4, fontVariantNumeric: 'tabular-nums' }}>
                        {progress.done}/{progress.total}
                      </span>
                    )}
                  </>
                ) : <MagicIcon />}
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => setEditOpen(true)}
              >
                + Добавить авиа
              </button>
            </div>
          )}
          {actionError && (
            <div style={{ fontSize: 11, color: 'var(--danger)' }}>{actionError}</div>
          )}
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        accept=".pdf,.jpg,.jpeg,.png"
        multiple
        style={{ display: 'none' }}
        onChange={(e) => { handleUpload(e.target.files); e.target.value = ''; }}
      />

      <FlightDataForm
        open={editOpen}
        initial={flight}
        onClose={() => setEditOpen(false)}
        onSave={handleSave}
      />
      <ConfirmModal
        open={confirmApplyOpen}
        title="Применить ко всей группе?"
        message="Скопировать этот перелёт всем остальным туристам в этой группе? Их текущие данные о рейсах будут перезаписаны."
        confirmText="Применить"
        cancelText="Отмена"
        variant="primary"
        busy={applying}
        onConfirm={handleApplyToSubgroup}
        onCancel={() => !applying && setConfirmApplyOpen(false)}
      />
    </div>
  );
});

// ── Card header ──────────────────────────────────────────────────────────────

function CardHeader({ tourist, onDelete, subgroups, onAssign, fileCount, onOpenFiles }) {
  const name = getTouristName(tourist);
  const showFiles = !!tourist.submission_id && fileCount > 0;
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
      <div style={{ minWidth: 0 }}>
        <Link
          to={`/tourists/${tourist.id}`}
          title="Открыть карточку туриста"
          style={{
            display: 'block',
            fontSize: 13,
            fontWeight: 500,
            color: 'var(--white)',
            textDecoration: 'none',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            cursor: 'pointer',
          }}
          onMouseEnter={(e) => { e.currentTarget.style.textDecoration = 'underline'; }}
          onMouseLeave={(e) => { e.currentTarget.style.textDecoration = 'none'; }}
        >
          {name}
        </Link>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
        {showFiles && (
          <button
            type="button"
            onClick={onOpenFiles}
            title="Файлы из публичной формы"
            aria-label={`Файлы туриста (${fileCount})`}
            style={{
              background: 'none',
              border: '1px solid var(--border)',
              borderRadius: 4,
              color: 'var(--white-dim)',
              fontSize: 11,
              padding: '2px 8px',
              cursor: 'pointer',
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4,
              transition: 'color 0.15s, border-color 0.15s',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.color = 'var(--white)';
              e.currentTarget.style.borderColor = 'var(--white-dim)';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = 'var(--white-dim)';
              e.currentTarget.style.borderColor = 'var(--border)';
            }}
          >
            <span aria-hidden="true">📎</span>
            <span style={{ fontVariantNumeric: 'tabular-nums' }}>{fileCount}</span>
          </button>
        )}
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

// ── Public API ───────────────────────────────────────────────────────────────

export default function TouristCard({
  tourist, onDelete, subgroups, onAssign, onUpdated, fileCount = 0, onFilesChanged,
  onVoucherParseRequest,
}) {
  const [filesOpen, setFilesOpen] = useState(false);
  const flightRef = useRef(null);
  const closeFiles = () => {
    setFilesOpen(false);
    onFilesChanged?.();
  };

  // Triggered by the modal's magic button. Closes the modal first so the
  // user sees the spinner appear in the relevant section, not in the modal.
  const handleParseFromModal = (fileType, uploadIds) => {
    setFilesOpen(false);
    onFilesChanged?.();
    if (fileType === 'ticket') {
      flightRef.current?.parseExisting(uploadIds);
    } else if (fileType === 'voucher') {
      onVoucherParseRequest?.(tourist.id, uploadIds);
    }
  };

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
      <CardHeader
        tourist={tourist}
        onDelete={onDelete}
        subgroups={subgroups}
        onAssign={onAssign}
        fileCount={fileCount}
        onOpenFiles={() => setFilesOpen(true)}
      />
      <FlightSection ref={flightRef} tourist={tourist} onUpdated={onUpdated} />

      <TouristFilesModal
        open={filesOpen}
        onClose={closeFiles}
        submissionId={tourist.submission_id}
        touristId={tourist.id}
        touristName={getTouristName(tourist)}
        onUpdated={onUpdated}
        onParseRequest={handleParseFromModal}
      />
    </div>
  );
}
