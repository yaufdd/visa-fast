export default function FormThanksPage() {
  return (
    <main className="thanks-page">
      <div className="thanks-card">
        <h1>Спасибо!</h1>
        <p>Ваша анкета отправлена. Менеджер свяжется с вами в ближайшее время.</p>
      </div>

      <style>{`
        .thanks-page {
          min-height: 60vh;
          display: flex;
          align-items: center;
          justify-content: center;
          padding: 40px 24px;
          color: var(--white);
          font-family: var(--font-body);
        }

        .thanks-card {
          max-width: 480px;
          text-align: center;
          background: var(--graphite);
          border: 1px solid var(--border);
          border-radius: 12px;
          padding: 40px 32px;
        }

        .thanks-card h1 {
          font-size: 24px;
          font-weight: 600;
          color: var(--white);
          margin-bottom: 12px;
        }

        .thanks-card p {
          font-size: 14px;
          color: var(--white-dim);
          line-height: 1.6;
        }
      `}</style>
    </main>
  );
}
