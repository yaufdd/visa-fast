import { useNavigate } from 'react-router-dom';
import SubmissionForm from '../components/SubmissionForm';
import { createSubmission } from '../api/client';

export default function SubmissionFormPage() {
  const nav = useNavigate();

  const handleSubmit = async (payload, consent) => {
    await createSubmission(payload, consent, 'tourist');
    nav('/form/thanks');
  };

  return (
    <main className="public-form">
      <header className="public-form-header">
        <h1>Анкета для оформления визы в Японию</h1>
        <p className="lead">FujiTravel · пожалуйста, заполните данные как в паспорте.</p>
      </header>
      <SubmissionForm onSubmit={handleSubmit} />

      <style>{`
        .public-form {
          max-width: 880px;
          margin: 0 auto;
          padding: 40px 24px 64px;
          color: var(--white);
          font-family: var(--font-body);
        }

        .public-form-header {
          margin-bottom: 28px;
          text-align: center;
        }

        .public-form-header h1 {
          font-size: 24px;
          font-weight: 600;
          margin-bottom: 8px;
          color: var(--white);
        }

        .public-form-header .lead {
          font-size: 14px;
          color: var(--white-dim);
        }
      `}</style>
    </main>
  );
}
