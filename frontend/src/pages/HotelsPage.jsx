import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { getHotels, createHotel } from '../api/client';
import { CANONICAL_CITIES, normalizeCity } from '../constants/cities';

const EMPTY_FORM = {
  name_en: '',
  city: '',
  address: '',
  phone: '',
};

export default function HotelsPage() {
  const navigate = useNavigate();
  const [hotels, setHotels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [form, setForm] = useState(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [formOpen, setFormOpen] = useState(false);

  const load = async () => {
    try {
      setLoading(true);
      const data = await getHotels();
      setHotels(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleChange = (field, value) => {
    setForm(f => ({ ...f, [field]: value }));
  };

  const closeForm = () => {
    setFormOpen(false);
    setForm(EMPTY_FORM);
    setSaveError(null);
  };

  const openForm = () => {
    setFormOpen(true);
    setSaveError(null);
    setSaveSuccess(false);
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!form.name_en.trim()) return;
    setSaving(true);
    setSaveError(null);
    setSaveSuccess(false);
    try {
      await createHotel({ ...form, city: normalizeCity(form.city) });
      setForm(EMPTY_FORM);
      setFormOpen(false);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
      await load();
    } catch (e) {
      setSaveError(e.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Отели</div>
          <div className="page-subtitle">База отелей для программы поездок</div>
        </div>
        {!formOpen && (
          <button
            type="button"
            className="btn btn-primary"
            onClick={openForm}
          >
            + Добавить отель
          </button>
        )}
      </div>

      {error && <div className="error-message">{error}</div>}
      {saveSuccess && <div className="success-message">Отель добавлен успешно</div>}

      {/* Shared datalist for city autocomplete */}
      <datalist id="city-suggestions">
        {CANONICAL_CITIES.map(c => <option key={c} value={c} />)}
      </datalist>

      {/* Add hotel form (collapsible, above table) */}
      {formOpen && (
        <div className="card" style={{ marginBottom: 24 }}>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: 20,
          }}>
            <div className="section-title">Добавить отель</div>
            <button
              type="button"
              onClick={closeForm}
              title="Закрыть"
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--white-dim)',
                fontSize: 18,
                cursor: 'pointer',
                lineHeight: 1,
                padding: 4,
              }}
            >
              ✕
            </button>
          </div>

          {saveError && <div className="error-message">{saveError}</div>}

          <form onSubmit={handleSubmit}>
            <div className="form-group">
              <label className="form-label">Название (English) *</label>
              <input
                className="form-input"
                type="text"
                placeholder="Hotel name in English"
                value={form.name_en}
                onChange={e => handleChange('name_en', e.target.value)}
              />
            </div>

            <div className="grid-3">
              <div className="form-group">
                <label className="form-label">Город (по-русски)</label>
                <input
                  className="form-input"
                  type="text"
                  placeholder="Токио"
                  list="city-suggestions"
                  value={form.city}
                  onChange={e => handleChange('city', e.target.value)}
                  onBlur={e => handleChange('city', normalizeCity(e.target.value))}
                />
              </div>
              <div className="form-group">
                <label className="form-label">Телефон</label>
                <input
                  className="form-input"
                  type="text"
                  placeholder="+81-3-XXXX-XXXX"
                  value={form.phone}
                  onChange={e => handleChange('phone', e.target.value)}
                />
              </div>
            </div>

            <div className="form-group">
              <label className="form-label">Адрес</label>
              <input
                className="form-input"
                type="text"
                placeholder="Full address"
                value={form.address}
                onChange={e => handleChange('address', e.target.value)}
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10, marginTop: 8 }}>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={closeForm}
                disabled={saving}
              >
                Отмена
              </button>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={saving || !form.name_en.trim()}
              >
                {saving ? <><span className="spinner" /> Сохранение...</> : '+ Добавить отель'}
              </button>
            </div>
          </form>
        </div>
      )}

      {loading ? (
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          <span>Загрузка...</span>
        </div>
      ) : hotels.length === 0 ? null : (
        <div className="table-wrapper" style={{ marginBottom: 32 }}>
          <table>
            <thead>
              <tr>
                <th>Название (EN)</th>
                <th>Город</th>
                <th>Адрес</th>
                <th>Телефон</th>
              </tr>
            </thead>
            <tbody>
              {hotels.map(h => {
                const cityDisplay = normalizeCity(h.city);
                return (
                  <tr
                    key={h.id}
                    onClick={() => navigate(`/hotels/${h.id}`)}
                    style={{ cursor: 'pointer' }}
                    title="Нажмите, чтобы редактировать"
                  >
                    <td style={{ fontWeight: 500 }}>{h.name_en}</td>
                    <td>
                      <span style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        minWidth: 90,
                        height: 22,
                        padding: '0 10px',
                        borderRadius: 4,
                        background: cityDisplay ? 'var(--accent-dim)' : 'transparent',
                        color: cityDisplay ? 'var(--accent)' : 'var(--white-dim)',
                        fontSize: 11,
                        fontWeight: 500,
                        textAlign: 'center',
                        boxSizing: 'border-box',
                      }}>
                        {cityDisplay || '—'}
                      </span>
                    </td>
                    <td style={{ fontSize: 12, color: 'var(--white-dim)' }}>{h.address || '—'}</td>
                    <td>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--white-dim)' }}>
                        {h.phone || '—'}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
