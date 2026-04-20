import { useMemo, useState } from 'react';
import FlightDataForm from './FlightDataForm';
import { updateFlightData } from '../api/client';

function safeParse(raw) {
  if (!raw) return null;
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return null; }
}

function formatLeg(leg) {
  if (!leg) return null;
  const parts = [];
  if (leg.flight_number) parts.push(leg.flight_number);
  const route = [leg.airport_from, leg.airport_to].filter(Boolean).join(' → ');
  if (route) parts.push(route);
  const when = [leg.date, leg.time].filter(Boolean).join(' ');
  if (when) parts.push(when);
  return parts.length ? parts.join(' · ') : null;
}

function isEmpty(leg) {
  return !leg || (!leg.flight_number && !leg.date && !leg.time
    && !leg.airport_from && !leg.airport_to);
}

export default function FlightDataCard({ tourist, onUpdated }) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState(null);

  const flight = useMemo(() => safeParse(tourist.flight_data) || {}, [tourist]);
  const arrivalStr = formatLeg(flight.arrival);
  const departureStr = formatLeg(flight.departure);
  const hasAny = !isEmpty(flight.arrival) || !isEmpty(flight.departure);

  const handleSave = async (data) => {
    setError(null);
    try {
      await updateFlightData(tourist.id, data);
      onUpdated?.();
    } catch (e) {
      setError(e.message);
      throw e;
    }
  };

  return (
    <div className="flight-card">
      <div className="flight-card-header">
        <span className="flight-card-title">✈ Рейсы</span>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => setOpen(true)}
        >
          {hasAny ? 'Изменить' : 'Добавить'}
        </button>
      </div>

      {error && (
        <div className="error-message" style={{ margin: '8px 0 0' }}>{error}</div>
      )}

      {hasAny ? (
        <div className="flight-card-body">
          {arrivalStr && (
            <div className="flight-leg-row">
              <span className="flight-leg-label">Прилёт</span>
              <span className="flight-leg-value">{arrivalStr}</span>
            </div>
          )}
          {departureStr && (
            <div className="flight-leg-row">
              <span className="flight-leg-label">Обратно</span>
              <span className="flight-leg-value">{departureStr}</span>
            </div>
          )}
          {!arrivalStr && !departureStr && (
            <div className="flight-card-empty">Данные заполнены частично</div>
          )}
        </div>
      ) : (
        <div className="flight-card-empty">Нет данных о рейсах</div>
      )}

      <FlightDataForm
        open={open}
        initial={flight}
        onClose={() => setOpen(false)}
        onSave={handleSave}
      />
    </div>
  );
}
