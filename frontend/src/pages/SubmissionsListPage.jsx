import { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { listSubmissions } from '../api/client';

function formatDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: '2-digit', month: 'short', year: 'numeric',
  });
}

const STATUS_LABELS = {
  pending: 'Ожидает',
  attached: 'Привязана',
  archived: 'Архив',
};

function safeParsePayload(raw) {
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

function submissionName(s) {
  const p = safeParsePayload(s.payload);
  return p.name_lat || p.name_cyr || '—';
}

export default function SubmissionsListPage() {
  const navigate = useNavigate();
  const [rows, setRows] = useState([]);
  const [q, setQ] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const debounceRef = useRef(null);

  const load = useCallback(async (query, status) => {
    try {
      setLoading(true);
      setError(null);
      const data = await listSubmissions(query, status);
      setRows(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  // Debounced search
  useEffect(() => {
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => load(q, statusFilter), 300);
    return () => clearTimeout(debounceRef.current);
  }, [q, statusFilter, load]);

  return (
    <div className="page-container submissions-page">
      <div className="page-header">
        <div>
          <div className="page-title">Анкеты</div>
          <div className="page-subtitle">Пул анкет от туристов и менеджера</div>
        </div>
        <button
          className="btn btn-primary"
          onClick={() => navigate('/submissions/new')}
        >
          <span>+</span> Создать вручную
        </button>
      </div>

      <div className="submissions-filters">
        <input
          className="form-input"
          type="text"
          placeholder="Поиск по имени (латиницей)..."
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <select
          className="form-input"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          style={{ maxWidth: 200 }}
        >
          <option value="">Все</option>
          <option value="pending">Ожидает</option>
          <option value="attached">Привязана</option>
          <option value="archived">Архив</option>
        </select>
      </div>

      {error && <div className="error-message">{error}</div>}

      {loading ? (
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          <span>Загрузка...</span>
        </div>
      ) : rows.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="empty-state-icon">⌬</div>
            <div className="empty-state-title">Нет анкет</div>
            <div className="empty-state-text">
              Здесь появятся анкеты, заполненные туристами или вручную менеджером
            </div>
          </div>
        </div>
      ) : (
        <div className="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>Имя (латиницей)</th>
                <th>Статус</th>
                <th>Дата</th>
                <th>Источник</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const badgeClass = `submission-status submission-status--${s.status || 'pending'}`;
                return (
                  <tr
                    key={s.id}
                    className="clickable"
                    onClick={() => navigate(`/submissions/${s.id}`)}
                  >
                    <td>
                      <span style={{ fontWeight: 500 }}>{submissionName(s)}</span>
                    </td>
                    <td>
                      <span className={badgeClass}>
                        {STATUS_LABELS[s.status] || s.status || '—'}
                      </span>
                    </td>
                    <td>
                      <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
                        {formatDate(s.created_at)}
                      </span>
                    </td>
                    <td>
                      <span
                        style={{
                          color: 'var(--white-dim)',
                          fontSize: 12,
                          fontFamily: 'var(--font-mono)',
                        }}
                      >
                        {s.source || '—'}
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
