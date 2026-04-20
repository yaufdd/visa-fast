import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import SubmissionForm from '../components/SubmissionForm';
import AttachGroupModal from '../components/AttachGroupModal';
import {
  archiveSubmission,
  attachSubmission,
  createSubmission,
  getSubmission,
  updateSubmission,
} from '../api/client';

function safeParsePayload(raw) {
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

const STATUS_LABELS = {
  pending: 'Ожидает',
  attached: 'Привязана',
  archived: 'Архив',
};

export default function SubmissionDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const isNew = id === 'new';

  const [submission, setSubmission] = useState(null);
  const [loading, setLoading] = useState(!isNew);
  const [error, setError] = useState(null);
  const [actionError, setActionError] = useState(null);
  const [notice, setNotice] = useState(null);

  const [attachOpen, setAttachOpen] = useState(false);
  const [archiving, setArchiving] = useState(false);

  useEffect(() => {
    if (isNew) {
      setSubmission(null);
      setLoading(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError(null);
    getSubmission(id)
      .then((s) => { if (alive) setSubmission(s); })
      .catch((e) => { if (alive) setError(e.message); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [id, isNew]);

  const initialPayload = useMemo(
    () => safeParsePayload(submission?.payload),
    [submission],
  );

  const handleSubmit = useCallback(async (payload, consent) => {
    setActionError(null);
    if (isNew) {
      const res = await createSubmission(payload, consent, 'manager');
      navigate(`/submissions/${res.id}`);
      return;
    }
    await updateSubmission(id, payload);
    // Re-fetch to refresh timestamps / state.
    const fresh = await getSubmission(id);
    setSubmission(fresh);
    setNotice('Сохранено');
    setTimeout(() => setNotice(null), 2000);
  }, [isNew, id, navigate]);

  const handleArchive = useCallback(async () => {
    if (!submission) return;
    if (!confirm('Архивировать анкету? Её можно будет восстановить.')) return;
    setArchiving(true);
    setActionError(null);
    try {
      await archiveSubmission(submission.id);
      navigate('/submissions');
    } catch (e) {
      setActionError(e.message);
    } finally {
      setArchiving(false);
    }
  }, [submission, navigate]);

  const handleAttach = useCallback(async (groupId, subgroupId) => {
    if (!submission) return;
    await attachSubmission(submission.id, groupId, subgroupId);
    setAttachOpen(false);
    navigate(`/groups/${groupId}`);
  }, [submission, navigate]);

  if (loading) {
    return (
      <div className="page-container">
        <div className="loading-center">
          <div className="spinner spinner-lg" /> Загрузка...
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-container">
        <div className="error-message">{error}</div>
        <button className="btn btn-ghost" onClick={() => navigate('/submissions')}>
          ← К списку анкет
        </button>
      </div>
    );
  }

  const status = submission?.status || 'pending';
  const canAct = !isNew && submission && status !== 'attached';
  const title = isNew ? 'Новая анкета' : (
    initialPayload.name_lat || initialPayload.name_cyr || submission?.id
  );

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div
            style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}
          >
            <button
              onClick={() => navigate('/submissions')}
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 13,
                cursor: 'pointer',
                padding: 0,
              }}
            >
              ← Анкеты
            </button>
            <span style={{ color: 'var(--border)' }}>/</span>
            <span style={{ color: 'var(--white-dim)', fontSize: 13 }}>
              {isNew ? 'Новая' : (submission?.id || '').slice(0, 8)}
            </span>
          </div>
          <div className="page-title">{title}</div>
          {!isNew && submission && (
            <div
              className="page-subtitle"
              style={{ display: 'flex', gap: 10, alignItems: 'center', marginTop: 6 }}
            >
              <span
                className={`submission-status submission-status--${status}`}
              >
                {STATUS_LABELS[status] || status}
              </span>
              <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
                Источник: {submission.source}
              </span>
            </div>
          )}
        </div>

        {canAct && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button
              type="button"
              className="btn btn-ghost"
              onClick={handleArchive}
              disabled={archiving}
            >
              {archiving ? 'Архивация...' : 'Архивировать'}
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => setAttachOpen(true)}
            >
              Привязать к группе
            </button>
          </div>
        )}
      </div>

      {actionError && <div className="error-message">{actionError}</div>}
      {notice && <div className="success-message">{notice}</div>}

      <SubmissionForm
        onSubmit={handleSubmit}
        initialPayload={initialPayload}
        submitLabel={isNew ? 'Создать анкету' : 'Сохранить'}
        showConsent={isNew}
      />

      <AttachGroupModal
        open={attachOpen}
        onClose={() => setAttachOpen(false)}
        onConfirm={handleAttach}
      />
    </div>
  );
}
