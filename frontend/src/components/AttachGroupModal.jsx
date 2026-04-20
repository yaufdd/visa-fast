import { useEffect, useState } from 'react';
import Modal from './Modal';
import { getGroups, getSubgroups } from '../api/client';

export default function AttachGroupModal({ open, onClose, onConfirm }) {
  const [groups, setGroups] = useState([]);
  const [groupId, setGroupId] = useState('');
  const [subgroups, setSubgroups] = useState([]);
  const [subgroupId, setSubgroupId] = useState('');
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setLoading(true);
    getGroups()
      .then((data) => setGroups(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [open]);

  useEffect(() => {
    if (!groupId) {
      setSubgroups([]);
      setSubgroupId('');
      return;
    }
    getSubgroups(groupId)
      .then((data) => setSubgroups(Array.isArray(data) ? data : []))
      .catch(() => setSubgroups([]));
    setSubgroupId('');
  }, [groupId]);

  const reset = () => {
    setGroupId('');
    setSubgroupId('');
    setSubgroups([]);
    setError(null);
  };

  const handleClose = () => {
    if (submitting) return;
    reset();
    onClose();
  };

  const handleConfirm = async () => {
    if (!groupId) return;
    setSubmitting(true);
    setError(null);
    try {
      await onConfirm(groupId, subgroupId || null);
      reset();
    } catch (e) {
      setError(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal open={open} onClose={handleClose} title="Привязать к группе" width={480}>
      {error && <div className="error-message">{error}</div>}
      {loading ? (
        <div className="loading-center" style={{ padding: 32 }}>
          <div className="spinner" />
          <span>Загрузка...</span>
        </div>
      ) : (
        <>
          <div className="form-group">
            <label className="form-label">Подача (группа)</label>
            <select
              className="form-input"
              value={groupId}
              onChange={(e) => setGroupId(e.target.value)}
            >
              <option value="">— выберите подачу —</option>
              {groups.map((g) => (
                <option key={g.id} value={g.id}>{g.name}</option>
              ))}
            </select>
          </div>

          {groupId && subgroups.length > 0 && (
            <div className="form-group">
              <label className="form-label">Подгруппа (необязательно)</label>
              <select
                className="form-input"
                value={subgroupId}
                onChange={(e) => setSubgroupId(e.target.value)}
              >
                <option value="">— без подгруппы —</option>
                {subgroups.map((sg) => (
                  <option key={sg.id} value={sg.id}>{sg.name}</option>
                ))}
              </select>
            </div>
          )}

          <div
            style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 16 }}
          >
            <button
              type="button"
              className="btn btn-ghost"
              onClick={handleClose}
              disabled={submitting}
            >
              Отмена
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={handleConfirm}
              disabled={!groupId || submitting}
            >
              {submitting ? (
                <><span className="spinner" /> Привязка...</>
              ) : (
                'Привязать'
              )}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
