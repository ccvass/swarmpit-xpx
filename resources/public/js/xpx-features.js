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

  /* ── Helpers ── */
  function getToken() {
    try { var r = localStorage.getItem('token') || sessionStorage.getItem('token'); return r ? r.replace(/^"|"$/g, '') : ''; } catch(e) { return ''; }
  }
  function api(method, path, body) {
    var t = getToken(), opts = { method: method, headers: { 'Authorization': t, 'Content-Type': 'application/json' } };
    if (body) opts.body = JSON.stringify(body);
    return fetch(path, opts).then(function(r) { return r.ok ? r.json() : r.json().then(function(e) { throw e; }); });
  }
  function heading(t) { return '<h5 style="margin:0 0 20px;font-weight:400;font-size:1.25rem;color:rgba(0,0,0,.87)">' + t + '</h5>'; }
  function btn(id, label, color) {
    return '<button id="' + id + '" style="padding:6px 16px;background:' + (color || '#1976d2') + ';color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:.875rem;font-weight:500;margin-right:8px;letter-spacing:.02em;line-height:1.75;text-transform:uppercase">' + label + '</button>';
  }
  function tbl(heads, rows) {
    var h = '<table style="width:100%;border-collapse:collapse;font-size:.875rem"><thead><tr style="border-bottom:2px solid #e0e0e0">';
    heads.forEach(function(t) { h += '<th style="padding:8px 12px;text-align:left;font-weight:500">' + t + '</th>'; });
    h += '</tr></thead><tbody>';
    rows.forEach(function(r) { h += '<tr style="border-bottom:1px solid #f0f0f0">'; r.forEach(function(c) { h += '<td style="padding:8px 12px">' + c + '</td>'; }); h += '</tr>'; });
    return h + '</tbody></table>';
  }
  function banner(msg, bg, fg) { return '<div style="padding:12px 16px;background:' + bg + ';border-radius:4px;color:' + fg + ';font-weight:500;margin-bottom:16px">' + msg + '</div>'; }
  function loading() { return '<div style="color:#999;padding:16px 0">Cargando...</div>'; }
  function card(html) { return '<div style="padding:16px;background:#fff;border:1px solid #e0e0e0;border-radius:4px">' + html + '</div>'; }

  var ICONS = {
    gitops: 'M6 2a2 2 0 0 0-2 2v1.17A3.001 3.001 0 0 0 2 8a3 3 0 0 0 2 2.83V14a2 2 0 0 0 2 2h3.17A3.001 3.001 0 0 0 12 18a3 3 0 0 0 2.83-2H18a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2H6zm0 2h12v10h-3.17A3.001 3.001 0 0 0 12 12a3 3 0 0 0-2.83 2H6v-3.17A3.001 3.001 0 0 0 8 8a3 3 0 0 0-2-2.83V4zm-1 4a1 1 0 1 1 0 2 1 1 0 0 1 0-2zm7 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z',
    updates: 'M21 10.12h-6.78l2.74-2.82c-2.73-2.7-7.15-2.8-9.88-.1-2.73 2.71-2.73 7.08 0 9.79s7.15 2.71 9.88 0C18.32 15.65 19 14.08 19 12.1h2c0 2.48-.94 4.96-2.82 6.86-3.72 3.72-9.76 3.72-13.48 0s-3.72-9.76 0-13.48 9.76-3.72 13.48 0L21 2v8.12z',
    prune: 'M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z',
    backup: 'M19.35 10.04A7.49 7.49 0 0 0 12 4C9.11 4 6.6 5.64 5.35 8.04A5.994 5.994 0 0 0 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM14 13v4h-4v-4H7l5-5 5 5h-3z'
  };
  var ITEMS = [
    { name: 'GitOps', hash: '#/xpx/gitops', icon: ICONS.gitops },
    { name: 'Actualizaciones', hash: '#/xpx/updates', icon: ICONS.updates },
    { name: 'Limpieza', hash: '#/xpx/prune', icon: ICONS.prune },
    { name: 'Respaldo S3', hash: '#/xpx/backup', icon: ICONS.backup }
  ];

  /* ── Sidebar injection ── */
  function svgIcon(d) {
    return '<svg class="MuiSvgIcon-root" focusable="false" viewBox="0 0 24 24" aria-hidden="true" role="presentation"><path d="' + d + '"/></svg>';
  }
  function injectSidebar() {
    if (document.getElementById('xpx-tools')) return;
    var nav = document.querySelector('nav');
    if (!nav) return;
    var content = nav.querySelector('.Swarmpit-drawer-content');
    if (!content || content.children.length < 2) return;
    var navList = content.children[1];
    var w = document.createElement('div');
    w.id = 'xpx-tools';
    // Category header
    var li = document.createElement('li');
    li.className = 'MuiListItem-root Swarmpit-drawer-category MuiListItem-gutters Mui-disabled';
    li.setAttribute('disabled', '');
    li.innerHTML = '<div class="MuiListItemText-root Swarmpit-drawer-category-text"><span class="MuiTypography-root MuiListItemText-primary MuiTypography-body1">TOOLS</span></div>';
    w.appendChild(li);
    // Items
    ITEMS.forEach(function(it) {
      var a = document.createElement('a');
      a.className = 'MuiButtonBase-root MuiListItem-root Swarmpit-drawer-item MuiListItem-dense MuiListItem-gutters MuiListItem-button';
      a.href = it.hash;
      a.innerHTML = '<div class="MuiListItemIcon-root Swarmpit-drawer-item-icon" color="primary">' + svgIcon(it.icon) + '</div>' +
        '<div class="MuiListItemText-root Swarmpit-drawer-item-text MuiListItemText-dense"><h6 class="MuiTypography-root MuiTypography-subtitle2">' + it.name + '</h6></div>';
      w.appendChild(a);
    });
    navList.appendChild(w);
  }
  function ensureToolsAtEnd() {
    var t = document.getElementById('xpx-tools');
    if (t && t.parentNode && t.parentNode.lastElementChild !== t) t.parentNode.appendChild(t);
  }

  /* ── Page rendering ── */
  function renderPage() {
    var h = window.location.hash;
    if (h.indexOf('/xpx/') === -1) { var p = document.getElementById('xpx-page'); if (p) p.remove(); return; }
    var main = document.querySelector('main');
    if (!main) return;
    var page = document.getElementById('xpx-page');
    if (!page) {
      page = document.createElement('div');
      page.id = 'xpx-page';
      page.style.cssText = 'position:absolute;top:0;left:0;right:0;bottom:0;background:#fafafa;z-index:100;padding:24px 32px;overflow:auto;font-family:Roboto,sans-serif';
      main.style.position = 'relative';
      main.appendChild(page);
    }
    page.innerHTML = loading();
    if (h.indexOf('gitops') > -1) viewGitOps(page);
    else if (h.indexOf('updates') > -1) viewUpdates(page);
    else if (h.indexOf('prune') > -1) viewPrune(page);
    else if (h.indexOf('backup') > -1) viewBackup(page);
  }

  /* ── GitOps ── */
  function viewGitOps(el) {
    el.innerHTML = heading('GitOps Stacks') + '<div id="go-list">' + loading() + '</div>' +
      '<div style="margin-top:16px">' + btn('go-add', '+ Nuevo Stack', '#2e7d32') + '</div><div id="go-form-wrap"></div>';
    loadStacks();
    el.querySelector('#go-add').onclick = function() { showGitOpsForm(); };
  }
  function loadStacks() {
    api('GET', '/api/gitops').then(function(data) {
      var el = document.getElementById('go-list');
      if (!el) return;
      if (!data || !data.length) { el.innerHTML = '<p style="color:#999">Sin stacks configurados.</p>'; return; }
      var statusMap = function(gs) {
        if (gs.lastError) return '<span style="color:#d32f2f;font-weight:500">Error</span>';
        if (gs.lastHash) return '<span style="color:#2e7d32;font-weight:500">OK</span>';
        return '<span style="color:#9e9e9e">Pendiente</span>';
      };
      var rows = data.map(function(gs) {
        return [gs.stackName, '<span style="max-width:220px;display:inline-block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + gs.repoUrl + '</span>',
          gs.branch, statusMap(gs), gs.lastSync || '-',
          btn('', 'Sync', '#1976d2').replace('id=""', 'data-sync="' + gs.id + '"') +
          btn('', 'Eliminar', '#d32f2f').replace('id=""', 'data-del="' + gs.id + '"')];
      });
      el.innerHTML = tbl(['Stack', 'Repositorio', 'Branch', 'Estado', 'Último Sync', 'Acciones'], rows);
      el.querySelectorAll('[data-sync]').forEach(function(b) {
        b.onclick = function() { b.textContent = '...'; api('POST', '/api/gitops/' + b.dataset.sync + '/sync').then(loadStacks).catch(function() { b.textContent = 'Sync'; }); };
      });
      el.querySelectorAll('[data-del]').forEach(function(b) {
        b.onclick = function() { if (confirm('¿Eliminar este stack?')) api('DELETE', '/api/gitops/' + b.dataset.del).then(loadStacks); };
      });
    }).catch(function(e) { var el = document.getElementById('go-list'); if (el) el.innerHTML = banner('Error: ' + (e.message || e), '#ffebee', '#c62828'); });
  }
  function showGitOpsForm() {
    var wrap = document.getElementById('go-form-wrap');
    if (!wrap || document.getElementById('go-form')) return;
    var inp = function(id, ph, val) { return '<input id="' + id + '" placeholder="' + ph + '" value="' + (val || '') + '" style="width:100%;padding:8px;margin-bottom:8px;box-sizing:border-box;border:1px solid #ccc;border-radius:4px;font-size:.875rem">'; };
    wrap.innerHTML = card(
      '<div style="font-weight:500;margin-bottom:12px">Nuevo Stack GitOps</div>' +
      inp('gf-name', 'Nombre del Stack') + inp('gf-repo', 'URL del Repositorio') +
      '<div style="display:flex;gap:8px">' + '<div style="flex:1">' + inp('gf-branch', 'Branch', 'main') + '</div><div style="flex:1">' + inp('gf-path', 'Compose Path', 'docker-compose.yml') + '</div></div>' +
      inp('gf-interval', 'Intervalo sync (seg, 0=manual)', '0') +
      inp('gf-creds', 'Credenciales (opcional)') +
      btn('gf-ok', 'Crear', '#2e7d32') + btn('gf-no', 'Cancelar', '#757575')
    );
    wrap.id = 'go-form-wrap';
    document.getElementById('gf-no').onclick = function() { wrap.innerHTML = ''; };
    document.getElementById('gf-ok').onclick = function() {
      var b = { stackName: document.getElementById('gf-name').value, repoUrl: document.getElementById('gf-repo').value,
        branch: document.getElementById('gf-branch').value, composePath: document.getElementById('gf-path').value,
        syncInterval: parseInt(document.getElementById('gf-interval').value) || 0 };
      var c = document.getElementById('gf-creds').value; if (c) b.credentials = c;
      api('POST', '/api/gitops', b).then(function() { wrap.innerHTML = ''; loadStacks(); }).catch(function(e) { alert('Error: ' + (e.message || e)); });
    };
  }

  /* ── Image Updates ── */
  function viewUpdates(el) {
    el.innerHTML = heading('Actualizaciones de Imágenes') +
      '<p style="color:#666;font-size:.875rem;margin:0 0 16px">Verifica si hay imágenes más recientes disponibles para los servicios en ejecución.</p>' +
      btn('up-check', 'Verificar Ahora') + '<div id="up-result" style="margin-top:16px"></div>';
    el.querySelector('#up-check').onclick = function() {
      var b = this; b.disabled = true; b.textContent = 'Verificando...';
      document.getElementById('up-result').innerHTML = loading();
      api('POST', '/api/services/check-updates').then(function(data) {
        b.disabled = false; b.textContent = 'VERIFICAR AHORA';
        var r = document.getElementById('up-result');
        if (!r) return;
        if (!data || !data.length) { r.innerHTML = banner('Todas las imágenes están actualizadas.', '#e8f5e9', '#2e7d32'); return; }
        var rows = data.map(function(u) {
          var st = u.updateAvailable ? '<span style="color:#e65100;font-weight:500">Actualización disponible</span>' : '<span style="color:#2e7d32">Al día</span>';
          return [u.serviceName, '<code style="font-size:.8rem;background:#f5f5f5;padding:2px 6px;border-radius:3px">' + u.currentImage + '</code>', st];
        });
        var upCount = data.filter(function(u) { return u.updateAvailable; }).length;
        r.innerHTML = (upCount > 0 ? banner(upCount + ' actualización(es) disponible(s)', '#fff3e0', '#e65100') : banner('Todo al día', '#e8f5e9', '#2e7d32')) +
          tbl(['Servicio', 'Imagen Actual', 'Estado'], rows);
      }).catch(function(e) {
        b.disabled = false; b.textContent = 'VERIFICAR AHORA';
        document.getElementById('up-result').innerHTML = banner('Error: ' + (e.message || e), '#ffebee', '#c62828');
      });
    };
  }

  /* ── System Prune ── */
  function viewPrune(el) {
    var chk = function(id, label) { return '<label style="margin-right:16px;font-size:.875rem;cursor:pointer"><input type="checkbox" id="' + id + '" checked style="margin-right:4px">' + label + '</label>'; };
    el.innerHTML = heading('Limpieza del Sistema') +
      '<p style="color:#666;font-size:.875rem;margin:0 0 12px">Elimina recursos Docker no utilizados.</p>' +
      '<div style="margin-bottom:16px">' + chk('pr-img', 'Imágenes') + chk('pr-vol', 'Volúmenes') + chk('pr-net', 'Redes') + '</div>' +
      btn('pr-preview', 'Vista Previa') + btn('pr-exec', 'Limpiar Ahora', '#d32f2f') +
      '<div id="pr-result" style="margin-top:16px"></div>';
    el.querySelector('#pr-preview').onclick = function() { doPrune(true); };
    el.querySelector('#pr-exec').onclick = function() { if (confirm('¿Eliminar todos los recursos no utilizados? Esta acción no se puede deshacer.')) doPrune(false); };
  }
  function doPrune(dry) {
    var body = { images: document.getElementById('pr-img').checked, volumes: document.getElementById('pr-vol').checked, networks: document.getElementById('pr-net').checked, dryRun: dry };
    var r = document.getElementById('pr-result'); if (r) r.innerHTML = loading();
    api('POST', '/api/system/prune', body).then(function(d) {
      if (!r) return;
      var fmt = function(b) { return b >= 1e9 ? (b / 1e9).toFixed(1) + ' GB' : b >= 1e6 ? (b / 1e6).toFixed(1) + ' MB' : (b / 1e3).toFixed(0) + ' KB'; };
      var row = function(label, obj) { return '<tr><td style="padding:4px 16px 4px 0;font-weight:500">' + label + '</td><td>' + (obj ? obj.count || 0 : 0) + '</td><td style="color:#666">' + (obj && obj.spaceReclaimed ? fmt(obj.spaceReclaimed) : '-') + '</td></tr>'; };
      r.innerHTML = banner(dry ? 'Vista previa — sin cambios realizados' : 'Limpieza completada', dry ? '#e3f2fd' : '#e8f5e9', dry ? '#1565c0' : '#2e7d32') +
        card('<table style="font-size:.875rem"><thead><tr><th style="text-align:left;padding:4px 16px 4px 0">Categoría</th><th style="text-align:left;padding:4px 16px 4px 0">Eliminados</th><th style="text-align:left">Espacio</th></tr></thead><tbody>' +
          row('Imágenes', d.images) + row('Volúmenes', d.volumes) + row('Redes', d.networks) + '</tbody></table>');
    }).catch(function(e) { if (r) r.innerHTML = banner('Error: ' + (e.message || e), '#ffebee', '#c62828'); });
  }

  /* ── S3 Backup ── */
  function viewBackup(el) {
    el.innerHTML = heading('Respaldo S3') +
      btn('bk-now', 'Respaldar Ahora', '#2e7d32') +
      '<div id="bk-status" style="margin-top:12px"></div>' +
      '<div id="bk-list" style="margin-top:16px">' + loading() + '</div>';
    el.querySelector('#bk-now').onclick = function() {
      var b = this; b.disabled = true; b.textContent = 'Respaldando...';
      document.getElementById('bk-status').innerHTML = loading();
      api('POST', '/api/backup/s3').then(function() {
        b.disabled = false; b.textContent = 'RESPALDAR AHORA';
        document.getElementById('bk-status').innerHTML = banner('Respaldo creado exitosamente.', '#e8f5e9', '#2e7d32');
        loadBackups();
      }).catch(function(e) {
        b.disabled = false; b.textContent = 'RESPALDAR AHORA';
        document.getElementById('bk-status').innerHTML = banner('Error: ' + (e.message || e), '#ffebee', '#c62828');
      });
    };
    loadBackups();
  }
  function loadBackups() {
    api('GET', '/api/backup/s3').then(function(data) {
      var el = document.getElementById('bk-list');
      if (!el) return;
      if (!data || !data.length) { el.innerHTML = '<p style="color:#999">Sin respaldos disponibles.</p>'; return; }
      var fmt = function(b) { return b >= 1e9 ? (b / 1e9).toFixed(1) + ' GB' : b >= 1e6 ? (b / 1e6).toFixed(1) + ' MB' : (b / 1e3).toFixed(0) + ' KB'; };
      var rows = data.map(function(bk) {
        return [bk.key, bk.date || bk.lastModified || '-', bk.size ? fmt(bk.size) : '-',
          btn('', 'Restaurar', '#e65100').replace('id=""', 'data-restore="' + bk.key + '"')];
      });
      el.innerHTML = tbl(['Clave', 'Fecha', 'Tamaño', 'Acciones'], rows);
      el.querySelectorAll('[data-restore]').forEach(function(b) {
        b.onclick = function() {
          if (confirm('¿Restaurar desde este respaldo? Los datos actuales serán reemplazados.'))  {
            b.disabled = true; b.textContent = 'Restaurando...';
            api('POST', '/api/restore/s3', { key: b.dataset.restore }).then(function() {
              alert('Restauración completada.'); loadBackups();
            }).catch(function(e) { alert('Error: ' + (e.message || e)); b.disabled = false; b.textContent = 'RESTAURAR'; });
          }
        };
      });
    }).catch(function(e) { var el = document.getElementById('bk-list'); if (el) el.innerHTML = banner('Error: ' + (e.message || e), '#ffebee', '#c62828'); });
  }

  /* ── Init ── */
  window.addEventListener('hashchange', function() { setTimeout(renderPage, 300); });
  setInterval(function() {
    if (!getToken()) return;
    injectSidebar();
    ensureToolsAtEnd();
  }, 2000);
  var _fi = setInterval(function() {
    if (!getToken()) return;
    injectSidebar();
    renderPage();
    if (document.getElementById('xpx-tools')) clearInterval(_fi);
  }, 300);
})();
