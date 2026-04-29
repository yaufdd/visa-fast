import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import FormWizard from '../components/forms/FormWizard';
import { adminWizardAdapter } from '../components/forms/adminWizardAdapter';
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

// Bucket the flat array returned by GET /submissions/{id}/files into the
// shape the wizard keeps in state. passport_internal joined the multi-row
// types in migration 000024 (manager often uploads main + registration
// page); passport_foreign stays single (one row per submission,
// replace-on-upload).
function bucketFilesByType(arr) {
  const out = {
    passport_internal: [],
    passport_foreign: null,
    ticket: [],
    voucher: [],
  };
  if (!Array.isArray(arr)) return out;
  for (const f of arr) {
    if (!f || !f.file_type) continue;
    if (f.file_type === 'passport_foreign') {
      out[f.file_type] = f;
    } else if (
      f.file_type === 'passport_internal'
      || f.file_type === 'ticket'
      || f.file_type === 'voucher'
    ) {
      out[f.file_type].push(f);
    }
  }
  // Stable order: oldest first so newly added files appear at the bottom
  // of the wizard's list (matches upload order from the tourist).
  for (const k of ['passport_internal', 'ticket', 'voucher']) {
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
  // and hasn't been finalised yet. Used below to hide the
  // SubmissionFilesPanel for fresh drafts where the wizard's own widgets
  // are the only place files exist.
  const isDraftRow = submission?.status === 'draft';
  // Manager doesn't need the consent checkbox at all — the tourist
  // grants consent on the public form, and managers acting on a
  // submission don't re-stamp it. Was previously gated on isDraftRow,
  // now always off in admin mode.
  const showConsent = false;

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

  return (
    <div className="page-container">
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
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
          {submission && (
            <button
              type="button"
              onClick={() => navigate(`/submissions/${id}/settings`)}
              title="Настройки анкеты"
              aria-label="Настройки"
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 13,
                cursor: 'pointer',
                padding: 0,
                display: 'inline-flex',
                alignItems: 'center',
              }}
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="14"
                height="14"
                fill="currentColor"
                viewBox="0 0 16 16"
                style={{ flexShrink: 0 }}
              >
                <path d="M9.405 1.05c-.413-1.4-2.397-1.4-2.81 0l-.1.34a1.464 1.464 0 0 1-2.105.872l-.31-.17c-1.283-.698-2.686.705-1.987 1.987l.169.311c.446.82.023 1.841-.872 2.105l-.34.1c-1.4.413-1.4 2.397 0 2.81l.34.1a1.464 1.464 0 0 1 .872 2.105l-.17.31c-.698 1.283.705 2.686 1.987 1.987l.311-.169a1.464 1.464 0 0 1 2.105.872l.1.34c.413 1.4 2.397 1.4 2.81 0l.1-.34a1.464 1.464 0 0 1 2.105-.872l.31.17c1.283.698 2.686-.705 1.987-1.987l-.169-.311a1.464 1.464 0 0 1 .872-2.105l.34-.1c1.4-.413 1.4-2.397 0-2.81l-.34-.1a1.464 1.464 0 0 1-.872-2.105l.17-.31c.698-1.283-.705-2.686-1.987-1.987l-.311.169a1.464 1.464 0 0 1-2.105-.872l-.1-.34zM8 10.93a2.929 2.929 0 1 1 0-5.858 2.929 2.929 0 0 1 0 5.858z"/>
              </svg>
            </button>
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
