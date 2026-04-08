import { useState, useRef } from 'react';

export default function FileUpload({ onUpload, accept = '*', label = 'Upload file', multiple = false }) {
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef(null);

  const handleDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) {
      multiple ? onUpload(files) : onUpload(files[0]);
    }
  };

  const handleChange = (e) => {
    const files = Array.from(e.target.files);
    if (files.length > 0) {
      multiple ? onUpload(files) : onUpload(files[0]);
    }
    e.target.value = '';
  };

  return (
    <div
      className={`file-drop-zone${dragging ? ' dragging' : ''}`}
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={() => setDragging(false)}
      onDrop={handleDrop}
      onClick={() => inputRef.current?.click()}
    >
      <input
        ref={inputRef}
        type="file"
        accept={accept}
        multiple={multiple}
        onChange={handleChange}
        style={{ display: 'none' }}
      />
      <div className="file-drop-icon">⬆</div>
      <div className="file-drop-label">{label}</div>
      <div className="file-drop-hint">drag & drop or click to browse</div>

      <style>{`
        .file-drop-zone {
          border: 2px dashed var(--border);
          border-radius: 10px;
          padding: 28px 20px;
          text-align: center;
          cursor: pointer;
          transition: all 0.15s ease;
          background: var(--graphite);
        }

        .file-drop-zone:hover,
        .file-drop-zone.dragging {
          border-color: var(--accent);
          background: var(--accent-dim);
        }

        .file-drop-icon {
          font-size: 24px;
          margin-bottom: 8px;
          opacity: 0.6;
        }

        .file-drop-label {
          font-size: 13px;
          font-weight: 500;
          color: var(--white);
          margin-bottom: 4px;
        }

        .file-drop-hint {
          font-size: 12px;
          color: var(--white-dim);
        }
      `}</style>
    </div>
  );
}
