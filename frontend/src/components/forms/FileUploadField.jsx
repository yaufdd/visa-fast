import { useEffect, useRef, useState } from 'react';

// 50 MB — matches the backend cap (handlers_public_files.go
// maxSubmissionFileSize). Keeping the client check in sync avoids a
// round-trip to discover the limit; the server still enforces it.
const MAX_BYTES = 50 * 1024 * 1024;

function formatSize(bytes) {
  if (!bytes && bytes !== 0) return '';
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}

// mimeAllowed is a forgiving prefix/wildcard check matching how the
// browser interprets the input[type=file] accept attr ("image/*" should
// match "image/jpeg", "application/pdf" should match exactly).
function mimeAllowed(accept, mime) {
  if (!accept) return true;
  const list = accept.split(',').map((s) => s.trim()).filter(Boolean);
  if (list.length === 0) return true;
  return list.some((rule) => {
    if (rule.endsWith('/*')) {
      const prefix = rule.slice(0, -1); // "image/"
      return mime.startsWith(prefix);
    }
    if (rule.startsWith('.')) {
      // We can't reach the filename from the mime alone; skip — the
      // input element already filters by extension client-side, and the
      // server validates by sniffing the bytes. Accept here.
      return true;
    }
    return mime === rule;
  });
}

// Build a synthetic "file meta" object that mimics the backend response
// shape (id / original_name / size_bytes / mime_type) so the rest of the
// wizard treats locally-picked and server-uploaded files uniformly. The
// `_localFile` slot carries the actual File object — the public adapter
// pulls it out at submit time to attach to the multipart body.
function localFileMeta(file) {
  return {
    id: `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    original_name: file.name,
    size_bytes: file.size,
    mime_type: file.type || '',
    _localFile: file,
  };
}

// FileUploadField — single-file widget. Has two modes:
//
//   filesMode === 'upload-now' (admin)
//     Picks → uploads to the backend immediately via adapter.uploadFile.
//     Shows a progress bar while bytes go up. Replace / Delete also hit
//     the backend. This is what the manager wizard uses — the row exists
//     server-side from the moment the page mounts.
//
//   filesMode === 'upload-on-submit' (public, default)
//     Picks → keeps the File in component state via onUploaded(meta) where
//     meta carries `_localFile`. No network call. Replace clears + repicks;
//     Delete (when shown) clears state. The parent wizard ships the File
//     refs as part of the final multipart submit.
//
// States in 'upload-now' mode:
//   empty / uploading (progress) / uploaded / parsing / error
// States in 'upload-on-submit' mode:
//   empty / uploaded / error  (no uploading or parsing)
export default function FileUploadField({
  label,
  fileType,
  adapter,
  submissionId,
  currentFile = null,
  onUploaded,
  onDeleted,
  onAutoFill,
  parseType,
  acceptMime = '',
  showDelete = false,
  filesMode = 'upload-now',
}) {
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [parsing, setParsing] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef(null);
  const [dragging, setDragging] = useState(false);

  const localOnly = filesMode === 'upload-on-submit';

  // Reset progress whenever the parent swaps the underlying file (e.g.
  // after deletion or replacement) so a stale 100% bar doesn't flash.
  useEffect(() => {
    setProgress(0);
  }, [currentFile?.id]);

  // In upload-now mode the widget is gated on a real submissionId. In
  // upload-on-submit mode there is no submission row yet — the picker
  // is always enabled.
  const disabled = localOnly ? false : !submissionId;

  const pickFile = () => {
    if (disabled || uploading || parsing) return;
    setError('');
    inputRef.current?.click();
  };

  const validateFile = (file) => {
    if (file.size > MAX_BYTES) {
      return 'Файл слишком большой (>50 МБ)';
    }
    if (acceptMime && file.type && !mimeAllowed(acceptMime, file.type)) {
      return 'Неподдерживаемый формат файла';
    }
    return '';
  };

  const handlePick = async (file) => {
    const localErr = validateFile(file);
    if (localErr) {
      setError(localErr);
      return;
    }
    setError('');

    if (localOnly) {
      // No network — hand a synthetic meta carrying the File reference up.
      onUploaded?.(localFileMeta(file));
      return;
    }

    setUploading(true);
    setProgress(0);
    try {
      const meta = await adapter.uploadFile(
        submissionId,
        fileType,
        file,
        (pct) => setProgress(pct),
      );
      onUploaded?.(meta);
    } catch (err) {
      setError(err?.message || 'Не удалось загрузить файл.');
    } finally {
      setUploading(false);
    }
  };

  const handleInputChange = (e) => {
    const file = e.target.files?.[0];
    if (file) handlePick(file);
    // Allow re-picking the same file later.
    e.target.value = '';
  };

  const handleDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    if (disabled || uploading || parsing) return;
    const file = e.dataTransfer.files?.[0];
    if (file) handlePick(file);
  };

  const handleDelete = async () => {
    if (!currentFile?.id || uploading || parsing) return;
    setError('');
    if (localOnly) {
      // No backend call — just drop the reference.
      onDeleted?.();
      return;
    }
    try {
      await adapter.deleteFile(submissionId, currentFile.id);
      onDeleted?.();
    } catch (err) {
      setError(err?.message || 'Не удалось удалить файл.');
    }
  };

  const handleParse = async () => {
    if (!currentFile?.id || !onAutoFill || !parseType) return;
    setError('');
    setParsing(true);
    try {
      const fields = await adapter.parsePassport(submissionId, currentFile.id, parseType);
      onAutoFill(fields);
    } catch (err) {
      setError(err?.message || 'Не удалось распознать документ.');
    } finally {
      setParsing(false);
    }
  };

  const showEmpty = !currentFile && !uploading;

  // Recognition is only meaningful when the file lives on the server.
  // In upload-on-submit mode (public), hide the parse affordance so we
  // don't dangle a button that can't fire.
  const canParse = !localOnly && onAutoFill && parseType;

  return (
    <div className="ff-wrap" data-field={`file_${fileType}`}>
      <span className="ff-label">{label}</span>

      <input
        ref={inputRef}
        type="file"
        accept={acceptMime || undefined}
        onChange={handleInputChange}
        style={{ display: 'none' }}
      />

      {showEmpty && (
        <div
          className={`ff-drop${dragging ? ' is-dragging' : ''}${disabled ? ' is-disabled' : ''}`}
          onClick={pickFile}
          onDragOver={(e) => { e.preventDefault(); if (!disabled) setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={handleDrop}
          role="button"
          tabIndex={disabled ? -1 : 0}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pickFile(); } }}
        >
          <div className="ff-drop-title">Перетащите файл сюда</div>
          <div className="ff-drop-sub">или нажмите, чтобы выбрать</div>
          <div className="ff-drop-hint">PDF, JPEG, PNG · до 50 МБ</div>
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

      {currentFile && !uploading && (
        <div className="ff-file">
          <div className="ff-file-info">
            <div className="ff-file-name" title={currentFile.original_name}>
              {currentFile.original_name || 'файл'}
            </div>
            <div className="ff-file-meta">
              {formatSize(currentFile.size_bytes)}
              {currentFile.mime_type ? ` · ${currentFile.mime_type}` : ''}
            </div>
          </div>
          <div className="ff-file-actions">
            {canParse && (
              <button
                type="button"
                className="ff-btn ff-btn-accent btn-magic"
                onClick={handleParse}
                disabled={parsing}
                title="Распознать сканы и заполнить данные паспорта"
              >
                {parsing ? <span className="spinner" /> : '✦ Распознать'}
              </button>
            )}
            {!localOnly && adapter?.downloadUrl && currentFile.id && (
              <a
                className="ff-btn ff-icon-btn"
                href={adapter.downloadUrl(submissionId, currentFile.id)}
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
              onClick={pickFile}
              disabled={parsing}
              title="Заменить файл"
              aria-label="Заменить"
            >
              {/* Two opposite-direction arrows — clearer "swap / replace"
                  metaphor than the curved refresh arrows we had before
                  (those read as "обновить / refresh"). */}
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                <path d="M3 5h9M9.5 2.5 12 5l-2.5 2.5M13 11H4M6.5 13.5 4 11l2.5-2.5"
                  stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </button>
            {/* In upload-on-submit mode the tourist always gets a Delete
                button — there's no server row to "preserve" by hiding it. */}
            {(showDelete || localOnly) && (
              <button
                type="button"
                className="ff-btn ff-icon-btn"
                onClick={handleDelete}
                disabled={parsing}
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
