import { useState, useEffect, useRef } from 'react';
import StatusBadge, { GROUP_STATUSES } from './StatusBadge';
import { updateGroupStatus, updateGroupNotes } from '../api/client';
import Celebration from './Celebration';

function StatusPill({ cfg, active, onClick, disabled }) {
  const bg = active ? `${cfg.color}26` : 'transparent';
  const border = active ? cfg.color : `${cfg.color}55`;
  const color = active ? cfg.color : `${cfg.color}cc`;
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '6px 14px',
        borderRadius: 100,
        fontSize: 12,
        fontWeight: 600,
        letterSpacing: '0.02em',
        color,
        background: bg,
        border: `1px solid ${border}`,
        cursor: disabled ? 'default' : 'pointer',
        opacity: active ? 1 : 0.7,
        transition: 'opacity 0.15s, background 0.15s, border-color 0.15s',
        fontFamily: 'inherit',
      }}
      onMouseEnter={e => { if (!active && !disabled) e.currentTarget.style.opacity = '1'; }}
      onMouseLeave={e => { if (!active && !disabled) e.currentTarget.style.opacity = '0.7'; }}
    >
      <span style={{
        width: 7,
        height: 7,
        borderRadius: '50%',
        background: cfg.color,
        flexShrink: 0,
      }} />
      {cfg.label}
    </button>
  );
}

export default function StatusSection({ group, onGroupUpdated }) {
  const currentStatus = group?.status || 'draft';
  const [editing, setEditing] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState(null);

  const [notes, setNotes] = useState(group?.notes || '');
  const [savingNotes, setSavingNotes] = useState(false);
  const [notesError, setNotesError] = useState(null);

  // Celebration trigger — fires only when the user presses "Готово" and the
  // final confirmed status is visa_issued AND it was changed in this edit session.
  const [celebrate, setCelebrate] = useState(0);
  const editStartStatusRef = useRef(currentStatus);

  useEffect(() => {
    setNotes(group?.notes || '');
  }, [group?.notes]);

  const handleStatusClick = async (nextStatus) => {
    if (nextStatus === currentStatus || updating) return;
    setError(null);
    const prev = group;
    // Optimistic
    onGroupUpdated?.({ ...group, status: nextStatus });
    setUpdating(true);
    try {
      const updated = await updateGroupStatus(group.id, nextStatus);
      onGroupUpdated?.(updated);
    } catch (e) {
      setError(e.message);
      onGroupUpdated?.(prev); // revert
    } finally {
      setUpdating(false);
    }
  };

  const notesDirty = (notes || '') !== (group?.notes || '');

  const handleDone = async () => {
    setError(null);
    setNotesError(null);
    if (notesDirty) {
      setSavingNotes(true);
      try {
        const updated = await updateGroupNotes(group.id, notes);
        onGroupUpdated?.(updated);
      } catch (e) {
        setNotesError(e.message);
        setSavingNotes(false);
        return; // stay in edit mode so user can retry
      }
      setSavingNotes(false);
    }
    // 🎉 Celebrate only if the user transitioned TO visa_issued in this session.
    if (
      currentStatus === 'visa_issued' &&
      editStartStatusRef.current !== 'visa_issued'
    ) {
      setCelebrate(c => c + 1);
    }
    setEditing(false);
  };

  // ── Read-only view ─────────────────────────────────────────────
  if (!editing) {
    return (
      <>
      <Celebration trigger={celebrate} />
      <div
        className="card"
        style={{
          marginBottom: 20,
          padding: '18px 20px',
          background: 'var(--graphite)',
          border: '1px solid var(--border)',
          borderRadius: 8,
        }}
      >
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 12,
          marginBottom: 14,
        }}>
          <div style={{
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            color: 'var(--white-dim)',
          }}>
            Статус подачи
          </div>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => {
              editStartStatusRef.current = currentStatus;
              setEditing(true);
            }}
          >
            Поменять статус
          </button>
        </div>

        <div style={{ marginBottom: group?.notes ? 16 : 0 }}>
          <StatusBadge status={currentStatus} />
        </div>

        {group?.notes && (
          <div>
            <div style={{
              fontSize: 11,
              fontWeight: 600,
              letterSpacing: '0.08em',
              textTransform: 'uppercase',
              color: 'var(--white-dim)',
              marginBottom: 6,
            }}>
              Чего не хватает
            </div>
            <div style={{
              fontSize: 13,
              color: 'var(--white)',
              whiteSpace: 'pre-wrap',
              lineHeight: 1.5,
            }}>
              {group.notes}
            </div>
          </div>
        )}
      </div>
      </>
    );
  }

  // ── Edit form ──────────────────────────────────────────────────
  return (
    <>
    <Celebration trigger={celebrate} />
    <div
      className="card"
      style={{
        marginBottom: 20,
        padding: '18px 20px',
        background: 'var(--graphite)',
        border: '1px solid var(--border)',
        borderRadius: 8,
      }}
    >
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 12,
        marginBottom: 12,
      }}>
        <div style={{
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--white-dim)',
        }}>
          Изменить статус подачи
        </div>
        <button
          type="button"
          className="btn btn-primary btn-sm"
          onClick={handleDone}
          disabled={savingNotes}
        >
          {savingNotes ? <><span className="spinner" /> Сохранение...</> : 'Готово'}
        </button>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 6 }}>
        {GROUP_STATUSES.map(cfg => (
          <StatusPill
            key={cfg.id}
            cfg={cfg}
            active={currentStatus === cfg.id}
            onClick={() => handleStatusClick(cfg.id)}
            disabled={updating}
          />
        ))}
      </div>
      {error && (
        <div style={{ fontSize: 11, color: 'var(--danger, #ef4444)', marginTop: 6 }}>{error}</div>
      )}

      <div style={{ marginTop: 18 }}>
        <label style={{
          display: 'block',
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          color: 'var(--white-dim)',
          marginBottom: 8,
        }}>
          Чего не хватает
        </label>
        <textarea
          className="form-input"
          rows={3}
          value={notes}
          onChange={e => setNotes(e.target.value)}
          placeholder="Заметки о том, каких документов или данных не хватает..."
          style={{
            width: '100%',
            resize: 'vertical',
            fontFamily: 'inherit',
            fontSize: 13,
            color: 'var(--white)',
          }}
        />
        {notesError && (
          <div style={{ fontSize: 12, color: 'var(--danger, #ef4444)', marginTop: 6 }}>{notesError}</div>
        )}
      </div>
    </div>
    </>
  );
}
