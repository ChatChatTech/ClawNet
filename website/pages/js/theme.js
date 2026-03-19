/* ── Theme Switching ── */

const THEMES = {
  teal: {
    name: 'Teal',
    hex: {
      accent500: '#4fd1c5', accent400: '#38b2ac', accent300: '#2d9d8f',
      accent100: '#134e48', surface0: '#0c0c12'
    }
  },
  lobster: {
    name: 'Lobster',
    hex: {
      accent500: '#E63946', accent400: '#d63545', accent300: '#b0303c',
      accent100: '#4e1318', surface0: '#100a0b'
    }
  }
};

let currentTheme = localStorage.getItem('clawnet-theme') || 'teal';

function applyTheme(theme) {
  currentTheme = theme || currentTheme;
  localStorage.setItem('clawnet-theme', currentTheme);
  document.documentElement.setAttribute('data-theme', currentTheme);
  const btn = document.getElementById('theme-toggle-btn');
  if (btn) btn.textContent = currentTheme === 'teal' ? '🦞 Lobster' : '🌊 Teal';
}

function toggleTheme() {
  applyTheme(currentTheme === 'teal' ? 'lobster' : 'teal');
  const hash = location.hash.slice(1) || 'dashboard';
  if (hash === 'topology') loadTopology();
}

function themeHex(key) {
  return THEMES[currentTheme].hex[key];
}
