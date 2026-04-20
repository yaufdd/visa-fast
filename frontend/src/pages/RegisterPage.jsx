import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [err, setErr] = useState(null)
  const [busy, setBusy] = useState(false)

  const valid =
    orgName.trim().length > 0 &&
    /\S+@\S+\.\S+/.test(email) &&
    password.length >= 8 &&
    password === confirm

  const submit = async (e) => {
    e.preventDefault()
    if (!valid) return
    setErr(null)
    setBusy(true)
    try {
      await register(orgName.trim(), email.trim(), password)
      nav('/groups', { replace: true })
    } catch (e) {
      setErr(e.message || 'Ошибка регистрации')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-page">
      <h1>Регистрация турфирмы</h1>
      <form onSubmit={submit} className="auth-form">
        <label className="form-group">
          <span className="form-label">Название турфирмы</span>
          <input className="form-input" value={orgName}
                 onChange={(e) => setOrgName(e.target.value)}
                 required autoFocus />
        </label>
        <label className="form-group">
          <span className="form-label">Email</span>
          <input className="form-input" type="email" value={email}
                 onChange={(e) => setEmail(e.target.value)}
                 required autoComplete="email" />
        </label>
        <label className="form-group">
          <span className="form-label">Пароль (минимум 8 символов)</span>
          <input className="form-input" type="password" value={password}
                 onChange={(e) => setPassword(e.target.value)}
                 required minLength={8} autoComplete="new-password" />
        </label>
        <label className="form-group">
          <span className="form-label">Подтверждение пароля</span>
          <input className="form-input" type="password" value={confirm}
                 onChange={(e) => setConfirm(e.target.value)}
                 required minLength={8} autoComplete="new-password" />
          {confirm && confirm !== password && <span className="auth-error">Пароли не совпадают</span>}
        </label>
        {err && <div className="auth-error">{err}</div>}
        <button type="submit" className="btn btn-primary" disabled={!valid || busy}>
          {busy ? 'Создаём аккаунт…' : 'Зарегистрировать'}
        </button>
      </form>
      <p className="auth-link">Уже есть аккаунт? <Link to="/login">Войти</Link></p>
    </main>
  )
}
