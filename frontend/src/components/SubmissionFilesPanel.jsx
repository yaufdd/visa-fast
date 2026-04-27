import { useCallback, useEffect, useState } from 'react';
import {
  listSubmissionFilesAdmin,
  submissionFileDownloadUrl,
  deleteSubmissionFileAdmin,
} from '../api/client';

// Russian labels per file_type. Keep order stable (used for grouped render).
const TYPE_LABELS = {
  passport_internal: 'Внутренний паспорт',
  passport_foreign: 'Заграничный паспорт',
  ticket: 'Авиабилет',
  voucher: 'Ваучер',
};
const TYPE_ORDER = ['passport_internal', 'passport_foreign', 'ticket', 'voucher'];

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

export default function SubmissionFilesPanel({ submissionId }) {
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(true);
  // notFound = the list endpoint returned 404 (cross-org or stale id).
  // We render nothing in that case — the parent page handles its own 404.
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState(null);
  const [deletingId, setDeletingId] = useState(null);

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

  useEffect(() => {
    const ctrl = { aborted: false };
    setLoading(true);
    load(ctrl);
    return () => { ctrl.aborted = true; };
  }, [load]);

  const handleDelete = useCallback(async (file) => {
    const ok = window.confirm(`Удалить файл «${file.original_name}»?`);
    if (!ok) return;
    setDeletingId(file.id);
    setError(null);
    try {
      await deleteSubmissionFileAdmin(submissionId, file.id);
      setLoading(true);
      await load();
    } catch (e) {
      setError(e.message || 'Не удалось удалить файл');
    } finally {
      setDeletingId(null);
    }
  }, [submissionId, load]);

  if (notFound) return null;

  if (loading) {
    return (
      <div style={{ marginTop: 24, color: 'var(--white-dim)', fontSize: 13 }}>
        Загрузка...
      </div>
    );
  }

  // Group files by type, preserving TYPE_ORDER. Unknown types fall through
  // under their raw key at the end.
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
          {allGroups.map(([typeKey, group]) => (
            <div key={typeKey}>
              <div
                style={{
                  fontSize: 11,
                  fontWeight: 600,
                  textTransform: 'uppercase',
                  letterSpacing: '0.06em',
                  color: 'var(--white-dim)',
                  marginBottom: 8,
                }}
              >
                {TYPE_LABELS[typeKey] || typeKey}
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                {group.map((f) => (
                  <div
                    key={f.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '10px 12px',
                      border: '1px solid var(--border)',
                      borderRadius: 6,
                      background: 'var(--graphite, #161616)',
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
                    >
                      Скачать
                    </a>
                    <button
                      type="button"
                      className="btn btn-danger btn-sm"
                      onClick={() => handleDelete(f)}
                      disabled={deletingId === f.id}
                    >
                      {deletingId === f.id ? 'Удаление...' : 'Удалить'}
                    </button>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
