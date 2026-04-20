import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { apiLogin, apiLogout, apiMe, apiRegister } from '../api/client'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [state, setState] = useState({ loading: true, user: null, org: null })

  const refresh = useCallback(async () => {
    try {
      const data = await apiMe()
      if (!data) {
        setState({ loading: false, user: null, org: null })
      } else {
        setState({ loading: false, user: data.user, org: data.org })
      }
    } catch {
      setState({ loading: false, user: null, org: null })
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const login = async (email, password) => {
    const data = await apiLogin(email, password)
    setState({ loading: false, user: data.user, org: data.org })
  }

  const register = async (orgName, email, password) => {
    const data = await apiRegister(orgName, email, password)
    setState({ loading: false, user: data.user, org: data.org })
  }

  const logout = async () => {
    await apiLogout()
    setState({ loading: false, user: null, org: null })
  }

  return (
    <AuthContext.Provider value={{ ...state, login, logout, register, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}
