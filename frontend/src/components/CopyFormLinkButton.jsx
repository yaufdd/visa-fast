import { useState } from 'react'
import { useAuth } from '../auth/AuthContext'
import Modal from './Modal'

// Chain-link icon — replaces the 📎 emoji we used before; renders the
// same on every platform (some Androids drew a paperclip-with-attach
// flag emoji that looked off in admin chrome).
function LinkIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path
        d="M6.5 9.5L9.5 6.5M6.7 4.6l1.4-1.4a3 3 0 1 1 4.24 4.24l-1.4 1.4M9.3 11.4l-1.4 1.4a3 3 0 0 1-4.24-4.24l1.4-1.4"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

// Checkmark — shown briefly after a successful copy, paired with a
// little scale-pop in CSS.
function CheckIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path
        d="M3 8.5l3 3 7-7"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export default function CopyFormLinkButton() {
  const { org } = useAuth()
  const [copied, setCopied] = useState(false)
  const [popping, setPopping] = useState(false)
  const [fallbackOpen, setFallbackOpen] = useState(false)
  if (!org?.slug) return null
  const url = `${window.location.origin}/form/${org.slug}`

  const copy = async () => {
    setPopping(true)
    setTimeout(() => setPopping(false), 700)
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
      <button
        onClick={copy}
        className={`btn btn-secondary copy-form-link-btn${popping ? ' is-popping' : ''}${copied ? ' is-copied' : ''}`}
        style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}
      >
        <span style={{ display: 'inline-flex', width: 14, height: 14, alignItems: 'center', justifyContent: 'center' }}>
          {copied ? <CheckIcon /> : <LinkIcon />}
        </span>
        {copied ? 'Скопировано' : 'Ссылка на анкету'}
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
