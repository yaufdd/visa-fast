import { useState, useEffect, useMemo } from 'react';
import { getGroupAILogs } from '../api/client';

// Format a timestamp as "DD.MM.YYYY HH:MM:SS" — local timezone, no seconds dropped.
function fmtTs(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n) => String(n).padStart(2, '0');
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

const FN_LABEL = {
  translate: 'Перевод свободного текста',
  programme: 'Программа тура',
  ticket_parser: 'Парсинг билета',
  voucher_parser: 'Парсинг ваучера',
};

function StatusPill({ status }) {
  const bg = status === 'success' ? '#1b4332' : '#5a1e1e';
  const color = status === 'success' ? '#9bd19c' : '#d19b9b';
  return (
    <span style={{
      fontSize: 11, padding: '2px 8px', borderRadius: 10,
      background: bg, color, fontWeight: 600, textTransform: 'uppercase',
    }}>{status}</span>
  );
}

function prettyJSON(v) {
  try {
    return JSON.stringify(typeof v === 'string' ? JSON.parse(v) : v, null, 2);
  } catch {
    return typeof v === 'string' ? v : JSON.stringify(v);
  }
}

function LogRow({ log }) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{
      border: '1px solid #2a2a2a', borderRadius: 8, padding: 12,
      marginBottom: 8, background: '#1a1a1a',
    }}>
      <div
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'flex', alignItems: 'center', gap: 12,
          cursor: 'pointer', userSelect: 'none',
        }}
      >
        <span style={{ color: '#888', fontSize: 11, minWidth: 14 }}>{open ? '▼' : '▶'}</span>
        <StatusPill status={log.status} />
        <span style={{ color: '#e0e0e0', fontWeight: 500, flex: 1 }}>
          {FN_LABEL[log.function_name] || log.function_name}
        </span>
        <span style={{ color: '#888', fontSize: 12 }}>{log.model}</span>
        <span style={{ color: '#888', fontSize: 12 }}>{log.duration_ms} ms</span>
        <span style={{ color: '#666', fontSize: 12 }}>{fmtTs(log.started_at)}</span>
      </div>

      {open && (
        <div style={{ marginTop: 12, paddingLeft: 26 }}>
          {log.error_msg && (
            <div style={{ marginBottom: 10 }}>
              <div style={{ color: '#d19b9b', fontSize: 12, marginBottom: 4 }}>Ошибка:</div>
              <div style={{
                background: '#2a1a1a', padding: 8, borderRadius: 4,
                fontFamily: 'monospace', fontSize: 12, whiteSpace: 'pre-wrap',
                color: '#e8b4b4',
              }}>{log.error_msg}</div>
            </div>
          )}
          <div style={{ marginBottom: 10 }}>
            <div style={{ color: '#999', fontSize: 12, marginBottom: 4 }}>Что ушло в Claude (request):</div>
            <pre style={{
              background: '#0f0f0f', padding: 10, borderRadius: 4, margin: 0,
              fontSize: 11, maxHeight: 320, overflow: 'auto',
              color: '#c8c8c8', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
            }}>{prettyJSON(log.request_json)}</pre>
          </div>
          {log.response_text && (
            <div>
              <div style={{ color: '#999', fontSize: 12, marginBottom: 4 }}>Что вернул Claude (response):</div>
              <pre style={{
                background: '#0f0f0f', padding: 10, borderRadius: 4, margin: 0,
                fontSize: 11, maxHeight: 320, overflow: 'auto',
                color: '#c8c8c8', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              }}>{prettyJSON(log.response_text)}</pre>
            </div>
          )}
          <div style={{ marginTop: 10, color: '#555', fontSize: 11 }}>
            generation_id: <code>{log.generation_id}</code>
          </div>
        </div>
      )}
    </div>
  );
}

// GroupAILogsSection — expandable section under the Documents tab showing
// every Claude API call made for this group, grouped by generation_id.
export default function AILogsSection({ groupId }) {
  const [open, setOpen] = useState(false);
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError('');
      try {
        const data = await getGroupAILogs(groupId);
        if (!cancelled) setLogs(Array.isArray(data) ? data : []);
      } catch (e) {
        if (!cancelled) setError(e.message || 'Не удалось загрузить лог');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [open, groupId]);

  // Group by generation_id (preserve the newest-first order from the backend).
  const generations = useMemo(() => {
    const map = new Map();
    for (const l of logs) {
      if (!map.has(l.generation_id)) map.set(l.generation_id, []);
      map.get(l.generation_id).push(l);
    }
    return Array.from(map.entries()); // [ [genId, [log, log, ...]], ... ]
  }, [logs]);

  return (
    <div style={{
      marginTop: 24, border: '1px solid #2a2a2a', borderRadius: 8,
      padding: 16, background: '#141414',
    }}>
      <div
        onClick={() => setOpen((v) => !v)}
        style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer', userSelect: 'none' }}
      >
        <span style={{ color: '#888' }}>{open ? '▼' : '▶'}</span>
        <span style={{ fontWeight: 600, color: '#e0e0e0' }}>
          Аудит-лог ИИ-вызовов
        </span>
        <span style={{ color: '#888', fontSize: 12 }}>
          (что именно ушло в Claude для этой группы)
        </span>
      </div>

      {open && (
        <div style={{ marginTop: 14 }}>
          {loading && <div style={{ color: '#888' }}>Загрузка…</div>}
          {error && <div className="error-message">{error}</div>}
          {!loading && !error && logs.length === 0 && (
            <div style={{ color: '#777', fontSize: 13 }}>
              Пока ни одного ИИ-вызова для этой группы.
            </div>
          )}
          {generations.map(([genId, group]) => (
            <div key={genId} style={{ marginBottom: 18 }}>
              <div style={{
                color: '#999', fontSize: 12, marginBottom: 8,
                paddingBottom: 6, borderBottom: '1px solid #2a2a2a',
              }}>
                Генерация {genId.slice(0, 8)} · {group.length} {group.length === 1 ? 'вызов' : 'вызова'}
                {group[0]?.started_at && <> · {fmtTs(group[0].started_at)}</>}
              </div>
              {group.map((log) => <LogRow key={log.id} log={log} />)}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
