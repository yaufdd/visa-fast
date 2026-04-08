import { useEffect, useRef } from 'react';

export default function Modal({ open, onClose, title, children, width = 480 }) {
  const backdropRef = useRef(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      ref={backdropRef}
      className="modal-backdrop"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div className="modal-panel" style={{ width, maxWidth: '95vw' }}>
        <div className="modal-header">
          <span className="modal-title">{title}</span>
          <button className="modal-close" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>

      <style>{`
        .modal-backdrop {
          position: fixed;
          inset: 0;
          background: rgba(0, 0, 0, 0.65);
          backdrop-filter: blur(4px);
          display: flex;
          align-items: center;
          justify-content: center;
          z-index: 1000;
          padding: 24px;
        }

        .modal-panel {
          background: var(--gray-dark);
          border: 1px solid var(--border);
          border-radius: 12px;
          display: flex;
          flex-direction: column;
          max-height: 85vh;
          overflow: hidden;
          box-shadow: 0 25px 60px rgba(0, 0, 0, 0.5);
        }

        .modal-header {
          display: flex;
          align-items: center;
          justify-content: space-between;
          padding: 18px 24px;
          border-bottom: 1px solid var(--border);
          flex-shrink: 0;
        }

        .modal-title {
          font-size: 15px;
          font-weight: 600;
          color: var(--white);
        }

        .modal-close {
          background: transparent;
          color: var(--white-dim);
          font-size: 13px;
          padding: 4px 8px;
          border-radius: 4px;
          transition: all 0.15s;
        }

        .modal-close:hover {
          background: var(--gray);
          color: var(--white);
        }

        .modal-body {
          padding: 24px;
          overflow-y: auto;
          flex: 1;
        }
      `}</style>
    </div>
  );
}
