import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { getGroups, createGroup } from '../api/client';
import StatusBadge from '../components/StatusBadge';
import Modal from '../components/Modal';

function formatDate(iso) {
  if (!iso) return '—';
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: '2-digit', month: 'short', year: 'numeric',
  });
}

export default function GroupsPage() {
  const navigate = useNavigate();
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [podachaName, setPodachaName] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState(null);

  const load = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getGroups();
      setGroups(Array.isArray(data) ? data : []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleCreate = async (e) => {
    e.preventDefault();
    if (!podachaName.trim()) return;
    try {
      setCreating(true);
      setCreateError(null);
      const group = await createGroup(podachaName.trim());
      setModalOpen(false);
      setPodachaName('');
      navigate(`/groups/${group.id}`);
    } catch (e) {
      setCreateError(e.message);
    } finally {
      setCreating(false);
    }
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setPodachaName('');
    setCreateError(null);
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Подачи</div>
          <div className="page-subtitle">Управление туристическими подачами</div>
        </div>
        <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
          <span>+</span> Новая подача
        </button>
      </div>

      {error && <div className="error-message">{error}</div>}

      {loading ? (
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          <span>Загрузка...</span>
        </div>
      ) : groups.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="empty-state-icon">◫</div>
            <div className="empty-state-title">Нет подач</div>
            <div className="empty-state-text">
              Создайте первую подачу, нажав кнопку "Новая подача"
            </div>
          </div>
        </div>
      ) : (
        <div className="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>Название</th>
                <th>Статус</th>
                <th>Создана</th>
              </tr>
            </thead>
            <tbody>
              {groups.map(g => (
                <tr
                  key={g.id}
                  className="clickable"
                  onClick={() => navigate(`/groups/${g.id}`)}
                >
                  <td>
                    <span style={{ fontWeight: 500 }}>{g.name}</span>
                  </td>
                  <td>
                    <StatusBadge status={g.status || 'draft'} />
                  </td>
                  <td>
                    <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
                      {formatDate(g.created_at)}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={modalOpen} onClose={handleModalClose} title="Создать подачу" width={440}>
        <form onSubmit={handleCreate}>
          {createError && <div className="error-message" style={{ marginBottom: 14 }}>{createError}</div>}
          <div className="form-group">
            <label className="form-label">Название подачи</label>
            <input
              className="form-input"
              type="text"
              placeholder="напр. Япония — Май 2026"
              value={podachaName}
              onChange={e => setPodachaName(e.target.value)}
              autoFocus
            />
          </div>
          <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 8 }}>
            <button type="button" className="btn btn-ghost" onClick={handleModalClose}>
              Отмена
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={creating || !podachaName.trim()}
            >
              {creating ? <><span className="spinner" /> Создание...</> : 'Создать'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
