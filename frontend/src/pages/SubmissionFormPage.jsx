import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import FormWizard from '../components/forms/FormWizard'
import { publicGetOrg, publicCreateSubmission } from '../api/client'
import { startSubmission } from '../api/files'

// Public form page. Resolves the slug to an org name, then opens a draft
// submission so the tourist can attach scans before they finish typing.
// If /start fails (e.g. backend offline / Yandex misconfigured at boot)
// we degrade to the legacy "no uploads" mode — the form still submits.
export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)
  const [submissionId, setSubmissionId] = useState(null)
  const [draftErr, setDraftErr] = useState(false)

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  // Start a draft only after we've confirmed the slug resolves. We keep
  // the failure soft — the legacy submit path doesn't need the draft id.
  useEffect(() => {
    if (!orgName) return
    let cancelled = false
    startSubmission(slug)
      .then((data) => { if (!cancelled) setSubmissionId(data.submission_id) })
      .catch(() => { if (!cancelled) setDraftErr(true) })
    return () => { cancelled = true }
  }, [orgName, slug])

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
      <FormWizard
        onSubmit={handleSubmit}
        slug={slug}
        submissionId={submissionId}
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
