import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import FormWizard from '../components/forms/FormWizard'
import { publicGetOrg } from '../api/client'
import { listSubmissionFiles } from '../api/files'
import { publicWizardAdapter } from '../components/forms/publicWizardAdapter'
import { clearWizardBlob } from '../components/forms/wizardPersistence'

// Public form page. Resolves the slug to an org name, then opens a draft
// submission so the tourist can attach scans before they finish typing.
// If draft allocation fails (e.g. backend offline / Yandex misconfigured
// at boot) we degrade to the legacy "no uploads" mode — the form still
// submits.
//
// When the wizard hydrates a saved blob with a `submissionId`, we skip
// the draft-allocation call entirely so a reload doesn't orphan the
// previous draft (and its uploaded files). The wizard hands the restored
// id back via `onRestoreSubmissionId`.
export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)
  const [submissionId, setSubmissionId] = useState(null)
  const [draftErr, setDraftErr] = useState(false)
  const [staleDraftErr, setStaleDraftErr] = useState(false)
  const restoredIdRef = useRef(false)
  const [restoreSettled, setRestoreSettled] = useState(false)

  // The adapter is identity-stable per-slug — building it inside useMemo
  // stops a fresh object from triggering re-renders downstream.
  const adapter = useMemo(() => publicWizardAdapter(slug), [slug])

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  useEffect(() => {
    if (!orgName) return undefined
    if (!restoreSettled) return undefined
    if (restoredIdRef.current) return undefined
    let cancelled = false
    adapter.startSubmission()
      .then((data) => { if (!cancelled) setSubmissionId(data.submissionId) })
      .catch(() => { if (!cancelled) setDraftErr(true) })
    return () => { cancelled = true }
  }, [orgName, slug, restoreSettled, adapter])

  useEffect(() => {
    if (orgName && !restoreSettled) {
      const t = setTimeout(() => setRestoreSettled(true), 0)
      return () => clearTimeout(t)
    }
    return undefined
  }, [orgName, restoreSettled])

  const handleRestoreSubmissionId = (id) => {
    if (!id) return
    restoredIdRef.current = true
    setSubmissionId(id)
    listSubmissionFiles(slug, id).catch((err) => {
      const msg = String(err?.message || '')
      if (msg.includes('404') || /not found/i.test(msg)) {
        clearWizardBlob(slug)
        restoredIdRef.current = false
        setSubmissionId(null)
        setStaleDraftErr(true)
        setRestoreSettled(false)
        setTimeout(() => setRestoreSettled(true), 0)
      }
    })
  }

  const handleResetDraft = () => {
    restoredIdRef.current = false
    setSubmissionId(null)
    setDraftErr(false)
    setStaleDraftErr(false)
    adapter.startSubmission()
      .then((data) => setSubmissionId(data.submissionId))
      .catch(() => setDraftErr(true))
  }

  const handleSubmit = async (payload, consent) => {
    await adapter.submit(submissionId, payload, consent)
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
        adapter={adapter}
        onSubmit={handleSubmit}
        persistKey={slug}
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
