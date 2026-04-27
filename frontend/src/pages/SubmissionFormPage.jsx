import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import FormWizard from '../components/forms/FormWizard'
import { publicGetOrg, publicCreateSubmission } from '../api/client'
import { startSubmission, listSubmissionFiles } from '../api/files'
import { clearWizardBlob } from '../components/forms/wizardPersistence'

// Public form page. Resolves the slug to an org name, then opens a draft
// submission so the tourist can attach scans before they finish typing.
// If /start fails (e.g. backend offline / Yandex misconfigured at boot)
// we degrade to the legacy "no uploads" mode — the form still submits.
//
// When the wizard hydrates a saved blob with a `submissionId`, we skip
// the `/start` call entirely so a reload doesn't orphan the previous
// draft (and its uploaded files). The wizard hands the restored id back
// via `onRestoreSubmissionId`.
export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)
  const [submissionId, setSubmissionId] = useState(null)
  const [draftErr, setDraftErr] = useState(false)
  const [staleDraftErr, setStaleDraftErr] = useState(false)
  // True once the wizard has reported "I restored a submissionId from
  // storage, don't call /start". We use this to gate the auto-/start
  // effect; the ref mirrors the same flag so the gating runs even before
  // React commits the state.
  const restoredIdRef = useRef(false)
  const [restoreSettled, setRestoreSettled] = useState(false)

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  // Start a draft only after:
  //   (a) the slug resolves to an org, AND
  //   (b) the wizard has had one tick to tell us whether it restored an
  //       existing submissionId from localStorage.
  // We keep the failure soft — the legacy submit path doesn't need the
  // draft id.
  useEffect(() => {
    if (!orgName) return undefined
    if (!restoreSettled) return undefined
    if (restoredIdRef.current) return undefined
    let cancelled = false
    startSubmission(slug)
      .then((data) => { if (!cancelled) setSubmissionId(data.submission_id) })
      .catch(() => { if (!cancelled) setDraftErr(true) })
    return () => { cancelled = true }
  }, [orgName, slug, restoreSettled])

  // The wizard restores synchronously, so we settle on the very first
  // commit. If it reports an id, we adopt it; if not, the effect above
  // fires the normal `/start`.
  useEffect(() => {
    if (orgName && !restoreSettled) {
      // Defer one microtask so the wizard's mount effect (which calls
      // onRestoreSubmissionId) can run first.
      const t = setTimeout(() => setRestoreSettled(true), 0)
      return () => clearTimeout(t)
    }
    return undefined
  }, [orgName, restoreSettled])

  // Validate the restored submissionId against the server. If it returns
  // 404 (server cleaned old drafts, or different env) we clear the blob
  // and fall back to a fresh /start.
  const handleRestoreSubmissionId = (id) => {
    if (!id) return
    restoredIdRef.current = true
    setSubmissionId(id)
    // Best-effort liveness check: list the files for that submission.
    // If the server returns 404 we know the draft is gone and we should
    // start over.
    listSubmissionFiles(slug, id).catch((err) => {
      const msg = String(err?.message || '')
      if (msg.includes('404') || /not found/i.test(msg)) {
        clearWizardBlob(slug)
        restoredIdRef.current = false
        setSubmissionId(null)
        setStaleDraftErr(true)
        // Re-run the /start effect by toggling restoreSettled off then on.
        setRestoreSettled(false)
        setTimeout(() => setRestoreSettled(true), 0)
      }
      // Any other error (network blip, 5xx) — we keep the id; the next
      // upload attempt will surface a clearer error if needed.
    })
  }

  // "Начать заново" pressed inside the wizard banner. Issue a brand new
  // /start so subsequent uploads go to a fresh draft row.
  const handleResetDraft = () => {
    restoredIdRef.current = false
    setSubmissionId(null)
    setDraftErr(false)
    setStaleDraftErr(false)
    startSubmission(slug)
      .then((data) => setSubmissionId(data.submission_id))
      .catch(() => setDraftErr(true))
  }

  const handleSubmit = async (payload, consent) => {
    await publicCreateSubmission(slug, payload, consent, submissionId || undefined)
    nav('/form/thanks', { replace: true })
  }

  if (loadErr) {
    return (
      <main className="public-form">
        <h1>Ошибка</h1>
        <p>{loadErr}</p>
      </main>
    )
  }
  if (!orgName) return <main className="public-form"><h1>Загрузка…</h1></main>

  return (
    <main className="public-form">
      <h1>Анкета на визу в Японию</h1>
      <p className="lead">Для турфирмы: <strong>{orgName}</strong></p>
      {draftErr && (
        <div className="public-form-banner">
          Загрузка файлов недоступна, можно отправить анкету без сканов.
        </div>
      )}
      {staleDraftErr && (
        <div className="public-form-banner">
          Сохранённый черновик устарел, начнём заново.
        </div>
      )}
      <FormWizard
        onSubmit={handleSubmit}
        slug={slug}
        submissionId={submissionId}
        onRestoreSubmissionId={handleRestoreSubmissionId}
        onResetDraft={handleResetDraft}
      />
      <style>{`
        .public-form-banner {
          max-width: 560px;
          margin: 0 auto 14px;
          padding: 10px 14px;
          background: var(--graphite);
          border: 1px solid var(--border);
          border-radius: 8px;
          color: var(--white-dim);
          font-size: 13px;
        }
      `}</style>
    </main>
  )
}
