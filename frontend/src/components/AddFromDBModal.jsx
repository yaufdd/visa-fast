import { useEffect, useState, useCallback } from 'react';
import { listSubmissions, attachSubmission } from '../api/client';

function safeParsePayload(raw) {
  if (!raw) return {};
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return {}; }
}

export default function AddFromDBModal({ groupId, subgroupId, onClose, onAdded }) {
  const [rows, setRows] = useState([]);
  const [q, setQ] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [busyId, setBusyId] = useState(null);

  const load = useCallback((query) => {
    setLoading(true);
    setError(null);
    listSubmissions(query, 'pending')
      .then((data) => setRows(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const handle = setTimeout(() => load(q), 250);
    return () => clearTimeout(handle);
  }, [q, load]);

  const attach = async (id) => {
    setBusyId(id);
    setError(null);
    try {
      await attachSubmission(id, groupId, subgroupId);
      // Remove locally so the list updates without a refetch.
      setRows((prev) => prev.filter((r) => r.id !== id));
      onAdded();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div>
      <div className="form-group" style={{ marginBottom: 12 }}>
        <input
          className="form-input"
          autoFocus
          placeholder="Поиск по имени (латиницей)..."
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>

      {error && <div className="error-message">{error}</div>}

      {loading ? (
        <div className="loading-center" style={{ padding: 24 }}>
          <div className="spinner" />
          <span style={{ fontSize: 13, color: 'var(--white-dim)' }}>Загрузка...</span>
        </div>
      ) : (
        <ul className="submission-list">
          {rows.map((r) => {
            const p = safeParsePayload(r.payload);
            const name = p.name_lat || p.name_cyr || '—';
            const passport = p.passport_number || '';
            const isBusy = busyId === r.id;
            return (
              <li key={r.id}>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="submission-list-name">{name}</div>
                  {passport && (
                    <div className="submission-list-sub">паспорт {passport}</div>
                  )}
                </div>
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={() => attach(r.id)}
                  disabled={isBusy}
                >
                  {isBusy ? (
                    <><span className="spinner" /> Добавляем...</>
                  ) : (
                    'Добавить'
                  )}
                </button>
              </li>
            );
          })}
          {rows.length === 0 && (
            <li className="empty">Нет анкет в пуле</li>
          )}
        </ul>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          Закрыть
        </button>
      </div>
    </div>
  );
}
