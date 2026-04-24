import { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { listSubmissions, eraseSubmission } from '../api/client';
import ConfirmModal from '../components/ConfirmModal';

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
  const [deletingId, setDeletingId] = useState(null);
  const [confirmTarget, setConfirmTarget] = useState(null); // submission being deleted
  const [deleteError, setDeleteError] = useState(null);
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

  const requestDelete = (e, s) => {
    e.stopPropagation();
    setDeleteError(null);
    setConfirmTarget(s);
  };

  const confirmDelete = async () => {
    const s = confirmTarget;
    if (!s) return;
    setDeletingId(s.id);
    setDeleteError(null);
    try {
      await eraseSubmission(s.id);
      setConfirmTarget(null);
      await load(q, statusFilter);
    } catch (err) {
      setDeleteError(err.message);
    } finally {
      setDeletingId(null);
    }
  };

  const handleEdit = (e, s) => {
    e.stopPropagation();
    navigate(`/submissions/${s.id}`);
  };

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
                <th style={{ width: 1, whiteSpace: 'nowrap' }}></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const badgeClass = `submission-status submission-status--${s.status || 'pending'}`;
                const isDeleting = deletingId === s.id;
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
                    <td style={{ whiteSpace: 'nowrap' }}>
                      <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                        <button
                          type="button"
                          className="btn btn-ghost btn-sm"
                          onClick={(e) => handleEdit(e, s)}
                          disabled={isDeleting}
                          title="Редактировать"
                        >
                          Редактировать
                        </button>
                        <button
                          type="button"
                          className="btn btn-ghost btn-sm"
                          onClick={(e) => requestDelete(e, s)}
                          disabled={isDeleting}
                          title="Удалить навсегда"
                          style={{
                            color: isDeleting ? 'var(--white-dim)' : 'var(--danger, #ff6b6b)',
                            opacity: isDeleting ? 0.5 : 1,
                          }}
                        >
                          {isDeleting ? <span className="spinner" /> : 'Удалить'}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmModal
        open={!!confirmTarget}
        title="Удалить анкету?"
        message={confirmTarget
          ? `Удалить анкету «${submissionName(confirmTarget)}» навсегда? Это нельзя отменить.`
          : ''}
        confirmText="Удалить"
        cancelText="Отмена"
        variant="danger"
        busy={!!deletingId}
        error={deleteError}
        onConfirm={confirmDelete}
        onCancel={() => { setConfirmTarget(null); setDeleteError(null); }}
      />
    </div>
  );
}
