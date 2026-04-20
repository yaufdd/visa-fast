import { useState, useEffect } from 'react';
import Modal from './Modal';

// Shape of flight_data in DB (matches backend/internal/ai/ticket_parser.go):
//   { arrival:   { flight_number, date, time, airport },
//     departure: { flight_number, date, time, airport } }
// "airport" is the Japanese airport touched on that leg (landing airport for arrival,
// take-off airport for departure). Empty departure fields mean one-way.

function emptyLeg() {
  return {
    flight_number: '',
    date: '',
    time: '',
    airport: '',
  };
}

function normalizeLeg(leg) {
  if (!leg || typeof leg !== 'object') return emptyLeg();
  return {
    flight_number: leg.flight_number || '',
    date: leg.date || '',
    time: leg.time || '',
    airport: leg.airport || '',
  };
}

function isEmptyLeg(leg) {
  return !leg.flight_number && !leg.date && !leg.time && !leg.airport;
}

export default function FlightDataForm({ open, initial, onClose, onSave }) {
  const [arrival, setArrival] = useState(emptyLeg());
  const [departure, setDeparture] = useState(emptyLeg());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open) return;
    setArrival(normalizeLeg(initial?.arrival));
    setDeparture(normalizeLeg(initial?.departure));
    setError(null);
  }, [open, initial]);

  const update = (setter) => (key) => (e) => {
    const v = e.target.value;
    setter((prev) => ({ ...prev, [key]: v }));
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      // Send empty departure object if all fields blank (backend stores as-is).
      const payload = {
        arrival,
        departure: isEmptyLeg(departure) ? {} : departure,
      };
      await onSave(payload);
      onClose();
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const renderLeg = (leg, setter, prefix) => {
    const airportHint =
      prefix === 'arrival'
        ? 'Японский аэропорт прилёта (NARITA, KANSAI, ...)'
        : 'Японский аэропорт вылета (NARITA, KANSAI, ...)';
    return (
      <div className="flight-leg">
        <div className="grid-2">
          <div className="form-group">
            <label className="form-label">Номер рейса</label>
            <input
              className="form-input"
              value={leg.flight_number}
              onChange={update(setter)('flight_number')}
              placeholder="SU 262"
            />
          </div>
          <div className="form-group">
            <label className="form-label">Дата</label>
            <input
              className="form-input"
              value={leg.date}
              onChange={update(setter)('date')}
              placeholder="ДД.ММ.ГГГГ"
            />
          </div>
          <div className="form-group">
            <label className="form-label">Время</label>
            <input
              className="form-input"
              value={leg.time}
              onChange={update(setter)('time')}
              placeholder="ЧЧ:ММ"
            />
          </div>
          <div className="form-group" style={{ gridColumn: '1 / -1' }}>
            <label className="form-label">Аэропорт (японский)</label>
            <input
              className="form-input"
              value={leg.airport}
              onChange={update(setter)('airport')}
              placeholder="TOKYO NARITA"
            />
            <div
              style={{
                fontSize: 11,
                color: 'var(--white-dim)',
                marginTop: 4,
              }}
            >
              {airportHint}
            </div>
          </div>
        </div>
        {prefix === 'departure' && (
          <div
            style={{
              fontSize: 11,
              color: 'var(--white-dim)',
              marginTop: 4,
              fontStyle: 'italic',
            }}
          >
            Оставьте пустым, если билет в один конец.
          </div>
        )}
      </div>
    );
  };

  return (
    <Modal open={open} onClose={() => !saving && onClose()} title="Рейсы" width={560}>
      {error && <div className="error-message">{error}</div>}

      <div
        style={{
          fontSize: 12,
          color: 'var(--accent)',
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          marginBottom: 8,
        }}
      >
        Прилёт
      </div>
      {renderLeg(arrival, setArrival, 'arrival')}

      <div className="divider" />

      <div
        style={{
          fontSize: 12,
          color: 'var(--accent)',
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          marginBottom: 8,
        }}
      >
        Обратный рейс
      </div>
      {renderLeg(departure, setDeparture, 'departure')}

      <div
        style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 16 }}
      >
        <button
          type="button"
          className="btn btn-ghost"
          onClick={onClose}
          disabled={saving}
        >
          Отмена
        </button>
        <button
          type="button"
          className="btn btn-primary"
          onClick={handleSave}
          disabled={saving}
        >
          {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить'}
        </button>
      </div>
    </Modal>
  );
}
