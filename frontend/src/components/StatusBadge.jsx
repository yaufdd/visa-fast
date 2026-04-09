// Canonical group statuses (new 5) + backward-compatible tourist badges.
export const GROUP_STATUSES = [
  { id: 'draft',       label: 'Черновик',         color: '#6b7280' },
  { id: 'in_progress', label: 'В процессе',       color: '#f59e0b' },
  { id: 'docs_ready',  label: 'Документы готовы', color: '#3b82f6' },
  { id: 'submitted',   label: 'Подан',            color: '#10b981' },
  { id: 'visa_issued', label: 'Виза получена',    color: '#22c55e' },
];

const configs = {
  // Group statuses
  draft:       { label: 'Черновик',         color: '#6b7280' },
  in_progress: { label: 'В процессе',       color: '#f59e0b' },
  docs_ready:  { label: 'Документы готовы', color: '#3b82f6' },
  submitted:   { label: 'Подан',            color: '#10b981' },
  visa_issued: { label: 'Виза получена',    color: '#22c55e' },

  // Legacy tourist/row badges (kept for TouristRow compatibility)
  processing: { label: 'Processing', color: '#f59e0b' },
  ready:      { label: 'Ready',      color: '#22c55e' },
  completed:  { label: 'Completed',  color: '#3b82f6' },
  confirmed:  { label: 'Confirmed',  color: '#22c55e' },
  unmatched:  { label: 'Unmatched',  color: '#ef4444' },
  parsed:     { label: 'Parsed',     color: '#a78bfa' },
  pending:    { label: 'Pending',    color: '#6b7280' },
};

function bgFor(color) {
  // Turn hex to rgba 15% tint
  const m = /^#([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(color);
  if (!m) return `${color}26`;
  const r = parseInt(m[1], 16), g = parseInt(m[2], 16), b = parseInt(m[3], 16);
  return `rgba(${r},${g},${b},0.15)`;
}

export default function StatusBadge({ status }) {
  const cfg = configs[status] || configs.pending;
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '6px',
        padding: '3px 10px',
        borderRadius: '100px',
        fontSize: '11px',
        fontWeight: 600,
        letterSpacing: '0.02em',
        color: cfg.color,
        background: bgFor(cfg.color),
        border: `1px solid ${cfg.color}33`,
      }}
    >
      <span style={{
        width: 6,
        height: 6,
        borderRadius: '50%',
        background: cfg.color,
        flexShrink: 0,
      }} />
      {cfg.label}
    </span>
  );
}
