// HotelsListModal — manager-side modal for reviewing / editing the list
// of hotels parsed from a voucher (or entered manually) on the
// submission. Opens from the «✎ Открыть» button under the
// Документы → Ваучеры section.
//
// Stored as `payload.hotels` (array of plain objects). When the
// submission is later attached to a tour, this list can be migrated
// into group_hotels — that backfill is out of scope for this component.

import { useEffect, useState } from 'react';
import Modal from '../Modal';

function emptyHotel() {
  return {
    hotel_name: '',
    city: '',
    address: '',
    check_in: '',
    check_out: '',
  };
}

export default function HotelsListModal({ open, onClose, value, onChange }) {
  const [hotels, setHotels] = useState([]);

  useEffect(() => {
    if (!open) return;
    const seeded = Array.isArray(value) && value.length > 0
      ? value.map((h) => ({ ...emptyHotel(), ...h }))
      : [emptyHotel()]; // open with one blank card so the user sees the form right away
    setHotels(seeded);
  }, [open, value]);

  const update = (idx, patch) => {
    setHotels((arr) => arr.map((h, i) => (i === idx ? { ...h, ...patch } : h)));
  };
  const add = () => setHotels((arr) => [...arr, emptyHotel()]);
  const remove = (idx) => setHotels((arr) => arr.filter((_, i) => i !== idx));

  const save = () => {
    onChange(hotels);
    onClose();
  };

  return (
    <Modal open={open} onClose={onClose} title="Отели по ваучерам" width={640}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {hotels.map((h, idx) => (
            <div
              key={idx}
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: 8,
                padding: 12,
                border: '1px solid var(--border)',
                borderRadius: 6,
                background: 'var(--graphite)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: 11, color: 'var(--white-dim)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Отель {idx + 1}
                </span>
                <button
                  type="button"
                  onClick={() => remove(idx)}
                  title="Удалить"
                  style={{
                    background: 'none', border: 'none', cursor: 'pointer',
                    color: 'var(--white-dim)', fontSize: 16, padding: '0 4px',
                  }}
                >×</button>
              </div>
              <div className="form-group" style={{ marginBottom: 0 }}>
                <label className="form-label">Название</label>
                <input
                  className="form-input"
                  value={h.hotel_name}
                  onChange={(e) => update(idx, { hotel_name: e.target.value })}
                  placeholder="Hotel Granvia Kyoto"
                />
              </div>
              <div className="grid-2">
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Город</label>
                  <input
                    className="form-input"
                    value={h.city}
                    onChange={(e) => update(idx, { city: e.target.value })}
                    placeholder="Kyoto"
                  />
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Адрес</label>
                  <input
                    className="form-input"
                    value={h.address}
                    onChange={(e) => update(idx, { address: e.target.value })}
                  />
                </div>
              </div>
              <div className="grid-2">
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Заезд</label>
                  <input
                    type="date"
                    className="form-input"
                    value={h.check_in}
                    onChange={(e) => update(idx, { check_in: e.target.value })}
                  />
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Выезд</label>
                  <input
                    type="date"
                    className="form-input"
                    value={h.check_out}
                    onChange={(e) => update(idx, { check_out: e.target.value })}
                  />
                </div>
              </div>
            </div>
        ))}

        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
          <button type="button" className="btn btn-secondary btn-sm" onClick={add}>
            + Отель
          </button>
          <div style={{ display: 'flex', gap: 8 }}>
            <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>
              Отмена
            </button>
            <button type="button" className="btn btn-primary btn-sm" onClick={save}>
              Сохранить
            </button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
