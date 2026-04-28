import { useRef, useState } from 'react';

// Multi-file companion to FileUploadField: a list of already-uploaded files
// with a per-file "Удалить" button and a single drop zone at the bottom for
// adding more. Reuses the .ff-* styles from FileUploadField (mounted on the
// page once via that component's <style> block).

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

export default function FileMultiUploadField({
  label,
  fileType,
  adapter,
  submissionId,
  currentFiles = [],
  onAdded,
  onRemoved,
  acceptMime = '',
}) {
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [deletingId, setDeletingId] = useState(null);
  const [error, setError] = useState('');
  const inputRef = useRef(null);
  const [dragging, setDragging] = useState(false);

  const disabled = !submissionId;

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
    setUploading(true);
    setProgress(0);
    try {
      const meta = await adapter.uploadFile(
        submissionId,
        fileType,
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
    // Sequential upload — keeps the progress bar meaningful and prevents
    // a flood of parallel multipart requests for a tourist on mobile.
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

  const handleDelete = async (file) => {
    if (uploading || deletingId) return;
    setError('');
    setDeletingId(file.id);
    try {
      await adapter.deleteFile(submissionId, file.id);
      onRemoved?.(file.id);
    } catch (err) {
      setError(err?.message || 'Не удалось удалить файл.');
    } finally {
      setDeletingId(null);
    }
  };

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
                <button
                  type="button"
                  className="ff-btn ff-btn-danger"
                  onClick={() => handleDelete(f)}
                  disabled={uploading || deletingId === f.id}
                >
                  {deletingId === f.id ? 'Удаляю…' : 'Удалить'}
                </button>
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
          className={`ff-drop${dragging ? ' is-dragging' : ''}${disabled ? ' is-disabled' : ''}`}
          onClick={pickFile}
          onDragOver={(e) => { e.preventDefault(); if (!disabled) setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={handleDrop}
          role="button"
          tabIndex={disabled ? -1 : 0}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pickFile(); } }}
        >
          <div className="ff-drop-title">
            {currentFiles.length > 0 ? 'Добавить ещё файл' : 'Перетащите файл сюда'}
          </div>
          <div className="ff-drop-sub">или нажмите, чтобы выбрать</div>
          <div className="ff-drop-hint">PDF, JPEG, PNG · до 50 МБ · можно несколько</div>
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
