(function() {
  'use strict';

  let lastToken = '';

  function syncSession() {
    try {
      const raw = localStorage.getItem('userToken');
      if (!raw) return;

      // Avoid spamming if unchanged and recently synced
      if (raw === lastToken) return;

      fetch('http://localhost:20130/api/providers/deepseek-web/quick-sync', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          userToken: raw,
          cookie: document.cookie
        })
      })
      .then(r => r.json())
      .then(data => {
        lastToken = raw;
        console.log('[9Router Bridge] Synced successfully:', data);
        showBadge('✅ 9Router Synced: ' + (data.message || 'Active'));
      })
      .catch(err => {
        console.warn('[9Router Bridge] Sync failed:', err);
      });
    } catch (e) {
      console.error('[9Router Bridge] Error:', e);
    }
  }

  function showBadge(text) {
    let b = document.getElementById('9router-bridge-badge');
    if (!b) {
      b = document.createElement('div');
      b.id = '9router-bridge-badge';
      b.style.cssText = 'position:fixed;bottom:20px;right:20px;background:rgba(16,185,129,0.95);color:#fff;padding:8px 16px;border-radius:10px;font-size:12px;font-weight:600;z-index:999999;box-shadow:0 8px 24px rgba(0,0,0,0.25);font-family:system-ui,-apple-system,sans-serif;pointer-events:none;transition:all 0.4s ease;backdrop-filter:blur(8px);';
      document.body.appendChild(b);
    }
    b.textContent = text;
    b.style.opacity = '1';
    b.style.transform = 'translateY(0)';
    setTimeout(() => {
      b.style.opacity = '0';
      b.style.transform = 'translateY(8px)';
    }, 4000);
  }

  // Trigger on load and poll periodically
  setTimeout(syncSession, 1200);
  setInterval(syncSession, 20000);
})();

