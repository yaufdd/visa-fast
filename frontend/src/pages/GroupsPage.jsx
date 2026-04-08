import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { getGroups, createGroup, deleteGroup } from '../api/client';
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
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState(null);

  const handleDelete = async (e, id) => {
    e.stopPropagation();
    if (!window.confirm('Удалить группу? Все данные будут удалены безвозвратно.')) return;
    try {
      await deleteGroup(id);
      await load();
    } catch (e) {
      alert(e.message);
    }
  };

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
    if (!newName.trim()) return;
    try {
      setCreating(true);
      setCreateError(null);
      const group = await createGroup(newName.trim());
      setModalOpen(false);
      setNewName('');
      navigate(`/groups/${group.id}`);
    } catch (e) {
      setCreateError(e.message);
    } finally {
      setCreating(false);
    }
  };

  const handleModalClose = () => {
    setModalOpen(false);
    setNewName('');
    setCreateError(null);
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Группы</div>
          <div className="page-subtitle">Управление туристическими группами</div>
        </div>
        <button className="btn btn-primary" onClick={() => setModalOpen(true)}>
          <span>+</span> Новая группа
        </button>
      </div>

      {error && <div className="error-message">{error}</div>}

      {loading ? (
        <div className="loading-center">
          <div className="spinner spinner-lg" />
          <span>Загрузка групп...</span>
        </div>
      ) : groups.length === 0 ? (
        <div className="card">
          <div className="empty-state">
            <div className="empty-state-icon">◫</div>
            <div className="empty-state-title">Нет групп</div>
            <div className="empty-state-text">
              Создайте первую группу, нажав кнопку "Новая группа"
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
                <th>Туристов</th>
                <th>Создана</th>
                <th></th>
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
                    <span style={{ color: 'var(--white-dim)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
                      {g.tourist_count ?? '—'}
                    </span>
                  </td>
                  <td>
                    <span style={{ color: 'var(--white-dim)', fontSize: 12 }}>
                      {formatDate(g.created_at)}
                    </span>
                  </td>
                  <td onClick={e => e.stopPropagation()}>
                    <button
                      className="btn btn-danger btn-sm"
                      onClick={e => handleDelete(e, g.id)}
                    >
                      Удалить
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={modalOpen} onClose={handleModalClose} title="Создать группу" width={440}>
        <form onSubmit={handleCreate}>
          {createError && <div className="error-message">{createError}</div>}
          <div className="form-group">
            <label className="form-label">Название группы</label>
            <input
              className="form-input"
              type="text"
              placeholder="напр. Японский тур — Апрель 2026"
              value={newName}
              onChange={e => setNewName(e.target.value)}
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
              disabled={creating || !newName.trim()}
            >
              {creating ? <><span className="spinner" /> Создание...</> : 'Создать'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
