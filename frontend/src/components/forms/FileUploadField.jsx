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

// FileUploadField — single-file widget bound to one (submissionId, fileType)
// pair. Uses the public-form endpoints exposed by api/files.js.
//
// States:
//   empty      — drop zone visible, "Выбрать файл" button
//   uploading  — progress bar 0..100%
//   uploaded   — file name + size + Заменить / Удалить (and Распознать
//                if onAutoFill is set)
//   parsing    — spinner while /parse-passport is running
//   error      — red text under the widget; cleared on next action
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
}) {
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [parsing, setParsing] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef(null);
  const [dragging, setDragging] = useState(false);

  // Reset progress whenever the parent swaps the underlying file (e.g.
  // after deletion or replacement) so a stale 100% bar doesn't flash.
  useEffect(() => {
    setProgress(0);
  }, [currentFile?.id]);

  const disabled = !submissionId;

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

  const handleUpload = async (file) => {
    const localErr = validateFile(file);
    if (localErr) {
      setError(localErr);
      return;
    }
    setError('');
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
    if (file) handleUpload(file);
    // Allow re-picking the same file later.
    e.target.value = '';
  };

  const handleDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    if (disabled || uploading || parsing) return;
    const file = e.dataTransfer.files?.[0];
    if (file) handleUpload(file);
  };

  const handleDelete = async () => {
    if (!currentFile?.id || uploading || parsing) return;
    setError('');
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
            {onAutoFill && parseType && (
              <button
                type="button"
                className="ff-btn ff-btn-accent"
                onClick={handleParse}
                disabled={parsing}
              >
                {parsing ? 'Распознаю…' : 'Распознать'}
              </button>
            )}
            <button
              type="button"
              className="ff-btn"
              onClick={pickFile}
              disabled={parsing}
            >
              Заменить
            </button>
            <button
              type="button"
              className="ff-btn ff-btn-danger"
              onClick={handleDelete}
              disabled={parsing}
            >
              Удалить
            </button>
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
