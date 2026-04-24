import { useRef, useState } from 'react';

// Character-per-box text input with unlimited length. Renders a row of small
// boxes mirroring the current value; a transparent <input> on top absorbs
// keystrokes/paste. Use for fields where the user wants per-char visual
// feedback (flight numbers, passport issuing authority codes, etc.).
//
// Props:
//   value, onChange — controlled value
//   sanitize        — (s) => s, applied to every input before onChange
//   uppercase       — shorthand that wraps sanitize with .toUpperCase()
//   ariaLabel, className — passthrough
export default function BoxedCharInput({
  value = '',
  onChange,
  sanitize = (s) => s,
  uppercase = false,
  ariaLabel,
  className = '',
}) {
  const inputRef = useRef(null);
  const [focused, setFocused] = useState(false);

  const apply = (s) => {
    const out = uppercase ? s.toUpperCase() : s;
    return sanitize(out);
  };

  const handleChange = (e) => onChange(apply(e.target.value));

  const chars = value.split('');

  return (
    <div
      className={`boxed-char-input${focused ? ' is-focused' : ''}${className ? ` ${className}` : ''}`}
      onClick={() => inputRef.current?.focus()}
    >
      <input
        ref={inputRef}
        type="text"
        className="bci-hidden-input"
        value={value}
        onChange={handleChange}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        aria-label={ariaLabel}
        autoComplete="off"
        spellCheck={false}
      />
      <div className="bci-display" aria-hidden="true">
        {chars.map((c, i) =>
          c === ' '
            ? <span key={i} className="bci-gap" />
            : <span key={i} className="bci-box">{c}</span>,
        )}
        <span className="bci-box bci-caret" />
      </div>
    </div>
  );
}
