/* swarmpit-xpx features — floating tools panel */
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

  /* ── Floating button + panel ── */
  function createToolsButton() {
    if (document.getElementById('xpx-tools-btn')) return;

    // Floating button
    var btn = document.createElement('div');
    btn.id = 'xpx-tools-btn';
    btn.textContent = 'Tools';
    btn.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:9999;background:#1976d2;color:#fff;padding:10px 20px;border-radius:24px;cursor:pointer;font-size:14px;font-weight:600;box-shadow:0 2px 8px rgba(0,0,0,0.3);user-select:none;';
    btn.onclick = togglePanel;
    document.body.appendChild(btn);
  }

  function togglePanel() {
    var panel = document.getElementById('xpx-tools-panel');
    if (panel) { panel.remove(); return; }

    panel = document.createElement('div');
    panel.id = 'xpx-tools-panel';
    panel.style.cssText = 'position:fixed;bottom:70px;right:20px;z-index:9998;background:#fff;border-radius:8px;box-shadow:0 4px 20px rgba(0,0,0,0.25);width:600px;max-height:70vh;overflow:auto;padding:20px;font-family:Roboto,sans-serif;';

    // Tabs
    var tabs = document.createElement('div');
    tabs.style.cssText = 'display:flex;gap:8px;margin-bottom:16px;border-bottom:2px solid #eee;padding-bottom:8px;';
    var pages = ['GitOps', 'Image Updates', 'System Prune'];
    pages.forEach(function (name) {
      var tab = document.createElement('button');
      tab.textContent = name;
      tab.dataset.tab = name;
      tab.style.cssText = 'padding:6px 16px;border:none;background:none;cursor:pointer;font-size:14px;color:#666;border-bottom:2px solid transparent;margin-bottom:-10px;';
      tab.onclick = function () {
        tabs.querySelectorAll('button').forEach(function (b) { b.style.color = '#666'; b.style.borderBottomColor = 'transparent'; });
        tab.style.color = '#1976d2';
        tab.style.borderBottomColor = '#1976d2';
        renderTab(name, content);
      };
      tabs.appendChild(tab);
    });
    panel.appendChild(tabs);

    var content = document.createElement('div');
    content.id = 'xpx-tab-content';
    panel.appendChild(content);

    document.body.appendChild(panel);

    // Activate first tab
    tabs.querySelector('button').click();
  }

  function renderTab(name, container) {
    container.innerHTML = '';
    if (name === 'GitOps') renderGitOps(container);
    else if (name === 'Image Updates') renderUpdates(container);
    else if (name === 'System Prune') renderPrune(container);
  }

  /* ── GitOps ── */
  function renderGitOps(el) {
    el.innerHTML = '<div id="xpx-gitops-list" style="color:#666">Loading...</div><button id="xpx-gitops-create" style="margin-top:12px;padding:6px 16px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:13px">Create GitOps Stack</button>';
    loadGitOps();
    document.getElementById('xpx-gitops-create').onclick = function () { showGitOpsForm(el); };
  }

  function loadGitOps() {
    api('GET', '/api/gitops').then(function (data) {
      var el = document.getElementById('xpx-gitops-list');
      if (!el) return;
      if (!data || data.length === 0) { el.innerHTML = '<p style="color:#999;margin:0">No GitOps stacks configured.</p>'; return; }
      var html = '<table style="width:100%;border-collapse:collapse;font-size:13px"><thead><tr style="text-align:left;border-bottom:2px solid #eee"><th style="padding:6px">Stack</th><th style="padding:6px">Repo</th><th style="padding:6px">Branch</th><th style="padding:6px">Status</th><th style="padding:6px">Actions</th></tr></thead><tbody>';
      data.forEach(function (gs) {
        var status = gs.lastError ? '<span style="color:#f44336">Error</span>' : gs.lastHash ? '<span style="color:#4caf50">OK</span>' : '<span style="color:#999">Pending</span>';
        html += '<tr style="border-bottom:1px solid #f5f5f5"><td style="padding:6px">' + gs.stackName + '</td><td style="padding:6px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + gs.repoUrl + '</td><td style="padding:6px">' + gs.branch + '</td><td style="padding:6px">' + status + '</td><td style="padding:6px"><button data-sync="' + gs.id + '" style="padding:3px 10px;background:#1976d2;color:#fff;border:none;border-radius:3px;cursor:pointer;margin-right:4px;font-size:12px">Sync</button><button data-delete="' + gs.id + '" style="padding:3px 10px;background:#f44336;color:#fff;border:none;border-radius:3px;cursor:pointer;font-size:12px">Del</button></td></tr>';
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

  function showGitOpsForm(parent) {
    if (document.getElementById('xpx-gitops-form')) return;
    var form = document.createElement('div');
    form.id = 'xpx-gitops-form';
    form.style.cssText = 'margin-top:12px;padding:12px;background:#fafafa;border:1px solid #eee;border-radius:4px;';
    form.innerHTML = '<div style="font-weight:600;margin-bottom:8px">New GitOps Stack</div>' +
      '<input id="xf-name" placeholder="Stack Name" style="width:100%;padding:6px;margin-bottom:6px;box-sizing:border-box;border:1px solid #ddd;border-radius:3px">' +
      '<input id="xf-repo" placeholder="Repository URL" style="width:100%;padding:6px;margin-bottom:6px;box-sizing:border-box;border:1px solid #ddd;border-radius:3px">' +
      '<div style="display:flex;gap:6px;margin-bottom:6px"><input id="xf-branch" value="main" placeholder="Branch" style="flex:1;padding:6px;border:1px solid #ddd;border-radius:3px"><input id="xf-path" value="docker-compose.yml" placeholder="Compose path" style="flex:1;padding:6px;border:1px solid #ddd;border-radius:3px"></div>' +
      '<input id="xf-interval" type="number" value="0" placeholder="Sync interval (sec)" style="width:100%;padding:6px;margin-bottom:8px;box-sizing:border-box;border:1px solid #ddd;border-radius:3px">' +
      '<button id="xf-submit" style="padding:6px 16px;background:#4caf50;color:#fff;border:none;border-radius:4px;cursor:pointer;margin-right:6px;font-size:13px">Create</button>' +
      '<button id="xf-cancel" style="padding:6px 16px;background:#999;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:13px">Cancel</button>';
    parent.appendChild(form);
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

  /* ── Image Updates ── */
  function renderUpdates(el) {
    el.innerHTML = '<button id="xpx-check-btn" style="padding:6px 16px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:13px">Check Now</button><div id="xpx-updates-result" style="margin-top:12px"></div>';
    document.getElementById('xpx-check-btn').onclick = function () {
      var btn = this; btn.disabled = true; btn.textContent = 'Checking...';
      api('POST', '/api/services/check-updates').then(function (data) {
        btn.disabled = false; btn.textContent = 'Check Now';
        var r = document.getElementById('xpx-updates-result');
        if (!data || data.length === 0) { r.innerHTML = '<p style="color:#4caf50;margin:0">All images are up to date.</p>'; return; }
        var html = '<p style="color:#ff9800;margin:0 0 8px">' + data.length + ' update(s) available:</p><ul style="margin:0;padding-left:20px">';
        data.forEach(function (u) { html += '<li style="margin:2px 0;font-size:13px">' + u.serviceName + '</li>'; });
        r.innerHTML = html + '</ul>';
      });
    };
  }

  /* ── System Prune ── */
  function renderPrune(el) {
    el.innerHTML = '<p style="color:#666;margin:0 0 12px;font-size:13px">Remove unused Docker resources (dangling images, orphan volumes, unused networks).</p><button id="xpx-preview-btn" style="padding:6px 16px;background:#1976d2;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:13px;margin-right:6px">Preview</button><button id="xpx-prune-btn" style="padding:6px 16px;background:#f44336;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:13px">Prune Now</button><div id="xpx-prune-result" style="margin-top:12px"></div>';
    document.getElementById('xpx-preview-btn').onclick = function () { doPrune(true); };
    document.getElementById('xpx-prune-btn').onclick = function () { if (confirm('Remove all unused resources?')) doPrune(false); };
  }

  function doPrune(dryRun) {
    api('POST', '/api/system/prune', { images: true, volumes: true, networks: true, dryRun: dryRun }).then(function (r) {
      var el = document.getElementById('xpx-prune-result');
      if (!el) return;
      var label = dryRun ? 'Preview (no changes)' : 'Cleanup completed';
      var color = dryRun ? '#1976d2' : '#4caf50';
      el.innerHTML = '<p style="color:' + color + ';font-weight:500;margin:0 0 4px">' + label + '</p>' +
        '<ul style="margin:0;padding-left:20px;font-size:13px">' +
        '<li>Images: ' + (r.images ? r.images.count : 0) + ' (' + (r.images ? (r.images.spaceReclaimed / 1e9).toFixed(1) : 0) + ' GB)</li>' +
        '<li>Volumes: ' + (r.volumes ? r.volumes.count : 0) + '</li>' +
        '<li>Networks: ' + (r.networks ? r.networks.count : 0) + '</li></ul>';
    });
  }

  /* ── Stack compose editor auto-fill ── */
  function initComposeAutoFill() {
    var hash = window.location.hash;
    if (hash.indexOf('/stacks/') === -1 || hash.indexOf('/compose') === -1) return;
    var m = hash.match(/\/stacks\/([^/]+)/);
    if (!m) return;
    var stackName = m[1];
    var attempts = 0;
    var iv = setInterval(function () {
      if (++attempts > 15) { clearInterval(iv); return; }
      var el = document.querySelector('.CodeMirror');
      if (!el || !el.CodeMirror) return;
      var cm = el.CodeMirror;
      if (cm.getValue().trim()) { clearInterval(iv); return; }
      clearInterval(iv);
      api('GET', '/api/stacks/' + stackName + '/compose').then(function (data) {
        if (data && data.compose && !cm.getValue().trim()) cm.setValue(data.compose);
      });
    }, 2000);
  }

  /* ── Init: wait for login then show button ── */
  var initInterval = setInterval(function () {
    getToken();
    if (TOKEN) { createToolsButton(); clearInterval(initInterval); initComposeAutoFill(); }
  }, 1000);
  window.addEventListener('hashchange', function () { getToken(); if (TOKEN) initComposeAutoFill(); });
})();
