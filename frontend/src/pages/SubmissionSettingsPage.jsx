import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import SubmissionFilesPanel from '../components/SubmissionFilesPanel';
import ConfirmModal from '../components/ConfirmModal';
import { eraseSubmission, getSubmission } from '../api/client';

const STATUS_LABELS = {
  draft: 'Черновик',
  pending: 'Ожидает',
  attached: 'Привязана',
  archived: 'Архив',
};

// SubmissionSettingsPage — destructive / housekeeping actions for a single
// submission, peeled off the main editor page so the wizard there stays
// uncluttered. Reachable from a "⚙ Настройки" button under the wizard.
export default function SubmissionSettingsPage() {
  const { id } = useParams();
  const navigate = useNavigate();

  const [submission, setSubmission] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [actionError, setActionError] = useState(null);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError(null);
    getSubmission(id)
      .then((data) => { if (alive) setSubmission(data); })
      .catch((e) => { if (alive) setError(e.message); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [id]);

  const handleDelete = useCallback(async () => {
    if (!submission) return;
    setDeleting(true);
    setActionError(null);
    try {
      await eraseSubmission(submission.id);
      navigate('/submissions');
    } catch (e) {
      setActionError(e.message);
      setDeleting(false);
    }
  }, [submission, navigate]);

  const status = submission?.status || 'pending';

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
            <button
              onClick={() => navigate(`/submissions/${id}`)}
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 13,
                cursor: 'pointer',
                padding: 0,
              }}
            >
              ← К анкете
            </button>
          </div>
          <div className="page-title">Настройки анкеты</div>
          {submission && (
            <div
              className="page-subtitle"
              style={{ display: 'flex', gap: 10, alignItems: 'center', marginTop: 6 }}
            >
              <span className={`submission-status submission-status--${status}`}>
                {STATUS_LABELS[status] || status}
              </span>
              <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
                Источник: {submission.source}
              </span>
            </div>
          )}
        </div>
      </div>

      {loading && (
        <div className="loading-center">
          <div className="spinner spinner-lg" /> Загрузка...
        </div>
      )}

      {error && <div className="error-message">{error}</div>}

      {!loading && !error && submission && (
        <>
          <div
            style={{
              padding: 20,
              border: '1px solid var(--border)',
              borderRadius: 10,
              background: 'var(--gray-dark)',
              marginBottom: 24,
            }}
          >
            <SubmissionFilesPanel submissionId={id} allowDelete />
          </div>

          <div
            style={{
              padding: 20,
              border: '1px solid rgba(239, 68, 68, 0.25)',
              borderRadius: 10,
              background: 'var(--gray-dark)',
              display: 'flex',
              flexDirection: 'column',
              gap: 10,
            }}
          >
            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--white)' }}>
              Опасная зона
            </div>
            <div style={{ fontSize: 12, color: 'var(--white-dim)' }}>
              Удаление анкеты — необратимое действие. Все прикреплённые файлы и
              введённые данные будут стёрты.
            </div>
            <div style={{ marginTop: 6 }}>
              <button
                type="button"
                onClick={() => { setActionError(null); setDeleteConfirmOpen(true); }}
                disabled={deleting}
                style={{
                  background: 'none',
                  border: '1px solid rgba(239, 68, 68, 0.4)',
                  color: '#ef4444',
                  fontSize: 13,
                  fontWeight: 600,
                  padding: '8px 16px',
                  borderRadius: 6,
                  cursor: deleting ? 'default' : 'pointer',
                  opacity: deleting ? 0.6 : 1,
                  transition: 'background 0.15s, border-color 0.15s',
                }}
                onMouseEnter={e => { if (deleting) return; e.currentTarget.style.background = 'rgba(239, 68, 68, 0.08)'; e.currentTarget.style.borderColor = '#ef4444'; }}
                onMouseLeave={e => { if (deleting) return; e.currentTarget.style.background = 'none'; e.currentTarget.style.borderColor = 'rgba(239, 68, 68, 0.4)'; }}
              >
                {deleting ? 'Удаление…' : 'Удалить анкету'}
              </button>
            </div>
          </div>
        </>
      )}

      <ConfirmModal
        open={deleteConfirmOpen}
        title="Удалить анкету?"
        message="Анкета будет удалена навсегда. Это действие нельзя отменить."
        confirmText="Удалить"
        cancelText="Отмена"
        variant="danger"
        busy={deleting}
        error={actionError}
        onConfirm={handleDelete}
        onCancel={() => { if (!deleting) setDeleteConfirmOpen(false); }}
      />
    </div>
  );
}
