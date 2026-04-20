import { useState } from 'react'
import { useAuth } from '../auth/AuthContext'

export default function CopyFormLinkButton() {
  const { org } = useAuth()
  const [copied, setCopied] = useState(false)
  if (!org?.slug) return null
  const url = `${window.location.origin}/form/${org.slug}`

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      prompt('Скопируйте ссылку:', url)
    }
  }

  return (
    <button onClick={copy} className="btn btn-secondary btn-sm copy-form-link-btn">
      {copied ? '✓ Скопировано' : '📎 Ссылка на анкету'}
    </button>
  )
}
