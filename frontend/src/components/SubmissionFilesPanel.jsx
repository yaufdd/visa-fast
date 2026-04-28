import { useCallback, useEffect, useState } from 'react';
import {
  listSubmissionFilesAdmin,
  submissionFileDownloadUrl,
  deleteSubmissionFileAdmin,
  getTouristUploads,
} from '../api/client';
import ConfirmModal from './ConfirmModal';

// Russian labels per file_type. Keep order stable (used for grouped render).
const TYPE_LABELS = {
  passport_internal: 'Внутренний паспорт',
  passport_foreign: 'Заграничный паспорт',
  ticket: 'Авиабилет',
  voucher: 'Ваучер',
};
const TYPE_ORDER = ['passport_internal', 'passport_foreign', 'ticket', 'voucher'];
const PARSEABLE = new Set(['ticket', 'voucher']);

function humanSize(bytes) {
  if (bytes == null || Number.isNaN(bytes)) return '';
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(kb < 10 ? 1 : 0)} KB`;
  const mb = kb / 1024;
  return `${mb.toFixed(mb < 10 ? 1 : 0)} MB`;
}

function fmtDate(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n) => String(n).padStart(2, '0');
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`;
}

function DownloadIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M8 2.5v8M8 10.5l-3-3M8 10.5l3-3M3 13h10"
        stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}

function MagicIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M8 1.5L9.2 6.8L14.5 8L9.2 9.2L8 14.5L6.8 9.2L1.5 8L6.8 6.8Z"
        fill="currentColor"/>
    </svg>
  );
}

function TrashIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M3 4h10M6.5 4V2.5a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1V4M4 4l.5 8.5a1.5 1.5 0 0 0 1.5 1.4h4a1.5 1.5 0 0 0 1.5-1.4L12 4M6.5 7v4M9.5 7v4"
        stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}

export default function SubmissionFilesPanel({
  submissionId, touristId, onUpdated, allowDelete = false, onParseRequest,
}) {
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(true);
  // notFound = the list endpoint returned 404 (cross-org or stale id).
  // We render nothing in that case — the parent page handles its own 404.
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState(null);
  const [deletingId, setDeletingId] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteError, setDeleteError] = useState(null);
  // Tourist-side uploads — needed to find the upload ids that map to the
  // visible submission files when the manager triggers parsing from here.
  const [touristUploads, setTouristUploads] = useState([]);

  const load = useCallback(async (signal) => {
    if (!submissionId) return;
    setError(null);
    try {
      const data = await listSubmissionFilesAdmin(submissionId);
      if (signal?.aborted) return;
      setFiles(Array.isArray(data) ? data : []);
      setNotFound(false);
    } catch (e) {
      if (signal?.aborted) return;
      if (e && e.notFound) {
        setNotFound(true);
        setFiles([]);
      } else {
        setError(e.message || 'Не удалось загрузить файлы');
      }
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [submissionId]);

  const loadUploads = useCallback(async () => {
    if (!touristId) return;
    try {
      const data = await getTouristUploads(touristId);
      setTouristUploads(Array.isArray(data) ? data : []);
    } catch {
      // non-critical
    }
  }, [touristId]);

  useEffect(() => {
    const ctrl = { aborted: false };
    setLoading(true);
    load(ctrl);
    loadUploads();
    return () => { ctrl.aborted = true; };
  }, [load, loadUploads]);

  const requestDelete = useCallback((file) => {
    setDeleteError(null);
    setDeleteTarget(file);
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    const file = deleteTarget;
    setDeletingId(file.id);
    setDeleteError(null);
    try {
      await deleteSubmissionFileAdmin(submissionId, file.id);
      setDeleteTarget(null);
      setLoading(true);
      await load();
    } catch (e) {
      setDeleteError(e.message || 'Не удалось удалить файл');
    } finally {
      setDeletingId(null);
    }
  }, [submissionId, load, deleteTarget]);

  // Parse trigger: pick the N latest tourist_uploads of this type (where N
  // matches what's visible in this section), reverse to upload order, and
  // hand the IDs off to the parent — the parent closes us and runs the parse
  // in the relevant card section.
  const handleTriggerParse = useCallback((fileType) => {
    if (!touristId || !onParseRequest) return;
    const visibleCount = files.filter(f => f.file_type === fileType).length;
    const ids = touristUploads
      .filter(u => u.file_type === fileType)
      .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
      .slice(0, visibleCount)
      .reverse()
      .map(u => u.id);
    if (ids.length === 0) {
      setError('Файл ещё не привязан к туристу. Откройте карточку и загрузите его заново.');
      return;
    }
    onParseRequest(fileType, ids);
  }, [touristId, files, touristUploads, onParseRequest]);

  if (notFound) return null;

  if (loading) {
    return (
      <div style={{ marginTop: 24, color: 'var(--white-dim)', fontSize: 13 }}>
        Загрузка...
      </div>
    );
  }

  // Group files by type, preserving TYPE_ORDER.
  const grouped = new Map();
  for (const f of files) {
    const key = f.file_type;
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key).push(f);
  }
  const knownGroups = TYPE_ORDER
    .filter((k) => grouped.has(k))
    .map((k) => [k, grouped.get(k)]);
  const unknownGroups = Array.from(grouped.entries())
    .filter(([k]) => !TYPE_ORDER.includes(k));
  const allGroups = [...knownGroups, ...unknownGroups];

  return (
    <div
      style={{
        marginTop: 28,
        border: '1px solid var(--border)',
        borderRadius: 10,
        padding: 20,
        background: 'var(--gray-dark)',
      }}
    >
      <div className="section-header" style={{ marginBottom: 14 }}>
        <div className="section-title">Прикреплённые документы</div>
      </div>

      {error && <div className="error-message">{error}</div>}

      {files.length === 0 ? (
        <div style={{ color: 'var(--white-dim)', fontSize: 13 }}>
          Турист не прикрепил файлов.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
          {allGroups.map(([typeKey, group]) => {
            const canParse = touristId && PARSEABLE.has(typeKey) && !!onParseRequest;
            return (
              <div key={typeKey}>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    marginBottom: 8,
                  }}
                >
                  <div
                    style={{
                      fontSize: 11,
                      fontWeight: 600,
                      textTransform: 'uppercase',
                      letterSpacing: '0.06em',
                      color: 'var(--white-dim)',
                    }}
                  >
                    {TYPE_LABELS[typeKey] || typeKey}
                  </div>
                  {canParse && (
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm btn-magic"
                      onClick={() => handleTriggerParse(typeKey)}
                      title={typeKey === 'ticket'
                        ? 'Распознать билет — данные о рейсах заполнятся автоматически'
                        : 'Распознать ваучер — отели заполнятся автоматически'}
                      aria-label="Распознать"
                      style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
                    >
                      <MagicIcon />
                    </button>
                  )}
                </div>
                <div
                  style={{
                    border: '1px solid var(--border)',
                    borderRadius: 6,
                    background: 'var(--graphite, #161616)',
                    overflow: 'hidden',
                  }}
                >
                  {group.map((f, i) => (
                    <div
                      key={f.id}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 12,
                        padding: '10px 12px',
                        borderTop: i === 0 ? 'none' : '1px solid var(--border)',
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div
                          style={{
                            color: 'var(--white)',
                            fontSize: 13,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                          title={f.original_name}
                        >
                          {f.original_name}
                        </div>
                        <div
                          style={{
                            color: 'var(--white-dim)',
                            fontSize: 11,
                            marginTop: 2,
                            display: 'flex',
                            gap: 10,
                          }}
                        >
                          <span>{humanSize(f.size_bytes)}</span>
                          <span>{fmtDate(f.created_at)}</span>
                        </div>
                      </div>
                      <a
                        className="btn btn-ghost btn-sm"
                        href={submissionFileDownloadUrl(submissionId, f.id)}
                        download
                        title="Скачать"
                        aria-label="Скачать"
                        style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
                      >
                        <DownloadIcon />
                      </a>
                      {allowDelete && (
                        <button
                          type="button"
                          onClick={() => requestDelete(f)}
                          disabled={deletingId === f.id}
                          title="Удалить файл"
                          aria-label="Удалить файл"
                          style={{
                            background: 'none',
                            border: 'none',
                            cursor: deletingId === f.id ? 'default' : 'pointer',
                            color: 'var(--white-dim)',
                            padding: '4px 6px',
                            borderRadius: 4,
                            display: 'inline-flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            opacity: deletingId === f.id ? 0.5 : 1,
                            transition: 'color 0.15s, background 0.15s',
                          }}
                          onMouseEnter={e => { if (deletingId === f.id) return; e.currentTarget.style.color = 'var(--white)'; e.currentTarget.style.background = 'rgba(255,255,255,0.06)'; }}
                          onMouseLeave={e => { if (deletingId === f.id) return; e.currentTarget.style.color = 'var(--white-dim)'; e.currentTarget.style.background = 'none'; }}
                        >
                          {deletingId === f.id ? <span className="spinner" /> : <TrashIcon />}
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}

      <ConfirmModal
        open={!!deleteTarget}
        title="Удалить файл?"
        message={deleteTarget ? `Удалить файл «${deleteTarget.original_name}»? Удалённый файл не восстановить.` : ''}
        confirmText="Удалить"
        cancelText="Отмена"
        variant="danger"
        busy={!!deletingId}
        error={deleteError}
        onConfirm={confirmDelete}
        onCancel={() => { if (!deletingId) { setDeleteTarget(null); setDeleteError(null); } }}
      />
    </div>
  );
}
