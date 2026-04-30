// SmartCaptcha — thin React wrapper around Yandex SmartCaptcha's manual
// render API. The script tag in index.html drops `window.smartCaptcha`
// onto the page asynchronously, so we poll for it (max ~5 s) before
// calling `render`. Auto-mount mode (looking for `.smart-captcha` divs)
// is unreliable for SPAs that mount widgets after the initial HTML
// scan — manual `render` is the supported path for React.
//
// Props:
//   - siteKey       — public site key from Yandex Cloud. Empty → render
//                     nothing (the parent decides when captcha is
//                     soft-disabled).
//   - onToken(t)    — fired with the token string when the user passes
//                     the challenge. Captured via a ref so we don't
//                     re-render the widget when the parent's handler
//                     identity changes.
//   - onExpire?()   — optional, fired when the widget detects its own
//                     token expired (Yandex's `expired-callback`).
//   - resetSignal   — any value (number/timestamp). When it changes the
//                     widget is reset and the parent's stored token is
//                     cleared via onToken('').
//
// Reserved height of 100px on the container before render avoids the
// layout shift Yandex's docs warn about. If the script itself fails to
// load (network, ad blocker) we fall through to an inline notice and
// the submit will be rejected server-side once captcha is enabled.

import { useEffect, useRef, useState } from 'react';

const POLL_INTERVAL_MS = 100;
const POLL_MAX_ATTEMPTS = 50; // 50 * 100 ms = 5 s

export default function SmartCaptcha({
  siteKey,
  onToken,
  onExpire,
  resetSignal,
}) {
  const containerRef = useRef(null);
  const widgetIdRef = useRef(null);
  // Stable refs so the widget callback always sees the latest handler
  // without forcing a re-render of the widget itself.
  const onTokenRef = useRef(onToken);
  const onExpireRef = useRef(onExpire);
  const [scriptFailed, setScriptFailed] = useState(false);

  useEffect(() => { onTokenRef.current = onToken; }, [onToken]);
  useEffect(() => { onExpireRef.current = onExpire; }, [onExpire]);

  // Mount the widget once the script becomes available.
  useEffect(() => {
    if (!siteKey) return undefined;
    if (!containerRef.current) return undefined;

    let cancelled = false;
    let attempts = 0;
    let timer = null;

    const tryMount = () => {
      if (cancelled) return;
      const sc = typeof window !== 'undefined' ? window.smartCaptcha : null;
      if (sc && typeof sc.render === 'function') {
        try {
          const id = sc.render(containerRef.current, {
            sitekey: siteKey,
            hl: 'ru',
            callback: (token) => {
              if (typeof onTokenRef.current === 'function') {
                onTokenRef.current(token || '');
              }
            },
            'expired-callback': () => {
              if (typeof onTokenRef.current === 'function') {
                onTokenRef.current('');
              }
              if (typeof onExpireRef.current === 'function') {
                onExpireRef.current();
              }
            },
          });
          widgetIdRef.current = id;
        } catch {
          // Render failure is treated the same as the script never
          // loading — surface the inline notice and let the server
          // side reject if captcha is enforced.
          setScriptFailed(true);
        }
        return;
      }
      attempts += 1;
      if (attempts >= POLL_MAX_ATTEMPTS) {
        setScriptFailed(true);
        return;
      }
      timer = setTimeout(tryMount, POLL_INTERVAL_MS);
    };

    tryMount();

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      // Yandex SmartCaptcha doesn't currently expose a `destroy` —
      // letting the DOM node unmount is sufficient. We just drop our
      // widget id reference so a later resetSignal doesn't try to
      // reset a stale id.
      widgetIdRef.current = null;
    };
  }, [siteKey]);

  // React to resetSignal changes by resetting the widget and clearing
  // the parent's token. We deliberately skip the very first render
  // (resetSignal hasn't "changed" yet — that's its initial value).
  const lastResetRef = useRef(resetSignal);
  useEffect(() => {
    if (resetSignal === lastResetRef.current) return;
    lastResetRef.current = resetSignal;
    const sc = typeof window !== 'undefined' ? window.smartCaptcha : null;
    if (sc && typeof sc.reset === 'function' && widgetIdRef.current != null) {
      try { sc.reset(widgetIdRef.current); } catch { /* ignore */ }
    }
    if (typeof onTokenRef.current === 'function') {
      onTokenRef.current('');
    }
  }, [resetSignal]);

  if (!siteKey) return null;

  return (
    <div className="fw-captcha">
      <div
        ref={containerRef}
        className="fw-captcha-host"
        style={{ minHeight: 100 }}
      />
      {scriptFailed && (
        <div className="sf-hint" style={{ marginTop: 8 }}>
          Не удалось загрузить проверку безопасности. Перезагрузите страницу.
        </div>
      )}
    </div>
  );
}
