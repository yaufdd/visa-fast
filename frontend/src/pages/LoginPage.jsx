import { useState } from 'react'
import { useLocation, useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const location = useLocation()
  const from = location.state?.from?.pathname || '/groups'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    try {
      await login(email.trim(), password)
      nav(from, { replace: true })
    } catch (e) {
      setErr(e.message || 'Неверный email или пароль')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-page">
      <h1>Вход в систему</h1>
      <form onSubmit={submit} className="auth-form">
        <label className="form-group">
          <span className="form-label">Email</span>
          <input className="form-input" type="email" value={email}
                 onChange={(e) => setEmail(e.target.value)}
                 required autoFocus autoComplete="email" />
        </label>
        <label className="form-group">
          <span className="form-label">Пароль</span>
          <input className="form-input" type="password" value={password}
                 onChange={(e) => setPassword(e.target.value)}
                 required autoComplete="current-password" />
        </label>
        {err && <div className="auth-error">{err}</div>}
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {busy ? 'Проверка…' : 'Войти'}
        </button>
      </form>
      <p className="auth-link">Нет аккаунта? <Link to="/register">Зарегистрировать турфирму</Link></p>
      <p className="auth-hint">
        Забыли пароль? Свяжитесь с администратором: <a href="mailto:tour@fujitravel.ru">tour@fujitravel.ru</a>
      </p>
    </main>
  )
}
