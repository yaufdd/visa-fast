// formatAIError — turns a raw error from a Yandex-backed API call into a
// user-friendly Russian hint. Yandex Cloud only accepts traffic from RU
// networks, so VPN / foreign-IP users typically get connection failures or
// 403/451 status codes. The detector is broad on purpose: it's better to
// suggest disabling a VPN once unnecessarily than to show a confusing
// "yandex: gpt status 403" message.
//
// Use ONLY at AI-call sites (document generation, scan parsing). For plain
// CRUD errors keep the original message — the VPN hint would be misleading.

const AI_FAILURE_PATTERNS = [
  /yandex/i,
  /fetch failed|network|timeout|connection|tls|socket/i,
  /\b(403|429|451|502|503|504)\b/,
];

export function formatAIError(err) {
  const msg = (err && (err.message || String(err))) || '';
  if (!msg) return 'Неизвестная ошибка. Попробуйте ещё раз.';
  if (AI_FAILURE_PATTERNS.some(rx => rx.test(msg))) {
    return 'Не получилось. Попробуйте ещё раз. Если включён VPN — попробуйте отключить.';
  }
  return msg;
}
