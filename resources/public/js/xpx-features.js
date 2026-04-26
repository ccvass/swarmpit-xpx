/* swarmpit-xpx features — floating tools panel */
/* Patch: CLJS PersistentVector doesn't have native .filter/.map/.some
   MUI Autocomplete calls options.filter() which crashes on CLJS collections.
   Fix: monkey-patch Array.isArray to recognize CLJS collections, and add
   .filter/.map/.some/.indexOf to their prototypes. */
(function(){
  function isCljsColl(a) {
    return a && typeof a === 'object' && typeof a.cljs$lang$protocol_mask$partition0$ === 'number';
  }
  function toArr(obj) {
    if (Array.isArray(obj)) return obj;
    if (!obj) return [];
    var arr = [];
    if (typeof cljs !== 'undefined' && cljs.core && cljs.core.into_array) {
      try { return cljs.core.into_array.call(null, obj); } catch(e) {}
    }
    if (typeof obj.forEach === 'function') { obj.forEach(function(v){arr.push(v);}); return arr; }
    if (typeof obj.count === 'function') { for(var i=0;i<obj.count();i++) arr.push(cljs.core.nth.call(null,obj,i)); return arr; }
    return arr;
  }
  // Patch CLJS PersistentVector prototype once it exists
  var patchInterval = setInterval(function(){
    if (typeof cljs === 'undefined' || !cljs.core || !cljs.core.PersistentVector) return;
    clearInterval(patchInterval);
    var PV = cljs.core.PersistentVector.prototype;
    if (!PV.filter) {
      PV.filter = function(fn) { return toArr(this).filter(fn); };
      PV.map = function(fn) { return toArr(this).map(fn); };
      PV.some = function(fn) { return toArr(this).some(fn); };
      PV.indexOf = function(v) { return toArr(this).indexOf(v); };
      PV.forEach = PV.forEach || function(fn) { var a=toArr(this); for(var i=0;i<a.length;i++) fn(a[i],i,a); };
      PV.slice = function(a,b) { return toArr(this).slice(a,b); };
      PV.concat = function() { return toArr(this).concat.apply(toArr(this), arguments); };
    }
    // Also patch Subvec if it exists
    if (cljs.core.Subvec && !cljs.core.Subvec.prototype.filter) {
      var SV = cljs.core.Subvec.prototype;
      SV.filter = function(fn) { return toArr(this).filter(fn); };
      SV.map = function(fn) { return toArr(this).map(fn); };
      SV.some = function(fn) { return toArr(this).some(fn); };
      SV.indexOf = function(v) { return toArr(this).indexOf(v); };
      SV.forEach = SV.forEach || function(fn) { var a=toArr(this); for(var i=0;i<a.length;i++) fn(a[i],i,a); };
      SV.slice = function(a,b) { return toArr(this).slice(a,b); };
    }
    // Patch LazySeq
    if (cljs.core.LazySeq && !cljs.core.LazySeq.prototype.filter) {
      var LS = cljs.core.LazySeq.prototype;
      LS.filter = function(fn) { return toArr(this).filter(fn); };
      LS.map = function(fn) { return toArr(this).map(fn); };
      LS.some = function(fn) { return toArr(this).some(fn); };
      LS.indexOf = function(v) { return toArr(this).indexOf(v); };
      LS.forEach = LS.forEach || function(fn) { var a=toArr(this); for(var i=0;i<a.length;i++) fn(a[i],i,a); };
      LS.slice = function(a,b) { return toArr(this).slice(a,b); };
    }
  }, 50);
})();
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

  /* ── Sidebar: inject items into main nav list ── */
  function injectSidebar() {
    if (document.getElementById('xpx-tools')) return;
    var nav = document.querySelector('nav');
    if (!nav) return;
    // Find the main nav list (second child of Swarmpit-drawer-content)
    var content = nav.querySelector('.Swarmpit-drawer-content');
    if (!content || content.children.length < 2) return;
    var navList = content.children[1]; // the MuiBox with Dashboard, Registries, etc.

    var wrapper = document.createElement('div');
    wrapper.id = 'xpx-tools';

    var label = document.createElement('li');
    label.className = 'MuiListSubheader-root';
    label.style.cssText = 'list-style:none;padding:12px 16px 4px;color:rgba(255,255,255,0.5);font-size:0.75rem;text-transform:uppercase;letter-spacing:0.08333em;line-height:48px;font-weight:500;font-family:Roboto,sans-serif;';
    label.textContent = 'Tools';
    wrapper.appendChild(label);

    var items = [
      { name: 'GitOps', hash: '/xpx/gitops' },
      { name: 'Image Updates', hash: '/xpx/updates' },
      { name: 'System Prune', hash: '/xpx/prune' }
    ];
    items.forEach(function (item) {
      var a = document.createElement('a');
      a.href = '#' + item.hash;
      a.className = 'MuiButtonBase-root MuiListItem-root MuiListItem-gutters MuiListItem-button';
      a.style.cssText = 'display:flex;align-items:center;padding:4px 16px;color:rgba(255,255,255,0.7);text-decoration:none;font:400 14px/1.5 Roboto,sans-serif;cursor:pointer;transition:background 0.15s;width:100%;box-sizing:border-box;';
      a.onmouseenter = function () { a.style.background = 'rgba(255,255,255,0.08)'; };
      a.onmouseleave = function () { a.style.background = 'none'; };
      a.onclick = function (e) { e.preventDefault(); window.location.hash = item.hash; renderPage(); };
      var icon = document.createElement('div');
      icon.style.cssText = 'min-width:40px;display:flex;align-items:center;justify-content:center;color:rgba(255,255,255,0.5);';
      icon.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="3"/></svg>';
      a.appendChild(icon);
      var txt = document.createElement('h6');
      txt.className = 'MuiTypography-root MuiTypography-h6';
      txt.style.cssText = 'font-size:0.875rem;font-weight:400;line-height:1.5;color:inherit;margin:0;';
      txt.textContent = item.name;
      a.appendChild(txt);
      wrapper.appendChild(a);
    });

    navList.appendChild(wrapper);
  }

  /* ── Page rendering in main area ── */
  function renderPage() {
    var hash = window.location.hash;
    if (hash.indexOf('/xpx/') === -1) { removePage(); return; }
    var main = document.querySelector('main');
    if (!main) return;
    var page = document.getElementById('xpx-page');
    if (!page) {
      page = document.createElement('div');
      page.id = 'xpx-page';
      page.style.cssText = 'position:absolute;top:0;left:0;right:0;bottom:0;background:#fafafa;z-index:100;padding:24px 32px;overflow:auto;font-family:Roboto,sans-serif;';
      main.style.position = 'relative';
      main.appendChild(page);
    }
    page.innerHTML = '';
    if (hash.indexOf('gitops') > -1) renderGitOps(page);
    else if (hash.indexOf('updates') > -1) renderUpdates(page);
    else if (hash.indexOf('prune') > -1) renderPrune(page);
  }

  function removePage() {
    var p = document.getElementById('xpx-page');
    if (p) p.remove();
  }

  function heading(text) {
    return '<h5 style="margin:0 0 20px;font-weight:500;font-size:20px;color:#333">' + text + '</h5>';
  }
  function btn(id, label, color) {
    return '<button id="' + id + '" style="padding:8px 20px;background:' + (color || '#1976d2') + ';color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:14px;font-weight:500;margin-right:8px;transition:opacity 0.15s;" onmouseenter="this.style.opacity=0.85" onmouseleave="this.style.opacity=1">' + label + '</button>';
  }

  /* ── GitOps ── */
  function renderGitOps(el) {
    el.innerHTML = heading('GitOps Stacks') + '<div id="xpx-gitops-list" style="color:#666">Loading...</div><div style="margin-top:16px">' + btn('xpx-gitops-create', 'Create GitOps Stack') + '</div>';
    loadGitOps();
    document.getElementById('xpx-gitops-create').onclick = function () { showGitOpsForm(el); };
  }
  function loadGitOps() {
    api('GET', '/api/gitops').then(function (data) {
      var el = document.getElementById('xpx-gitops-list');
      if (!el) return;
      if (!data || data.length === 0) { el.innerHTML = '<p style="color:#999">No GitOps stacks configured yet.</p>'; return; }
      var html = '<table style="width:100%;border-collapse:collapse;font-size:14px"><thead><tr style="text-align:left;border-bottom:2px solid #e0e0e0"><th style="padding:10px 12px">Stack</th><th style="padding:10px 12px">Repository</th><th style="padding:10px 12px">Branch</th><th style="padding:10px 12px">Status</th><th style="padding:10px 12px">Last Sync</th><th style="padding:10px 12px">Actions</th></tr></thead><tbody>';
      data.forEach(function (gs) {
        var status = gs.lastError ? '<span style="color:#d32f2f;font-weight:500">Error</span>' : gs.lastHash ? '<span style="color:#2e7d32;font-weight:500">OK</span>' : '<span style="color:#999">Pending</span>';
        html += '<tr style="border-bottom:1px solid #f0f0f0"><td style="padding:10px 12px;font-weight:500">' + gs.stackName + '</td><td style="padding:10px 12px;max-width:250px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#666">' + gs.repoUrl + '</td><td style="padding:10px 12px">' + gs.branch + '</td><td style="padding:10px 12px">' + status + '</td><td style="padding:10px 12px;color:#666;font-size:13px">' + (gs.lastSync || '-') + '</td><td style="padding:10px 12px">' + btn('', 'Sync', '#1976d2').replace('id=""', 'data-sync="' + gs.id + '"') + btn('', 'Delete', '#d32f2f').replace('id=""', 'data-delete="' + gs.id + '"') + '</td></tr>';
      });
      el.innerHTML = html + '</tbody></table>';
      el.querySelectorAll('[data-sync]').forEach(function (b) { b.onclick = function () { api('POST', '/api/gitops/' + b.dataset.sync + '/sync').then(loadGitOps); }; });
      el.querySelectorAll('[data-delete]').forEach(function (b) { b.onclick = function () { if (confirm('Delete this GitOps stack?')) api('DELETE', '/api/gitops/' + b.dataset.delete).then(loadGitOps); }; });
    });
  }
  function showGitOpsForm(parent) {
    if (document.getElementById('xpx-gitops-form')) return;
    var f = document.createElement('div');
    f.id = 'xpx-gitops-form';
    f.style.cssText = 'margin-top:16px;padding:20px;background:#fff;border:1px solid #e0e0e0;border-radius:8px;max-width:520px;';
    var inp = 'style="width:100%;padding:10px;margin-bottom:10px;box-sizing:border-box;border:1px solid #ddd;border-radius:4px;font-size:14px"';
    f.innerHTML = '<div style="font-weight:500;font-size:16px;margin-bottom:16px">New GitOps Stack</div>' +
      '<input id="xf-name" placeholder="Stack Name" ' + inp + '>' +
      '<input id="xf-repo" placeholder="Repository URL" ' + inp + '>' +
      '<div style="display:flex;gap:10px;margin-bottom:10px"><input id="xf-branch" value="main" placeholder="Branch" style="flex:1;padding:10px;border:1px solid #ddd;border-radius:4px;font-size:14px"><input id="xf-path" value="docker-compose.yml" placeholder="Compose path" style="flex:1;padding:10px;border:1px solid #ddd;border-radius:4px;font-size:14px"></div>' +
      '<input id="xf-interval" type="number" value="0" placeholder="Sync interval (seconds, 0=manual)" ' + inp + '>' +
      btn('xf-submit', 'Create', '#2e7d32') + btn('xf-cancel', 'Cancel', '#757575');
    parent.appendChild(f);
    document.getElementById('xf-cancel').onclick = function () { f.remove(); };
    document.getElementById('xf-submit').onclick = function () {
      api('POST', '/api/gitops', { stackName: document.getElementById('xf-name').value, repoUrl: document.getElementById('xf-repo').value, branch: document.getElementById('xf-branch').value, composePath: document.getElementById('xf-path').value, syncInterval: parseInt(document.getElementById('xf-interval').value) || 0 }).then(function () { f.remove(); loadGitOps(); });
    };
  }

  /* ── Image Updates ── */
  function renderUpdates(el) {
    el.innerHTML = heading('Image Updates') + '<p style="color:#666;margin:0 0 16px;font-size:14px">Check if any running service has a newer image available in its registry.</p>' + btn('xpx-check-btn', 'Check Now') + '<div id="xpx-updates-result" style="margin-top:20px"></div>';
    document.getElementById('xpx-check-btn').onclick = function () {
      var b = this; b.disabled = true; b.textContent = 'Checking...';
      api('POST', '/api/services/check-updates').then(function (data) {
        b.disabled = false; b.textContent = 'Check Now';
        var r = document.getElementById('xpx-updates-result');
        if (!data || data.length === 0) { r.innerHTML = '<div style="padding:16px;background:#e8f5e9;border-radius:8px;color:#2e7d32;font-weight:500">All images are up to date.</div>'; return; }
        var html = '<div style="padding:16px;background:#fff3e0;border-radius:8px;margin-bottom:12px;color:#e65100;font-weight:500">' + data.length + ' update(s) available</div><ul style="margin:0;padding-left:20px">';
        data.forEach(function (u) { html += '<li style="margin:6px 0;font-size:14px"><strong>' + u.serviceName + '</strong> <span style="color:#666">→ newer image available</span></li>'; });
        r.innerHTML = html + '</ul>';
      });
    };
  }

  /* ── System Prune ── */
  function renderPrune(el) {
    el.innerHTML = heading('System Prune') + '<p style="color:#666;margin:0 0 16px;font-size:14px">Remove unused Docker resources: dangling images, orphan volumes, unused networks.</p>' + btn('xpx-preview-btn', 'Preview') + btn('xpx-prune-btn', 'Prune Now', '#d32f2f') + '<div id="xpx-prune-result" style="margin-top:20px"></div>';
    document.getElementById('xpx-preview-btn').onclick = function () { doPrune(true); };
    document.getElementById('xpx-prune-btn').onclick = function () { if (confirm('Remove all unused resources? This cannot be undone.')) doPrune(false); };
  }
  function doPrune(dryRun) {
    api('POST', '/api/system/prune', { images: true, volumes: true, networks: true, dryRun: dryRun }).then(function (r) {
      var el = document.getElementById('xpx-prune-result');
      if (!el) return;
      var bg = dryRun ? '#e3f2fd' : '#e8f5e9';
      var color = dryRun ? '#1565c0' : '#2e7d32';
      var label = dryRun ? 'Preview — no changes made' : 'Cleanup completed';
      el.innerHTML = '<div style="padding:16px;background:' + bg + ';border-radius:8px;color:' + color + ';font-weight:500;margin-bottom:12px">' + label + '</div>' +
        '<div style="padding:16px;background:#fff;border:1px solid #e0e0e0;border-radius:8px"><table style="font-size:14px"><tr><td style="padding:4px 16px 4px 0;font-weight:500">Images</td><td>' + (r.images ? r.images.count : 0) + ' <span style="color:#666">(' + (r.images ? (r.images.spaceReclaimed / 1e9).toFixed(1) : 0) + ' GB)</span></td></tr><tr><td style="padding:4px 16px 4px 0;font-weight:500">Volumes</td><td>' + (r.volumes ? r.volumes.count : 0) + '</td></tr><tr><td style="padding:4px 16px 4px 0;font-weight:500">Networks</td><td>' + (r.networks ? r.networks.count : 0) + '</td></tr></table></div>';
    });
  }

  /* ── Init ── */
  window.addEventListener('hashchange', function () { setTimeout(renderPage, 300); });
  // Persistent interval: re-inject sidebar if ClojureScript removes it
  setInterval(function () {
    getToken();
    if (!TOKEN) return;
    injectSidebar();
  }, 2000);
  // Also try immediately and on short intervals at startup
  var fastInit = setInterval(function () {
    getToken();
    if (!TOKEN) return;
    injectSidebar();
    renderPage();
    if (document.getElementById('xpx-tools')) clearInterval(fastInit);
  }, 300);
})();
