/* swarmpit-xpx features overlay — injected into ClojureScript UI */
(function () {
  'use strict';
  var TOKEN = '';
  var API = '';

  function getToken() {
    try {
      var raw = localStorage.getItem('token') || sessionStorage.getItem('token');
      if (raw) { TOKEN = raw.replace(/^"|"$/g, ''); return; }
      document.cookie.split(';').forEach(function (c) {
        if (c.trim().indexOf('token=') === 0) TOKEN = c.trim().slice(6);
      });
    } catch (e) { /* ignore */ }
  }

  function api(method, path, body) {
    getToken();
    var opts = { method: method, headers: { 'Authorization': TOKEN, 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    return fetch(API + path, opts).then(function (r) { return r.json(); });
  }

  /* ── #66 Image Update Badges ── */
  var updateCache = {};
  function loadUpdateStatus() {
    api('GET', '/api/services/update-status').then(function (data) {
      updateCache = data || {};
      applyUpdateBadges();
    }).catch(function () {});
  }

  function applyUpdateBadges() {
    // Find service rows in the service list table and add badge
    var rows = document.querySelectorAll('table tbody tr');
    rows.forEach(function (row) {
      if (row.querySelector('.xpx-update-badge')) return;
      var link = row.querySelector('td a, td span');
      if (!link) return;
      // Match service ID from any data attribute or href
      var href = row.querySelector('a[href*="services/"]');
      if (!href) return;
      var parts = href.getAttribute('href').split('/');
      var svcName = parts[parts.length - 1];
      // Check all update entries
      var hasUpdate = Object.values(updateCache).some(function (v) { return v === true; });
      if (hasUpdate) {
        Object.keys(updateCache).forEach(function (svcId) {
          if (updateCache[svcId] && link.textContent && row.textContent.indexOf(svcName) >= 0) {
            var badge = document.createElement('span');
            badge.className = 'xpx-update-badge';
            badge.style.cssText = 'background:#ff9800;color:#fff;font-size:10px;padding:1px 6px;border-radius:8px;margin-left:6px;';
            badge.textContent = '↑ update';
            link.parentNode.appendChild(badge);
          }
        });
      }
    });
  }

  /* ── #67 Prune Button ── */
  function injectPruneButton() {
    var toolbar = document.querySelector('main > div > div:first-child');
    if (!toolbar || document.getElementById('xpx-prune-btn')) return;
    if (!window.location.hash.match(/^\#\/?$/)) return; // only on dashboard

    var btn = document.createElement('button');
    btn.id = 'xpx-prune-btn';
    btn.textContent = '🧹 Prune';
    btn.title = 'Clean up unused images, volumes, networks';
    btn.style.cssText = 'background:#f44336;color:#fff;border:none;padding:6px 16px;border-radius:4px;cursor:pointer;font-size:13px;margin-left:12px;';
    btn.onclick = function () {
      api('POST', '/api/system/prune', { images: true, volumes: true, networks: true, dryRun: true })
        .then(function (r) {
          var imgs = r.images ? r.images.count : 0;
          var space = r.images ? (r.images.spaceReclaimed / 1e9).toFixed(1) : 0;
          var vols = r.volumes ? r.volumes.count : 0;
          var nets = r.networks ? r.networks.count : 0;
          if (confirm('Prune will remove:\n• ' + imgs + ' dangling images (' + space + ' GB)\n• ' + vols + ' unused volumes\n• ' + nets + ' unused networks\n\nProceed?')) {
            api('POST', '/api/system/prune', { images: true, volumes: true, networks: true, dryRun: false })
              .then(function (r2) { alert('Pruned successfully!'); })
              .catch(function () { alert('Prune failed'); });
          }
        });
    };
    toolbar.appendChild(btn);
  }

  /* ── #69 GitOps indicator in Stacks ── */
  function injectGitOpsIndicator() {
    if (!window.location.hash.match(/stacks/)) return;
    var toolbar = document.querySelector('main > div > div:first-child');
    if (!toolbar || document.getElementById('xpx-gitops-btn')) return;

    var btn = document.createElement('button');
    btn.id = 'xpx-gitops-btn';
    btn.textContent = '⚙ GitOps';
    btn.title = 'Manage GitOps stacks';
    btn.style.cssText = 'background:#1976d2;color:#fff;border:none;padding:6px 16px;border-radius:4px;cursor:pointer;font-size:13px;margin-left:12px;';
    btn.onclick = function () {
      window.open('/admin#/gitops', '_blank');
    };
    toolbar.appendChild(btn);
  }

  /* ── Admin panel link ── */
  function injectAdminLink() {
    var nav = document.querySelector('nav');
    if (!nav || document.getElementById('xpx-admin-link')) return;
    var sep = nav.querySelector('hr');
    if (!sep) return;
    var container = sep.parentNode;

    var link = document.createElement('a');
    link.id = 'xpx-admin-link';
    link.href = '/admin';
    link.target = '_blank';
    link.style.cssText = 'display:flex;align-items:center;padding:8px 16px;color:rgba(255,255,255,0.7);text-decoration:none;font-size:14px;gap:8px;';
    link.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg><span style="font-size:12px;font-weight:500;">Admin Panel</span>';
    link.onmouseenter = function () { link.style.color = '#fff'; };
    link.onmouseleave = function () { link.style.color = 'rgba(255,255,255,0.7)'; };
    container.insertBefore(link, sep);
  }

  /* ── Main loop ── */
  function tick() {
    getToken();
    if (!TOKEN) return;
    injectPruneButton();
    injectGitOpsIndicator();
    injectAdminLink();
    applyUpdateBadges();
  }

  // Run on hash change and periodically
  window.addEventListener('hashchange', function () { setTimeout(tick, 500); });
  setInterval(tick, 3000);

  // Initial load
  setTimeout(function () {
    getToken();
    if (TOKEN) loadUpdateStatus();
    tick();
  }, 2000);

  // Refresh update status every 5 min
  setInterval(loadUpdateStatus, 300000);
})();
