import { useState, useEffect } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import { getHotel, updateHotel } from '../api/client';
import { CANONICAL_CITIES, normalizeCity } from '../constants/cities';

const EMPTY_FORM = {
  name_en: '',
  city: '',
  address: '',
  phone: '',
};

export default function HotelEditPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [form, setForm] = useState(EMPTY_FORM);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [saveError, setSaveError] = useState(null);

  useEffect(() => {
    (async () => {
      try {
        setLoading(true);
        const h = await getHotel(id);
        setForm({
          name_en: h.name_en || '',
          // Legacy rows may still carry English cities — normalize on load
          // so the UI always shows canonical Russian and saves it back.
          city: normalizeCity(h.city || ''),
          address: h.address || '',
          phone: h.phone || '',
        });
      } catch (e) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    })();
  }, [id]);

  const handleChange = (field, value) => {
    setForm(f => ({ ...f, [field]: value }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!form.name_en.trim()) return;
    setSaving(true);
    setSaveError(null);
    try {
      await updateHotel(id, { ...form, city: normalizeCity(form.city) });
      navigate('/hotels');
    } catch (err) {
      setSaveError(err.message);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="page-container">
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          <span>Загрузка...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-container">
        <div className="error-message">{error}</div>
        <Link to="/hotels" className="btn btn-secondary" style={{ marginTop: 12 }}>← К списку</Link>
      </div>
    );
  }

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Редактировать отель</div>
          <div className="page-subtitle">{form.name_en}</div>
        </div>
        <Link to="/hotels" className="btn btn-secondary">← К списку</Link>
      </div>

      {saveError && <div className="error-message">{saveError}</div>}

      <datalist id="city-suggestions">
        {CANONICAL_CITIES.map(c => <option key={c} value={c} />)}
      </datalist>

      <div className="card">
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
              onClick={() => navigate('/hotels')}
              disabled={saving}
            >
              Отмена
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={saving || !form.name_en.trim()}
            >
              {saving ? <><span className="spinner" /> Сохранение...</> : 'Сохранить'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
