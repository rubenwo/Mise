import { useEffect } from 'react';

// Presence signals for the backend's model warmer. Pages register the LLM
// features the user is likely to use right away (useIntent); the backend loads
// that model ahead of time if the GPU is free, keeps it loaded while the user
// stays active, and frees it once they leave.

const HEARTBEAT_MS = 60_000;
// Heartbeats stop after this long without input, so a tab left open on a
// recipe doesn't keep the GPU busy.
const ACTIVE_WINDOW_MS = 4 * 60_000;

const intents = new Map<string, number>(); // tag → number of mounted users
let lastInput = Date.now();
let lastSent = 0;
let lastSentKey = '';

function post(body: object, beacon = false) {
  const json = JSON.stringify(body);
  if (beacon && navigator.sendBeacon) {
    navigator.sendBeacon('/api/presence', new Blob([json], { type: 'application/json' }));
    return;
  }
  fetch('/api/presence', { method: 'POST', body: json, headers: { 'Content-Type': 'application/json' }, keepalive: true })
    .catch(() => {});
}

function report(force = false) {
  if (document.visibilityState !== 'visible') return;
  const tags = [...intents.keys()].sort();
  const key = tags.join(',');
  const now = Date.now();
  if (!force && key === lastSentKey && now - lastSent < HEARTBEAT_MS - 5_000) return;
  lastSent = now;
  lastSentKey = key;
  post({ active: true, intents: tags });
}

/** Starts presence reporting for the app; returns a cleanup function. */
export function startPresence(): () => void {
  const onInput = () => {
    lastInput = Date.now();
    report();
  };
  const onVisibility = () => {
    if (document.visibilityState === 'visible') {
      lastInput = Date.now();
      report(true);
    } else {
      post({ active: false }, true);
    }
  };
  const onLeave = () => post({ active: false }, true);
  const inputEvents = ['pointerdown', 'keydown', 'wheel', 'touchstart', 'scroll'] as const;
  for (const e of inputEvents) window.addEventListener(e, onInput, { passive: true, capture: true });
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('pagehide', onLeave);
  const timer = setInterval(() => {
    if (Date.now() - lastInput < ACTIVE_WINDOW_MS) report(true);
  }, HEARTBEAT_MS);
  report(true);
  return () => {
    for (const e of inputEvents) window.removeEventListener(e, onInput, { capture: true });
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('pagehide', onLeave);
    clearInterval(timer);
  };
}

/**
 * Marks an LLM feature (a provider tag such as "chat") as likely to be used
 * while the calling component is mounted and `enabled` is true.
 */
export function useIntent(tag: string, enabled = true) {
  useEffect(() => {
    if (!enabled) return;
    intents.set(tag, (intents.get(tag) ?? 0) + 1);
    lastInput = Date.now();
    report(true);
    return () => {
      const n = (intents.get(tag) ?? 1) - 1;
      if (n > 0) intents.set(tag, n); else intents.delete(tag);
    };
  }, [tag, enabled]);
}
