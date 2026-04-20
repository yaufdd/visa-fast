import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import SubmissionForm from '../components/SubmissionForm'
import { publicGetOrg, publicCreateSubmission } from '../api/client'

export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  const handleSubmit = async (payload, consent) => {
    await publicCreateSubmission(slug, payload, consent)
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
      <SubmissionForm onSubmit={handleSubmit} />
    </main>
  )
}
