import { useEffect, useState } from 'react';
import { getConsentText } from '../api/client';

export default function ConsentPage() {
  const [c, setC] = useState(null);
  const [error, setError] = useState('');

  useEffect(() => {
    getConsentText()
      .then(setC)
      .catch((err) => setError(err?.message || 'Не удалось загрузить текст согласия.'));
  }, []);

  if (error) {
    return (
      <article className="consent-page">
        <h1>Согласие на обработку персональных данных</h1>
        <p className="consent-error">{error}</p>
      </article>
    );
  }

  if (!c) {
    return (
      <article className="consent-page">
        <p className="consent-loading">Загрузка…</p>
      </article>
    );
  }

  return (
    <article className="consent-page">
      <h1>Согласие на обработку персональных данных</h1>
      <p className="version">Версия: {c.version}</p>
      <pre className="consent-text">{c.body}</pre>

      <style>{`
        .consent-page {
          max-width: 820px;
          margin: 0 auto;
          padding: 40px 24px 64px;
          color: var(--white);
          font-family: var(--font-body);
        }

        .consent-page h1 {
          font-size: 22px;
          font-weight: 600;
          margin-bottom: 8px;
          color: var(--white);
        }

        .consent-page .version {
          font-family: var(--font-mono);
          font-size: 12px;
          color: var(--white-dim);
          margin-bottom: 24px;
        }

        .consent-text {
          background: var(--graphite);
          border: 1px solid var(--border);
          border-radius: 8px;
          padding: 20px 22px;
          font-family: var(--font-mono);
          font-size: 13px;
          line-height: 1.6;
          color: var(--white-dim);
          white-space: pre-wrap;
          overflow-x: auto;
        }

        .consent-loading {
          color: var(--white-dim);
          font-family: var(--font-mono);
          font-size: 13px;
        }

        .consent-error {
          color: var(--danger);
          font-size: 13px;
        }
      `}</style>
    </article>
  );
}
