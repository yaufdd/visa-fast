import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import FormWizard from '../components/forms/FormWizard'
import { publicGetOrg } from '../api/client'
import { publicWizardAdapter } from '../components/forms/publicWizardAdapter'
import { clearWizardBlob } from '../components/forms/wizardPersistence'

// Public form page. Resolves the slug to an org name and renders the
// wizard. The tourist picks files locally as they fill the form; on
// final submit the wizard hands the entire payload + files bundle to
// the adapter, which posts a single multipart/form-data request to
// /api/public/submissions/<slug>. There is no draft submission row,
// no per-file upload during typing, and no server interaction beyond
// the org-info GET until the tourist clicks Submit.
//
// Typed text persists in localStorage so an accidental F5 doesn't blow
// away progress; selected files do NOT persist (browsers can't
// serialise File objects). On reload the wizard's `files` state is
// empty and the tourist picks the scans again.
export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)

  // The adapter is identity-stable per-slug — building it inside useMemo
  // stops a fresh object from triggering re-renders downstream.
  const adapter = useMemo(() => publicWizardAdapter(slug), [slug])

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  const handleSubmit = async (payload, consent, files, captchaToken) => {
    await adapter.submit(payload, consent, files, captchaToken)
    // Submission accepted — drop the typed-text draft so a back-nav
    // doesn't restore it. The wizard already does this on success, but
    // we double-tap here as a safety net (no-op if already cleared).
    clearWizardBlob(slug)
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
      <FormWizard
        adapter={adapter}
        onSubmit={handleSubmit}
        persistKey={slug}
      />
    </main>
  )
}
