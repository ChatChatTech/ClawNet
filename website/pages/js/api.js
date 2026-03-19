/* ── API Client & Utility Functions ── */

const API = '';
let MY_PEER_ID = '';
let refreshTimers = [];

async function api(path, opts = {}) {
  try {
    const res = await fetch(API + path, {
      headers: { 'Content-Type': 'application/json', ...opts.headers },
      ...opts
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || res.statusText);
    }
    return res.json();
  } catch (e) {
    if (e.message !== 'Failed to fetch') toast(e.message, 'error');
    throw e;
  }
}

const get = (p) => api(p);
const post = (p, body) => api(p, { method: 'POST', body: JSON.stringify(body) });
const put = (p, body) => api(p, { method: 'PUT', body: JSON.stringify(body) });
const del = (p) => api(p, { method: 'DELETE' });

function short(id, n = 8) {
  return id ? id.slice(0, n) + '...' : '--';
}

function timeAgo(ts) {
  if (!ts) return '';
  const d = new Date(ts);
  if (isNaN(d)) return ts;
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return t('time.just_now');
  if (s < 3600) return Math.floor(s / 60) + t('time.m_ago');
  if (s < 86400) return Math.floor(s / 3600) + t('time.h_ago');
  return Math.floor(s / 86400) + t('time.d_ago');
}

function statusBadge(s) {
  return `<span class="badge status-${s || 'open'}">${s || 'unknown'}</span>`;
}

function escHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function loading() {
  return '<div class="flex justify-center py-12"><span class="loading-spinner"></span></div>';
}

function toast(msg, type = 'info') {
  const c = document.getElementById('toast-container');
  const colors = {
    info: 'bg-surface-3/90 border border-surface-4',
    success: 'bg-emerald-900/80 border border-emerald-700/30',
    error: 'bg-red-900/80 border border-red-700/30',
    warning: 'bg-amber-900/80 border border-amber-700/30'
  };
  const el = document.createElement('div');
  el.className = `toast ${colors[type] || colors.info} text-gray-200 px-4 py-2.5 rounded-lg shadow-xl text-sm max-w-xs`;
  el.textContent = msg;
  c.appendChild(el);
  setTimeout(() => {
    el.style.opacity = '0';
    el.style.transition = 'opacity .3s';
    setTimeout(() => el.remove(), 300);
  }, 3000);
}
