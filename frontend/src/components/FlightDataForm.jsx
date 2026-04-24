import { useState, useEffect } from 'react';
import Modal from './Modal';
import ConfirmModal from './ConfirmModal';
import FlightNumberInput from './FlightNumberInput';
import { dmyToIso, isoToDmy } from '../utils/dates';
import { JAPANESE_AIRPORTS, JAPANESE_AIRPORT_VALUES } from '../constants/airports';

// Shape of flight_data in DB (matches backend/internal/ai/ticket_parser.go):
//   { arrival:   { flight_number, date, time, airport },
//     departure: { flight_number, date, time, airport } }
// "airport" is the Japanese airport touched on that leg (landing airport for arrival,
// take-off airport for departure). Empty departure fields mean one-way.

const CUSTOM_AIRPORT = '__custom__';

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

export default function FlightDataForm({
  open,
  initial,
  onClose,
  onSave,
  onApplyToSubgroup,
  canApplyToSubgroup = false,
}) {
  const [arrival, setArrival] = useState(emptyLeg());
  const [departure, setDeparture] = useState(emptyLeg());
  const [saving, setSaving] = useState(false);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState(null);
  // Per-leg toggle: when the stored airport isn't a canonical preset, we flip
  // this to true so the user can keep typing the non-Japanese name.
  const [customArrival, setCustomArrival] = useState(false);
  const [customDeparture, setCustomDeparture] = useState(false);
  const [confirmApplyOpen, setConfirmApplyOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    const a = normalizeLeg(initial?.arrival);
    const d = normalizeLeg(initial?.departure);
    setArrival(a);
    setDeparture(d);
    setCustomArrival(!!a.airport && !JAPANESE_AIRPORT_VALUES.has(a.airport));
    setCustomDeparture(!!d.airport && !JAPANESE_AIRPORT_VALUES.has(d.airport));
    setError(null);
  }, [open, initial]);

  const setLegField = (setter) => (key, value) => {
    setter((prev) => ({ ...prev, [key]: value }));
  };

  const buildPayload = () => ({
    arrival,
    departure: isEmptyLeg(departure) ? {} : departure,
  });

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(buildPayload());
      onClose();
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const requestApplyToSubgroup = () => {
    if (!onApplyToSubgroup) return;
    setError(null);
    setConfirmApplyOpen(true);
  };

  const handleApplyToSubgroup = async () => {
    setApplying(true);
    setError(null);
    try {
      await onApplyToSubgroup(buildPayload());
      setConfirmApplyOpen(false);
      onClose();
    } catch (e) {
      setError(e.message);
    } finally {
      setApplying(false);
    }
  };

  const renderAirport = (leg, setter, isCustom, setIsCustom, prefix) => {
    const selectValue = isCustom
      ? CUSTOM_AIRPORT
      : (JAPANESE_AIRPORT_VALUES.has(leg.airport) ? leg.airport : '');
    const handleSelect = (e) => {
      const v = e.target.value;
      if (v === CUSTOM_AIRPORT) {
        setIsCustom(true);
        setLegField(setter)('airport', '');
      } else {
        setIsCustom(false);
        setLegField(setter)('airport', v);
      }
    };
    return (
      <div className="form-group" style={{ gridColumn: '1 / -1' }}>
        <label className="form-label">
          Аэропорт {prefix === 'arrival' ? 'прилёта' : 'вылета'} (японский)
        </label>
        <select
          className="form-input"
          value={selectValue}
          onChange={handleSelect}
        >
          <option value="">— Выберите аэропорт —</option>
          {JAPANESE_AIRPORTS.map((a) => (
            <option key={a.value} value={a.value}>{a.label}</option>
          ))}
          <option value={CUSTOM_AIRPORT}>Другой аэропорт…</option>
        </select>
        {isCustom && (
          <input
            className="form-input"
            style={{ marginTop: 6 }}
            value={leg.airport}
            onChange={(e) => setLegField(setter)('airport', e.target.value)}
            placeholder="Введите полное название аэропорта"
          />
        )}
      </div>
    );
  };

  const renderLeg = (leg, setter, isCustom, setIsCustom, prefix) => (
    <div className="flight-leg">
      <div className="grid-2">
        <div className="form-group" style={{ gridColumn: '1 / -1' }}>
          <label className="form-label">Номер рейса</label>
          <FlightNumberInput
            value={leg.flight_number}
            onChange={(v) => setLegField(setter)('flight_number', v)}
          />
        </div>
        <div className="form-group">
          <label className="form-label">Дата</label>
          <input
            type="date"
            className="form-input"
            value={dmyToIso(leg.date)}
            onChange={(e) => setLegField(setter)('date', isoToDmy(e.target.value))}
          />
        </div>
        <div className="form-group">
          <label className="form-label">Время</label>
          <input
            type="time"
            className="form-input"
            value={leg.time}
            onChange={(e) => setLegField(setter)('time', e.target.value)}
          />
        </div>
        {renderAirport(leg, setter, isCustom, setIsCustom, prefix)}
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

  const busy = saving || applying;

  return (
    <Modal open={open} onClose={() => !busy && onClose()} title="Рейсы" width={560}>
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
      {renderLeg(arrival, setArrival, customArrival, setCustomArrival, 'arrival')}

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
      {renderLeg(departure, setDeparture, customDeparture, setCustomDeparture, 'departure')}

      <div
        style={{
          display: 'flex',
          gap: 10,
          justifyContent: 'space-between',
          alignItems: 'center',
          marginTop: 16,
          flexWrap: 'wrap',
        }}
      >
        <div>
          {canApplyToSubgroup && (
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={requestApplyToSubgroup}
              disabled={busy}
              title="Сохранить этот перелёт для всех туристов в подгруппе"
            >
              {applying
                ? <><span className="spinner" /> Применение…</>
                : '↯ Применить ко всей подгруппе'}
            </button>
          )}
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button
            type="button"
            className="btn btn-ghost"
            onClick={onClose}
            disabled={busy}
          >
            Отмена
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={handleSave}
            disabled={busy}
          >
            {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить'}
          </button>
        </div>
      </div>
      <ConfirmModal
        open={confirmApplyOpen}
        title="Применить ко всей подгруппе?"
        message="Скопировать этот перелёт всем остальным туристам в подгруппе? Их текущие данные о рейсах будут перезаписаны."
        confirmText="Применить"
        cancelText="Отмена"
        variant="primary"
        busy={applying}
        onConfirm={handleApplyToSubgroup}
        onCancel={() => setConfirmApplyOpen(false)}
      />
    </Modal>
  );
}
