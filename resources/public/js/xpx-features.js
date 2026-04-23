/* swarmpit-xpx features — sidebar items for GitOps, Prune, Image Updates */
(function () {
  'use strict';
  var TOKEN = '';
  function getToken() {
    try {
      var raw = localStorage.getItem('token') || sessionStorage.getItem('token');
      if (raw) TOKEN = raw.replace(/^"|"$/g, '');
    } catch (e) {}
  }
  function api(method, path, body) {
    getToken();
    var opts = { method: method, headers: { 'Authorization': TOKEN, 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    return fetch(path, opts).then(function (r) { return r.json(); });
  }

  /* ── Sidebar injection ── */
  function injectSidebar() {
    var nav = document.querySelector('nav');
    if (!nav || document.getElementById('xpx-sidebar')) return;
    var sep = nav.querySelector('hr');
    if (!sep) return;
    var container = sep.parentNode;

    var wrapper = document.createElement('div');
    wrapper.id = 'xpx-sidebar';

    var label = document.createElement('li');
    label.style.cssText = 'list-style:none;padding:8px 16px 4px;color:rgba(255,255,255,0.5);font-size:11px;text-transform:uppercase;letter-spacing:1px;';
    label.textContent = 'Tools';
    wrapper.appendChild(label);

    var items = [
      { name: 'GitOps', hash: '#/xpx/gitops' },
      { name: 'Image Updates', hash: '#/xpx/updates' },
      { name: 'System Prune', hash: '#/xpx/prune' },
    ];

    items.forEach(function (item) {
      var a = document.createElement('a');
      a.href = item.hash;
      a.textContent = item.name;
      a.style.cssText = 'display:block;padding:8px 16px;color:rgba(255,255,255,0.7);text-decoration:none;font-size:14px;cursor:pointer;';
      a.onmouseenter = function () { a.style.color = '#fff'; a.style.background = 'rgba(255,255,255,0.08)'; };
      a.onmouseleave = function () { a.style.color = 'rgba(255,255,255,0.7)'; a.style.background = 'none'; };
      a.onclick = function (e) { e.preventDefault(); window.location.hash = item.hash.slice(1); renderPage(); };
      wrapper.appendChild(a);
    });

    container.insertBefore(wrapper, sep);
  }

  /* ── Page rendering ── */
  function renderPage() {
    var hash = window.location.hash;
    if (hash.indexOf('/xpx/') === -1) { removePage(); return; }
    var main = document.querySelector('main');
    if (!main) return;

    var page = document.getElementById('xpx-page');
    if (!page) {
      page = document.createElement('div');
      page.id = 'xpx-page';
      page.style.cssText = 'position:absolute;top:0;left:0;right:0;bottom:0;background:#fafafa;z-index:100;padding:24px;overflow:auto;';
      main.style.position = 'relative';
      main.appendChild(page);
    }

    page.innerHTML = '';
    if (hash.indexOf('gitops') > -1) renderGitOps(page);
    else if (hash.indexOf('updates') > -1) renderUpdates(page);
    else if (hash.indexOf('prune') > -1) renderPrune(page);
  }

  function removePage() {
    var page = document.getElementById('xpx-page');
    if (page) page.remove();
  }

  /* ── GitOps page ── */
  function renderGitOps(page) {
    page.innerHTML = '<h2 style="margin:0 0 16px;font-weight:600">GitOps Stacks</h2><div id="xpx-gitops-list">Loading...</div><button id="xpx-gitops-create" style="margin-top:16px;padding:8px 20px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px">Create GitOps Stack</button>';
    loadGitOps();
    document.getElementById('xpx-gitops-create').onclick = function () { showGitOpsForm(page); };
  }

  function loadGitOps() {
    api('GET', '/api/gitops').then(function (data) {
      var el = document.getElementById('xpx-gitops-list');
      if (!el) return;
      if (!data || data.length === 0) { el.innerHTML = '<p style="color:#666">No GitOps stacks configured.</p>'; return; }
      var html = '<table style="width:100%;border-collapse:collapse;font-size:14px"><thead><tr style="text-align:left;border-bottom:2px solid #ddd"><th style="padding:8px">Stack</th><th style="padding:8px">Repository</th><th style="padding:8px">Branch</th><th style="padding:8px">Status</th><th style="padding:8px">Last Sync</th><th style="padding:8px">Actions</th></tr></thead><tbody>';
      data.forEach(function (gs) {
        var status = gs.lastError ? '<span style="color:#f44336">Error</span>' : gs.lastHash ? '<span style="color:#4caf50">OK</span>' : '<span style="color:#999">Pending</span>';
        html += '<tr style="border-bottom:1px solid #eee"><td style="padding:8px">' + gs.stackName + '</td><td style="padding:8px;max-width:300px;overflow:hidden;text-overflow:ellipsis">' + gs.repoUrl + '</td><td style="padding:8px">' + gs.branch + '</td><td style="padding:8px">' + status + '</td><td style="padding:8px">' + (gs.lastSync || '-') + '</td><td style="padding:8px"><button data-sync="' + gs.id + '" style="padding:4px 12px;background:#1976d2;color:#fff;border:none;border-radius:3px;cursor:pointer;margin-right:4px">Sync</button><button data-delete="' + gs.id + '" style="padding:4px 12px;background:#f44336;color:#fff;border:none;border-radius:3px;cursor:pointer">Delete</button></td></tr>';
      });
      html += '</tbody></table>';
      el.innerHTML = html;
      el.querySelectorAll('[data-sync]').forEach(function (btn) {
        btn.onclick = function () { api('POST', '/api/gitops/' + btn.dataset.sync + '/sync').then(loadGitOps); };
      });
      el.querySelectorAll('[data-delete]').forEach(function (btn) {
        btn.onclick = function () { if (confirm('Delete this GitOps stack?')) api('DELETE', '/api/gitops/' + btn.dataset.delete).then(loadGitOps); };
      });
    });
  }

  function showGitOpsForm(page) {
    var form = document.createElement('div');
    form.style.cssText = 'margin-top:16px;padding:16px;background:#fff;border:1px solid #ddd;border-radius:4px;max-width:500px;';
    form.innerHTML = '<h3 style="margin:0 0 12px">New GitOps Stack</h3>' +
      '<label style="display:block;margin-bottom:8px">Stack Name<br><input id="xf-name" style="width:100%;padding:6px;box-sizing:border-box"></label>' +
      '<label style="display:block;margin-bottom:8px">Repository URL<br><input id="xf-repo" style="width:100%;padding:6px;box-sizing:border-box"></label>' +
      '<label style="display:block;margin-bottom:8px">Branch<br><input id="xf-branch" value="main" style="width:100%;padding:6px;box-sizing:border-box"></label>' +
      '<label style="display:block;margin-bottom:8px">Compose Path<br><input id="xf-path" value="docker-compose.yml" style="width:100%;padding:6px;box-sizing:border-box"></label>' +
      '<label style="display:block;margin-bottom:8px">Sync Interval (seconds, 0=manual)<br><input id="xf-interval" type="number" value="0" style="width:100%;padding:6px;box-sizing:border-box"></label>' +
      '<button id="xf-submit" style="padding:8px 20px;background:#4caf50;color:#fff;border:none;border-radius:4px;cursor:pointer;margin-right:8px">Create</button>' +
      '<button id="xf-cancel" style="padding:8px 20px;background:#999;color:#fff;border:none;border-radius:4px;cursor:pointer">Cancel</button>';
    page.appendChild(form);
    document.getElementById('xf-cancel').onclick = function () { form.remove(); };
    document.getElementById('xf-submit').onclick = function () {
      api('POST', '/api/gitops', {
        stackName: document.getElementById('xf-name').value,
        repoUrl: document.getElementById('xf-repo').value,
        branch: document.getElementById('xf-branch').value,
        composePath: document.getElementById('xf-path').value,
        syncInterval: parseInt(document.getElementById('xf-interval').value) || 0,
      }).then(function () { form.remove(); loadGitOps(); });
    };
  }

  /* ── Image Updates page ── */
  function renderUpdates(page) {
    page.innerHTML = '<h2 style="margin:0 0 16px;font-weight:600">Image Updates</h2><button id="xpx-check-updates" style="padding:8px 20px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px">Check Now</button><div id="xpx-updates-result" style="margin-top:16px"></div>';
    document.getElementById('xpx-check-updates').onclick = function () {
      var btn = this; btn.disabled = true; btn.textContent = 'Checking...';
      api('POST', '/api/services/check-updates').then(function (data) {
        btn.disabled = false; btn.textContent = 'Check Now';
        var el = document.getElementById('xpx-updates-result');
        if (!data || data.length === 0) { el.innerHTML = '<p style="color:#4caf50;font-weight:500">All images are up to date.</p>'; return; }
        var html = '<p style="color:#ff9800;font-weight:500">' + data.length + ' update(s) available:</p><ul style="margin:8px 0;padding-left:20px">';
        data.forEach(function (u) { html += '<li style="margin:4px 0">' + u.serviceName + ': <code>' + u.image + '</code></li>'; });
        el.innerHTML = html + '</ul>';
      });
    };
  }

  /* ── System Prune page ── */
  function renderPrune(page) {
    page.innerHTML = '<h2 style="margin:0 0 16px;font-weight:600">System Prune</h2><p style="color:#666;margin-bottom:16px">Remove unused Docker resources (dangling images, orphan volumes, unused networks).</p><button id="xpx-prune-preview" style="padding:8px 20px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px;margin-right:8px">Preview</button><button id="xpx-prune-exec" style="padding:8px 20px;background:#f44336;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px">Prune Now</button><div id="xpx-prune-result" style="margin-top:16px"></div>';
    document.getElementById('xpx-prune-preview').onclick = function () { doPrune(true); };
    document.getElementById('xpx-prune-exec').onclick = function () { if (confirm('Remove all unused resources? This cannot be undone.')) doPrune(false); };
  }

  function doPrune(dryRun) {
    api('POST', '/api/system/prune', { images: true, volumes: true, networks: true, dryRun: dryRun }).then(function (r) {
      var el = document.getElementById('xpx-prune-result');
      if (!el) return;
      var label = dryRun ? 'Preview (no changes made)' : 'Cleanup completed';
      var color = dryRun ? '#1976d2' : '#4caf50';
      el.innerHTML = '<p style="color:' + color + ';font-weight:500">' + label + '</p>' +
        '<ul style="margin:8px 0;padding-left:20px">' +
        '<li>Images: ' + (r.images ? r.images.count : 0) + ' (' + (r.images ? (r.images.spaceReclaimed / 1e9).toFixed(1) : 0) + ' GB)</li>' +
        '<li>Volumes: ' + (r.volumes ? r.volumes.count : 0) + '</li>' +
        '<li>Networks: ' + (r.networks ? r.networks.count : 0) + '</li></ul>';
    });
  }

  /* ── Main loop ── */
  window.addEventListener('hashchange', function () { setTimeout(function () { injectSidebar(); renderPage(); }, 300); });

  // Use MutationObserver to re-inject when ClojureScript re-renders nav
  function startObserver() {
    var nav = document.querySelector('nav');
    if (!nav) return false;
    var observer = new MutationObserver(function () {
      if (!document.getElementById('xpx-sidebar')) injectSidebar();
    });
    observer.observe(nav, { childList: true, subtree: true });
    return true;
  }

  // Init: wait for nav to appear, then inject + observe
  var initInterval = setInterval(function () {
    getToken();
    if (!TOKEN) return;
    var nav = document.querySelector('nav');
    if (!nav || !nav.querySelector('hr')) return;
    injectSidebar();
    renderPage();
    if (startObserver()) clearInterval(initInterval);
  }, 500);
})();
