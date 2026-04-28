import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import FormWizard from '../components/forms/FormWizard';
import { adminWizardAdapter } from '../components/forms/adminWizardAdapter';
import SubmissionFilesPanel from '../components/SubmissionFilesPanel';
import AttachGroupModal from '../components/AttachGroupModal';
import ConfirmModal from '../components/ConfirmModal';
import {
  archiveSubmission,
  attachSubmission,
  getSubmission,
  listSubmissionFilesAdmin,
} from '../api/client';

function safeParsePayload(raw) {
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

const STATUS_LABELS = {
  draft: 'Черновик',
  pending: 'Ожидает',
  attached: 'Привязана',
  archived: 'Архив',
};

// Bucket the flat array returned by GET /submissions/{id}/files into the
// shape the wizard keeps in state. Passports are single objects (one per
// type, replace-on-upload). Tickets and vouchers are arrays — multiple
// files per submission are allowed since migration 000023.
function bucketFilesByType(arr) {
  const out = {
    passport_internal: null,
    passport_foreign: null,
    ticket: [],
    voucher: [],
  };
  if (!Array.isArray(arr)) return out;
  for (const f of arr) {
    if (!f || !f.file_type) continue;
    if (f.file_type === 'passport_internal' || f.file_type === 'passport_foreign') {
      out[f.file_type] = f;
    } else if (f.file_type === 'ticket' || f.file_type === 'voucher') {
      out[f.file_type].push(f);
    }
  }
  // Stable order: oldest first so newly added files appear at the bottom
  // of the wizard's list (matches upload order from the tourist).
  for (const k of ['ticket', 'voucher']) {
    out[k].sort((a, b) => {
      const av = a.created_at || '';
      const bv = b.created_at || '';
      if (av < bv) return -1;
      if (av > bv) return 1;
      return 0;
    });
  }
  return out;
}

export default function SubmissionDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const isNew = id === 'new';

  // Adapter is identity-stable — built once per page mount so the
  // wizard's effects don't re-run on every parent re-render.
  const adapter = useMemo(() => adminWizardAdapter(), []);

  const [submission, setSubmission] = useState(null);
  // For the "create new" branch we have to allocate a draft on mount and
  // then redirect /submissions/new to /submissions/<draftId> so a refresh
  // re-loads the same row instead of orphaning it.
  const [draftAllocating, setDraftAllocating] = useState(isNew);
  const [draftAllocErr, setDraftAllocErr] = useState(null);
  const [loading, setLoading] = useState(!isNew);
  const [error, setError] = useState(null);
  const [actionError, setActionError] = useState(null);
  const [initialFiles, setInitialFiles] = useState(null);

  const [attachOpen, setAttachOpen] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);

  // /submissions/new — allocate a draft and redirect immediately so the
  // URL becomes a stable handle (refresh-safe, shareable inside the org).
  useEffect(() => {
    if (!isNew) return undefined;
    let alive = true;
    setDraftAllocating(true);
    adapter.startSubmission()
      .then(({ submissionId }) => {
        if (!alive) return;
        navigate(`/submissions/${submissionId}`, { replace: true });
      })
      .catch((e) => {
        if (!alive) return;
        setDraftAllocErr(e?.message || 'Не удалось создать анкету.');
        setDraftAllocating(false);
      });
    return () => { alive = false; };
  }, [isNew, navigate, adapter]);

  // Existing submission — load row + attached files.
  useEffect(() => {
    if (isNew) return undefined;
    let alive = true;
    setLoading(true);
    setError(null);
    Promise.all([
      getSubmission(id),
      listSubmissionFilesAdmin(id).catch((e) => {
        // If the submission itself doesn't exist the parent fetch will
        // surface 404; tolerate file-list errors so the wizard still
        // mounts (worst case files seed empty and the manager can re-
        // upload).
        if (e && e.notFound) return [];
        return [];
      }),
    ])
      .then(([s, fileList]) => {
        if (!alive) return;
        setSubmission(s);
        setInitialFiles(bucketFilesByType(fileList));
      })
      .catch((e) => { if (alive) setError(e.message); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [id, isNew]);

  const initialPayload = useMemo(
    () => safeParsePayload(submission?.payload),
    [submission],
  );

  // True when the row was created via the manager-side draft endpoint
  // and hasn't been finalised yet. In that case we still want to show
  // the consent block on the Review step (treat the wizard as a "create"
  // experience, not an "edit"). For pending/attached/archived rows the
  // consent stamp is already on file — hide the block.
  const isDraftRow = submission?.status === 'draft';
  const showConsent = isDraftRow;

  const handleSubmit = useCallback(async (payload, consent) => {
    setActionError(null);
    await adapter.submit(id, payload, consent);
    // Once the row exists the workflow expects the manager back at the
    // submission list — that's where Attach / Archive happen.
    navigate('/submissions');
  }, [id, navigate, adapter]);

  const requestArchive = useCallback(() => {
    if (!submission) return;
    setActionError(null);
    setArchiveConfirmOpen(true);
  }, [submission]);

  const handleArchive = useCallback(async () => {
    if (!submission) return;
    setArchiving(true);
    setActionError(null);
    try {
      await archiveSubmission(submission.id);
      navigate('/submissions');
    } catch (e) {
      setActionError(e.message);
      setArchiving(false);
    }
  }, [submission, navigate]);

  const handleAttach = useCallback(async (groupId, subgroupId) => {
    if (!submission) return;
    await attachSubmission(submission.id, groupId, subgroupId);
    setAttachOpen(false);
    navigate(`/groups/${groupId}`);
  }, [submission, navigate]);

  // /submissions/new — render a small loading shell until the redirect
  // settles, or surface the allocation error.
  if (isNew) {
    if (draftAllocErr) {
      return (
        <div className="page-container">
          <div className="error-message">{draftAllocErr}</div>
          <button className="btn btn-ghost" onClick={() => navigate('/submissions')}>
            ← К списку анкет
          </button>
        </div>
      );
    }
    return (
      <div className="page-container">
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          {draftAllocating ? ' Создаём черновик…' : ' Загрузка…'}
        </div>
      </div>
    );
  }

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
  // canAct — the top-bar Archive / Attach buttons. A draft cannot be
  // archived or attached yet; an attached row can only be viewed.
  const canAct = submission && status !== 'attached' && status !== 'draft';
  const title = (
    initialPayload.name_lat || initialPayload.name_cyr || submission?.id || 'Анкета'
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
              {(submission?.id || '').slice(0, 8)}
            </span>
          </div>
          <div className="page-title">{title}</div>
          {submission && (
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
              onClick={requestArchive}
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

      <FormWizard
        adapter={adapter}
        onSubmit={handleSubmit}
        initialPayload={initialPayload}
        initialFiles={initialFiles}
        submissionId={id}
        showConsent={showConsent}
      />

      {/* SubmissionFilesPanel duplicates the per-step file widgets the
          wizard now renders inline, but it remains a useful "single
          consolidated" view: managers can see all four file types in
          one place plus get download URLs the wizard doesn't expose.
          Hidden on draft rows where the wizard's own widgets are the
          only place files have ever existed. */}
      {!isDraftRow && <SubmissionFilesPanel submissionId={id} />}

      <AttachGroupModal
        open={attachOpen}
        onClose={() => setAttachOpen(false)}
        onConfirm={handleAttach}
      />

      <ConfirmModal
        open={archiveConfirmOpen}
        title="Архивировать анкету?"
        message="Анкета уйдёт в архив. Её можно будет восстановить."
        confirmText="Архивировать"
        cancelText="Отмена"
        variant="primary"
        busy={archiving}
        error={actionError}
        onConfirm={handleArchive}
        onCancel={() => setArchiveConfirmOpen(false)}
      />
    </div>
  );
}
