(() => {
  const key = 'nfcx-theme';
  const root = document.documentElement;
  const button = document.querySelector('[data-theme-toggle]');
  const label = document.querySelector('[data-theme-label]');
  const chinese = root.lang.toLowerCase().startsWith('zh');

  function setButton(theme) {
    const light = theme === 'light';
    button.querySelector('[aria-hidden]').textContent = light ? '☾' : '☀';
    label.textContent = light ? (chinese ? '深色' : 'Dark') : (chinese ? '浅色' : 'Light');
    button.setAttribute('aria-label', light ? (chinese ? '切换至深色模式' : 'Switch to dark mode') : (chinese ? '切换至浅色模式' : 'Switch to light mode'));
  }

  if (!button || !label) return;
  setButton(root.dataset.theme || 'dark');
  button.addEventListener('click', () => {
    const theme = root.dataset.theme === 'light' ? 'dark' : 'light';
    root.dataset.theme = theme;
    try { localStorage.setItem(key, theme); } catch (e) {}
    setButton(theme);
  });
})();
