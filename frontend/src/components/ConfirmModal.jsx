import Modal from './Modal';

/**
 * ConfirmModal — custom confirm/alert dialog (replaces window.confirm/alert).
 *
 * Props:
 *   open          — boolean, show/hide
 *   title         — string, header
 *   message       — string | node, body text
 *   confirmText   — primary button label (default "ОК")
 *   cancelText    — secondary button label (default "Отмена"); pass null to
 *                    hide the cancel button and render an alert-style modal
 *   variant       — "primary" | "danger" (affects the confirm button color)
 *   busy          — boolean, disables buttons and shows spinner on confirm
 *   error         — string | null, optional error banner under the message
 *   onConfirm     — clicked the primary button
 *   onCancel      — clicked cancel, backdrop, or pressed Escape
 */
export default function ConfirmModal({
  open,
  title,
  message,
  confirmText = 'ОК',
  cancelText = 'Отмена',
  variant = 'primary',
  busy = false,
  error = null,
  onConfirm,
  onCancel,
  width = 440,
}) {
  const confirmClass = variant === 'danger' ? 'btn btn-danger' : 'btn btn-primary';
  return (
    <Modal open={open} onClose={busy ? () => {} : onCancel} title={title} width={width}>
      <div style={{ fontSize: 13, color: 'var(--white)', lineHeight: 1.55, whiteSpace: 'pre-wrap' }}>
        {message}
      </div>

      {error && (
        <div
          style={{
            marginTop: 12,
            padding: '8px 10px',
            borderRadius: 6,
            background: 'rgba(255, 107, 107, 0.08)',
            border: '1px solid rgba(255, 107, 107, 0.35)',
            color: 'var(--danger, #ff6b6b)',
            fontSize: 12,
          }}
        >
          {error}
        </div>
      )}

      <div
        style={{
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 8,
          marginTop: 20,
        }}
      >
        {cancelText !== null && (
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onCancel}
            disabled={busy}
          >
            {cancelText}
          </button>
        )}
        <button
          type="button"
          className={confirmClass}
          onClick={onConfirm}
          disabled={busy}
        >
          {busy ? <><span className="spinner" /> {confirmText}</> : confirmText}
        </button>
      </div>
    </Modal>
  );
}
