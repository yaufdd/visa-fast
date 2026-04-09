import { useEffect, useState, useMemo } from 'react';

// A pure-CSS/JS celebration overlay — no dependencies.
// Renders a big fullscreen layer with confetti pieces that fall + a title card,
// self-dismisses after ~3.5 s.

const COLORS = ['#22c55e', '#3b82f6', '#f59e0b', '#ef4444', '#a78bfa', '#ec4899', '#14b8a6', '#f97316'];

function randomBetween(a, b) {
  return a + Math.random() * (b - a);
}

export default function Celebration({ trigger, onDone, message = '🎉 Виза получена!' }) {
  const [visible, setVisible] = useState(false);

  // Regenerate particles on each trigger.
  const particles = useMemo(() => {
    if (trigger === 0) return [];
    return Array.from({ length: 120 }, (_, i) => ({
      id: `${trigger}-${i}`,
      left: randomBetween(0, 100),        // vw %
      color: COLORS[Math.floor(Math.random() * COLORS.length)],
      size: randomBetween(6, 12),
      delay: randomBetween(0, 0.6),
      duration: randomBetween(2.4, 3.8),
      drift: randomBetween(-80, 80),      // px horizontal drift
      rotate: randomBetween(-720, 720),   // deg
      shape: Math.random() > 0.5 ? 'square' : 'circle',
    }));
  }, [trigger]);

  useEffect(() => {
    if (trigger === 0) return;
    setVisible(true);
    const t = setTimeout(() => {
      setVisible(false);
      onDone?.();
    }, 3600);
    return () => clearTimeout(t);
  }, [trigger, onDone]);

  if (!visible) return null;

  return (
    <>
      <style>{`
        @keyframes fujifall {
          0%   { transform: translate3d(0, -20vh, 0) rotate(0deg); opacity: 1; }
          80%  { opacity: 1; }
          100% { transform: translate3d(var(--drift, 0), 110vh, 0) rotate(var(--rot, 720deg)); opacity: 0.2; }
        }
        @keyframes fujipop {
          0%   { transform: scale(0.4) translateY(20px); opacity: 0; }
          30%  { transform: scale(1.1) translateY(-4px); opacity: 1; }
          55%  { transform: scale(1) translateY(0); opacity: 1; }
          85%  { transform: scale(1) translateY(0); opacity: 1; }
          100% { transform: scale(0.96) translateY(-8px); opacity: 0; }
        }
      `}</style>

      <div
        style={{
          position: 'fixed',
          inset: 0,
          pointerEvents: 'none',
          zIndex: 9999,
          overflow: 'hidden',
        }}
      >
        {/* Confetti layer */}
        {particles.map(p => (
          <span
            key={p.id}
            style={{
              position: 'absolute',
              top: 0,
              left: `${p.left}vw`,
              width: p.size,
              height: p.size,
              background: p.color,
              borderRadius: p.shape === 'circle' ? '50%' : 2,
              boxShadow: `0 0 6px ${p.color}80`,
              animation: `fujifall ${p.duration}s cubic-bezier(0.25, 0.46, 0.45, 0.94) ${p.delay}s forwards`,
              ['--drift']: `${p.drift}px`,
              ['--rot']: `${p.rotate}deg`,
            }}
          />
        ))}

        {/* Pop-in title card */}
        <div
          style={{
            position: 'absolute',
            top: '28%',
            left: '50%',
            transform: 'translateX(-50%)',
            padding: '18px 34px',
            borderRadius: 14,
            background: 'linear-gradient(135deg, rgba(34,197,94,0.18), rgba(59,130,246,0.18))',
            backdropFilter: 'blur(14px)',
            WebkitBackdropFilter: 'blur(14px)',
            border: '1px solid rgba(34,197,94,0.45)',
            boxShadow: '0 20px 60px rgba(0,0,0,0.5), 0 0 40px rgba(34,197,94,0.25)',
            animation: 'fujipop 3.4s ease forwards',
            fontSize: 24,
            fontWeight: 700,
            color: '#fff',
            letterSpacing: '0.02em',
            whiteSpace: 'nowrap',
            textShadow: '0 2px 12px rgba(0,0,0,0.4)',
          }}
        >
          {message}
        </div>
      </div>
    </>
  );
}
