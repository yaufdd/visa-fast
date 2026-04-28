import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getTourist } from '../api/client';
import { OCCUPATION_OPTIONS, OCCUPATION_DEFAULT } from '../components/forms/steps/OccupationStep';
import '../components/forms/wizard.css';

const dash = '—';

function fmt(v) {
  const s = String(v ?? '').trim();
  return s === '' ? dash : s;
}

function safeParse(raw) {
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

function occupationLabel(payload) {
  const t = payload.occupation_type || OCCUPATION_DEFAULT;
  const opt = OCCUPATION_OPTIONS.find((o) => o.value === t);
  return opt ? opt.label : t;
}

function formatLeg(leg) {
  if (!leg) return dash;
  const parts = [];
  if (leg.flight_number) parts.push(leg.flight_number);
  if (leg.airport) parts.push(leg.airport);
  const when = [leg.date, leg.time].filter(Boolean).join(' ');
  if (when) parts.push(when);
  return parts.length ? parts.join(' · ') : dash;
}

function Row({ label, value }) {
  return (
    <>
      <dt>{label}</dt>
      <dd>{fmt(value)}</dd>
    </>
  );
}

export default function TouristDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [tourist, setTourist] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError(null);
    getTourist(id)
      .then((t) => { if (alive) setTourist(t); })
      .catch((e) => { if (alive) setError(e.message); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [id]);

  if (loading) {
    return (
      <div className="page-container">
        <div className="loading-center">
          <div className="spinner spinner-lg" /> Загрузка...
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-container">
        <div className="error-message">{error}</div>
        <button className="btn btn-ghost" onClick={() => navigate(-1)}>← Назад</button>
      </div>
    );
  }

  if (!tourist) return null;

  const payload = safeParse(tourist.submission_snapshot);
  const flight = safeParse(tourist.flight_data);
  const name = payload.name_lat || payload.name_cyr || 'Турист';
  const groupHref = `/groups/${tourist.group_id}`;

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
            <button
              onClick={() => navigate(groupHref)}
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 13,
                cursor: 'pointer',
                padding: 0,
              }}
            >
              ← К группе
            </button>
            <span style={{ color: 'var(--border)' }}>/</span>
            <span style={{ color: 'var(--white-dim)', fontSize: 13 }}>
              {tourist.id.slice(0, 8)}
            </span>
          </div>
          <div className="page-title">{name}</div>
          {payload.name_cyr && payload.name_lat && (
            <div className="page-subtitle" style={{ marginTop: 4 }}>
              {payload.name_cyr}
            </div>
          )}
        </div>

        {tourist.submission_id && (
          <div style={{ display: 'flex', gap: 8 }}>
            <button
              type="button"
              className="btn btn-ghost"
              onClick={() => navigate(`/submissions/${tourist.submission_id}`)}
            >
              Открыть анкету
            </button>
          </div>
        )}
      </div>

      <section className="fw-review-section">
        <h3>Личные данные</h3>
        <dl className="fw-review-dl">
          <Row label="ФИО кириллицей" value={payload.name_cyr} />
          <Row label="ФИО латиницей" value={payload.name_lat} />
          <Row label="Пол" value={payload.gender_ru} />
          <Row label="Дата рождения" value={payload.birth_date} />
          <Row label="Семейное положение" value={payload.marital_status_ru} />
          <Row label="Место рождения" value={payload.place_of_birth_ru} />
          <Row label="Гражданство" value={payload.nationality_ru} />
          <Row label="Прежнее гражданство" value={payload.former_nationality_ru} />
          <Row label="Другая фамилия" value={payload.maiden_name_ru} />
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Внутренний паспорт РФ</h3>
        <dl className="fw-review-dl">
          <Row label="Серия" value={payload.internal_series} />
          <Row label="Номер" value={payload.internal_number} />
          <Row label="Дата выдачи" value={payload.internal_issued_ru} />
          <Row label="Кем выдан" value={payload.internal_issued_by_ru} />
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Загранпаспорт</h3>
        <dl className="fw-review-dl">
          <Row label="Номер" value={payload.passport_number} />
          <Row label="Тип паспорта" value={payload.passport_type_ru} />
          <Row label="Дата выдачи" value={payload.issue_date} />
          <Row label="Дата окончания" value={payload.expiry_date} />
          <Row label="Кем выдан" value={payload.issued_by_ru} />
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Адреса</h3>
        <dl className="fw-review-dl">
          <Row label="Адрес регистрации" value={payload.reg_address_ru} />
          <Row label="Домашний адрес" value={payload.home_address_ru} />
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Работа</h3>
        <dl className="fw-review-dl">
          <Row label="Род занятий" value={occupationLabel(payload)} />
          {(payload.occupation_type || OCCUPATION_DEFAULT) === 'employed' && (
            <>
              <Row label="Должность" value={payload.occupation_ru} />
              <Row label="Название организации" value={payload.employer_ru} />
              <Row label="Адрес организации" value={payload.employer_address_ru} />
              <Row label="Телефон организации" value={payload.employer_phone} />
            </>
          )}
          {(payload.occupation_type === 'student' || payload.occupation_type === 'schoolchild') && (
            <>
              <Row label="Учебное заведение" value={payload.employer_ru} />
              <Row label="Адрес учебного заведения" value={payload.employer_address_ru} />
              <Row label="Телефон учебного заведения" value={payload.employer_phone} />
            </>
          )}
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Поездка</h3>
        <dl className="fw-review-dl">
          <Row label="Был ли в Японии" value={payload.been_to_japan_ru} />
          {payload.been_to_japan_ru === 'Да' && (
            <Row label="Прошлые визиты" value={payload.previous_visits_ru} />
          )}
          <Row label="Была ли судимость" value={payload.criminal_record_ru} />
          <Row label="Телефон" value={payload.phone} />
        </dl>
      </section>

      <section className="fw-review-section">
        <h3>Рейсы</h3>
        <dl className="fw-review-dl">
          <Row label="Прилёт" value={formatLeg(flight.arrival)} />
          <Row label="Вылет" value={formatLeg(flight.departure)} />
        </dl>
      </section>
    </div>
  );
}
