import { Fragment, useLayoutEffect, useRef, useState } from 'react';

export default function BoxedCharInput({
  value = '',
  onChange,
  sanitize = (s) => s,
  uppercase = false,
  ariaLabel,
  className = '',
}) {
  const inputRef = useRef(null);
  const pendingCaret = useRef(null);
  const [focused, setFocused] = useState(false);
  const [caret, setCaret] = useState(value.length);

  const apply = (s) => sanitize(uppercase ? s.toUpperCase() : s);

  const handleChange = (e) => {
    const raw = e.target.value;
    const rawCaret = e.target.selectionStart ?? raw.length;
    const cleanedCaret = apply(raw.slice(0, rawCaret)).length;
    pendingCaret.current = cleanedCaret;
    setCaret(cleanedCaret);
    onChange(apply(raw));
  };

  const handleSelect = (e) => {
    setCaret(e.target.selectionStart ?? 0);
  };

  useLayoutEffect(() => {
    if (pendingCaret.current !== null && inputRef.current) {
      const pos = Math.min(pendingCaret.current, value.length);
      inputRef.current.setSelectionRange(pos, pos);
      pendingCaret.current = null;
    }
  }, [value]);

  const chars = value.split('');
  const caretAt = Math.min(caret, chars.length);

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
        onSelect={handleSelect}
        onKeyUp={handleSelect}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        aria-label={ariaLabel}
        autoComplete="off"
        spellCheck={false}
      />
      <div className="bci-display" aria-hidden="true">
        {chars.map((c, i) => (
          <Fragment key={i}>
            {focused && i === caretAt && <span className="bci-box bci-caret" />}
            {c === ' '
              ? <span className="bci-gap" />
              : <span className="bci-box">{c}</span>}
          </Fragment>
        ))}
        {focused && caretAt === chars.length && <span className="bci-box bci-caret" />}
      </div>
    </div>
  );
}
