import { useEffect, useState, useCallback } from 'react';
import { listSubmissions, attachSubmission } from '../api/client';

// Group statuses where the previous visa case is considered finalized —
// the submission can then appear in the pool again as a fresh attach
// candidate (e.g. the same tourist starting another trip).
const POOL_ELIGIBLE_FINALIZED_STATUSES = new Set(['submitted', 'visa_issued']);

const GROUP_STATUS_RU = {
  draft: 'черновик',
  docs_ready: 'документы готовы',
  submitted: 'подано',
  visa_issued: 'виза выдана',
};

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
    // Fetch pending + attached in one call; we filter client-side based on
    // the latest attachment's group status so the manager only sees
    // submissions that are actually available for a new attach.
    listSubmissions(query, 'pending,attached')
      .then((data) => {
        const list = Array.isArray(data) ? data : [];
        setRows(list.filter((r) => {
          if (r.status === 'pending') return true;
          // Attached: show only when the latest group is finalized. Also
          // exclude the current group — re-attaching there is a no-op.
          if (r.status === 'attached') {
            if (r.current_group_id === groupId) return false;
            return POOL_ELIGIBLE_FINALIZED_STATUSES.has(r.current_group_status || '');
          }
          return false;
        }));
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [groupId]);

  useEffect(() => {
    const handle = setTimeout(() => load(q), 250);
    return () => clearTimeout(handle);
  }, [q, load]);

  const attach = async (row) => {
    setBusyId(row.id);
    setError(null);
    try {
      await attachSubmission(row.id, groupId, subgroupId);
      // Remove locally so the list updates without a refetch.
      setRows((prev) => prev.filter((r) => r.id !== row.id));
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
            const previousTrip = r.status === 'attached';

            return (
              <li key={r.id}>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="submission-list-name">{name}</div>
                  {passport && (
                    <div className="submission-list-sub">паспорт {passport}</div>
                  )}
                  {previousTrip && (
                    <div
                      className="submission-list-sub"
                      style={{ marginTop: 3, color: 'var(--white-dim)' }}
                    >
                      предыдущая поездка:
                      {' '}«{r.current_group_name || '—'}»
                      {r.current_group_status && (
                        <> · {GROUP_STATUS_RU[r.current_group_status] || r.current_group_status}</>
                      )}
                    </div>
                  )}
                </div>
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={() => attach(r)}
                  disabled={isBusy}
                >
                  {isBusy
                    ? <><span className="spinner" /> Добавляем...</>
                    : 'Добавить'}
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
