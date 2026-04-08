const configs = {
  draft:      { label: 'Draft',      color: '#6b7280', bg: 'rgba(107,114,128,0.15)' },
  processing: { label: 'Processing', color: '#f59e0b', bg: 'rgba(245,158,11,0.15)'  },
  ready:      { label: 'Ready',      color: '#22c55e', bg: 'rgba(34,197,94,0.15)'   },
  completed:  { label: 'Completed',  color: '#3b82f6', bg: 'rgba(59,130,246,0.15)'  },
  confirmed:  { label: 'Confirmed',  color: '#22c55e', bg: 'rgba(34,197,94,0.15)'   },
  unmatched:  { label: 'Unmatched',  color: '#ef4444', bg: 'rgba(239,68,68,0.15)'   },
  parsed:     { label: 'Parsed',     color: '#a78bfa', bg: 'rgba(167,139,250,0.15)' },
  pending:    { label: 'Pending',    color: '#6b7280', bg: 'rgba(107,114,128,0.15)' },
};

export default function StatusBadge({ status }) {
  const cfg = configs[status] || configs.pending;
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '5px',
        padding: '3px 9px',
        borderRadius: '100px',
        fontSize: '11px',
        fontWeight: 600,
        letterSpacing: '0.03em',
        color: cfg.color,
        background: cfg.bg,
        border: `1px solid ${cfg.color}33`,
        textTransform: 'uppercase',
      }}
    >
      <span style={{
        width: 5,
        height: 5,
        borderRadius: '50%',
        background: cfg.color,
        flexShrink: 0,
      }} />
      {cfg.label}
    </span>
  );
}
