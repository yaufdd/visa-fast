import { useRef, useState } from 'react';

// Multi-file companion to FileUploadField: a list of selected/uploaded
// files with per-file Скачать / Заменить affordances and a single drop
// zone at the bottom for adding more. Reuses the .ff-* styles from
// FileUploadField.
//
// `compact` shrinks the drop zone (no inner labels, just a small "+ файл"
// button) — used in the Документы step where the section sits inside a
// stack of three uploaders and full-size drop zones would crowd the page.
//
// Two modes (mirror of FileUploadField — see that file's header for the
// detailed contract):
//
//   filesMode === 'upload-now'        → uploads immediately on pick.
//   filesMode === 'upload-on-submit'  → keeps File refs in component
//                                       state; multipart submit ships
//                                       them later. No server calls
//                                       for delete / replace either.

const MAX_BYTES = 50 * 1024 * 1024;

function formatSize(bytes) {
  if (!bytes && bytes !== 0) return '';
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

function mimeAllowed(accept, mime) {
  if (!accept) return true;
  const list = accept.split(',').map((s) => s.trim()).filter(Boolean);
  if (list.length === 0) return true;
  return list.some((rule) => {
    if (rule.endsWith('/*')) return mime.startsWith(rule.slice(0, -1));
    if (rule.startsWith('.')) return true;
    return mime === rule;
  });
}

function localFileMeta(file) {
  return {
    id: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    original_name: file.name,
    size_bytes: file.size,
    mime_type: file.type || '',
    _localFile: file,
  };
}

export default function FileMultiUploadField({
  label,
  fileType,
  // Optional override for the actual server file type used when uploading.
  // Lets two UI slots share one backend type (e.g. passport_main and
  // passport_reg both upload as passport_internal). Ignored in
  // upload-on-submit mode where no server call is made during pick.
  uploadFileType,
  adapter,
  submissionId,
  currentFiles = [],
  onAdded,
  onRemoved,
  acceptMime = '',
  compact = false,
  showDelete = false,
  filesMode = 'upload-now',
}) {
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [busyFileId, setBusyFileId] = useState(null);
  const [error, setError] = useState('');
  const inputRef = useRef(null);
  const replaceInputRef = useRef(null);
  // Tracks which file is being replaced once the OS file picker returns.
  const replaceTargetRef = useRef(null);
  const [dragging, setDragging] = useState(false);

  const localOnly = filesMode === 'upload-on-submit';
  // In upload-now mode the widget needs a real submission row server-side;
  // upload-on-submit mode is purely client state, so it's always enabled.
  const disabled = localOnly ? false : !submissionId;

  const validateFile = (file) => {
    if (file.size > MAX_BYTES) return 'Файл слишком большой (>50 МБ)';
    if (acceptMime && file.type && !mimeAllowed(acceptMime, file.type)) {
      return 'Неподдерживаемый формат файла';
    }
    return '';
  };

  const handleUpload = async (file) => {
    const localErr = validateFile(file);
    if (localErr) { setError(localErr); return; }
    setError('');

    if (localOnly) {
      onAdded?.(localFileMeta(file));
      return;
    }

    setUploading(true);
    setProgress(0);
    try {
      const meta = await adapter.uploadFile(
        submissionId,
        uploadFileType || fileType,
        file,
        (pct) => setProgress(pct),
      );
      onAdded?.(meta);
    } catch (err) {
      setError(err?.message || 'Не удалось загрузить файл.');
    } finally {
      setUploading(false);
      setProgress(0);
    }
  };

  const pickFile = () => {
    if (disabled || uploading) return;
    setError('');
    inputRef.current?.click();
  };

  const handleInputChange = (e) => {
    const list = Array.from(e.target.files || []);
    e.target.value = '';
    if (list.length === 0) return;
    // Sequential — keeps the progress bar meaningful in upload-now mode
    // and avoids a flood of state updates in upload-on-submit mode.
    (async () => {
      for (const file of list) {
        // eslint-disable-next-line no-await-in-loop
        await handleUpload(file);
      }
    })();
  };

  const handleDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    if (disabled || uploading) return;
    const list = Array.from(e.dataTransfer.files || []);
    if (list.length === 0) return;
    (async () => {
      for (const file of list) {
        // eslint-disable-next-line no-await-in-loop
        await handleUpload(file);
      }
    })();
  };

  // Replace = upload new + delete old (server) OR swap the local meta in
  // place (client). In upload-now mode if upload fails, old stays in
  // place; if delete fails, both linger and the manager can clean up.
  const handleReplaceClick = (file) => {
    if (uploading || busyFileId) return;
    setError('');
    replaceTargetRef.current = file;
    replaceInputRef.current?.click();
  };
  const handleReplaceInput = async (e) => {
    const newFile = e.target.files?.[0];
    e.target.value = '';
    const oldFile = replaceTargetRef.current;
    replaceTargetRef.current = null;
    if (!newFile || !oldFile) return;
    const localErr = validateFile(newFile);
    if (localErr) { setError(localErr); return; }

    if (localOnly) {
      // Pure client-side swap: remove old meta, add new meta. Order
      // matters — onRemoved before onAdded keeps any duplicate-id guards
      // happy, though our local ids are unique by construction.
      onRemoved?.(oldFile.id);
      onAdded?.(localFileMeta(newFile));
      return;
    }

    setBusyFileId(oldFile.id);
    setError('');
    try {
      const meta = await adapter.uploadFile(submissionId, uploadFileType || fileType, newFile);
      onAdded?.(meta);
      try {
        await adapter.deleteFile(submissionId, oldFile.id);
        onRemoved?.(oldFile.id);
      } catch (innerErr) {
        setError(innerErr?.message || 'Не удалось убрать старый файл.');
      }
    } catch (err) {
      setError(err?.message || 'Не удалось заменить файл.');
    } finally {
      setBusyFileId(null);
    }
  };

  // Delete a single file. Server-side in upload-now mode, in-memory
  // drop in upload-on-submit mode. The original implementation didn't
  // have a delete handler at all (showDelete-gated) — we add one here
  // because the public form needs to let tourists clear a wrong pick.
  const handleDelete = async (file) => {
    if (!file?.id || uploading || busyFileId) return;
    setError('');
    if (localOnly) {
      onRemoved?.(file.id);
      return;
    }
    setBusyFileId(file.id);
    try {
      await adapter.deleteFile(submissionId, file.id);
      onRemoved?.(file.id);
    } catch (err) {
      setError(err?.message || 'Не удалось удалить файл.');
    } finally {
      setBusyFileId(null);
    }
  };

  // In upload-on-submit mode the tourist always gets a Delete button —
  // there is no server row to "preserve" by hiding it.
  const showDeleteEffective = showDelete || localOnly;

  return (
    <div className="ff-wrap" data-field={`file_${fileType}`}>
      <span className="ff-label">{label}</span>

      <input
        ref={inputRef}
        type="file"
        accept={acceptMime || undefined}
        onChange={handleInputChange}
        multiple
        style={{ display: 'none' }}
      />
      <input
        ref={replaceInputRef}
        type="file"
        accept={acceptMime || undefined}
        onChange={handleReplaceInput}
        style={{ display: 'none' }}
      />

      {currentFiles.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          {currentFiles.map((f) => (
            <div key={f.id} className="ff-file">
              <div className="ff-file-info">
                <div className="ff-file-name" title={f.original_name}>
                  {f.original_name || 'файл'}
                </div>
                <div className="ff-file-meta">
                  {formatSize(f.size_bytes)}
                  {f.mime_type ? ` · ${f.mime_type}` : ''}
                </div>
              </div>
              <div className="ff-file-actions">
                {!localOnly && adapter?.downloadUrl && f.id && (
                  <a
                    className="ff-btn ff-icon-btn"
                    href={adapter.downloadUrl(submissionId, f.id)}
                    download
                    title="Скачать"
                    aria-label="Скачать"
                  >
                    <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                      <path d="M8 2.5v8M8 10.5l-3-3M8 10.5l3-3M3 13h10"
                        stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                    </svg>
                  </a>
                )}
                <button
                  type="button"
                  className="ff-btn ff-icon-btn"
                  onClick={() => handleReplaceClick(f)}
                  disabled={uploading || busyFileId === f.id}
                  title="Заменить файл"
                  aria-label="Заменить"
                >
                  {busyFileId === f.id ? (
                    <span className="spinner" />
                  ) : (
                    <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                      <path d="M3 5h9M9.5 2.5 12 5l-2.5 2.5M13 11H4M6.5 13.5 4 11l2.5-2.5"
                        stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
                    </svg>
                  )}
                </button>
                {showDeleteEffective && (
                  <button
                    type="button"
                    className="ff-btn ff-icon-btn"
                    onClick={() => handleDelete(f)}
                    disabled={uploading || busyFileId === f.id}
                    title="Удалить файл"
                    aria-label="Удалить"
                  >
                    <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                      <path d="M3 4h10M6.5 4V2.5a1 1 0 0 1 1-1h1a1 1 0 0 1 1 1V4M4 4l.5 8.5a1.5 1.5 0 0 0 1.5 1.4h4a1.5 1.5 0 0 0 1.5-1.4L12 4M6.5 7v4M9.5 7v4"
                        stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
                    </svg>
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {uploading && (
        <div className="ff-uploading">
          <div className="ff-progress">
            <div className="ff-progress-bar" style={{ width: `${progress}%` }} />
          </div>
          <div className="ff-progress-label">Загрузка… {progress}%</div>
        </div>
      )}

      {!uploading && (
        <div
          className={`ff-drop${compact ? ' ff-drop-compact' : ''}${dragging ? ' is-dragging' : ''}${disabled ? ' is-disabled' : ''}`}
          onClick={pickFile}
          onDragOver={(e) => { e.preventDefault(); if (!disabled) setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={handleDrop}
          role="button"
          tabIndex={disabled ? -1 : 0}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pickFile(); } }}
        >
          {compact ? (
            <div className="ff-drop-compact-label">+ файл</div>
          ) : (
            <>
              <div className="ff-drop-title">
                {currentFiles.length > 0 ? 'Добавить ещё файл' : 'Перетащите файл сюда'}
              </div>
              <div className="ff-drop-sub">или нажмите, чтобы выбрать</div>
              <div className="ff-drop-hint">PDF, JPEG, PNG · до 50 МБ · можно несколько</div>
            </>
          )}
        </div>
      )}

      {error && (
        <div className="ff-error">
          {error}
          <button type="button" className="ff-error-clear" onClick={() => setError('')}>
            ×
          </button>
        </div>
      )}
    </div>
  );
}
