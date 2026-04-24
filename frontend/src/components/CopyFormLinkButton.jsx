import { useState } from 'react'
import { useAuth } from '../auth/AuthContext'
import Modal from './Modal'

export default function CopyFormLinkButton() {
  const { org } = useAuth()
  const [copied, setCopied] = useState(false)
  const [fallbackOpen, setFallbackOpen] = useState(false)
  if (!org?.slug) return null
  const url = `${window.location.origin}/form/${org.slug}`

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API unavailable (http origin, permission denied, etc.) —
      // show the URL in our own modal so the user can copy it manually.
      setFallbackOpen(true)
    }
  }

  return (
    <>
      <button onClick={copy} className="btn btn-secondary btn-sm copy-form-link-btn">
        {copied ? '✓ Скопировано' : '📎 Ссылка на анкету'}
      </button>

      <Modal
        open={fallbackOpen}
        onClose={() => setFallbackOpen(false)}
        title="Ссылка на анкету"
        width={500}
      >
        <div style={{ fontSize: 13, color: 'var(--white-dim)', marginBottom: 10 }}>
          Выделите и скопируйте ссылку:
        </div>
        <input
          className="form-input"
          value={url}
          readOnly
          onFocus={(e) => e.target.select()}
          style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}
        />
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => setFallbackOpen(false)}
          >
            Закрыть
          </button>
        </div>
      </Modal>
    </>
  )
}
