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

/* === XPX Features === */
(function(){
var ICONS={
gitops:'M6 2a4 4 0 00-4 4c0 1.82 1.22 3.35 2.88 3.84A3.99 3.99 0 006 14a4 4 0 003.88-3.04A4.01 4.01 0 0014 10a4 4 0 10-4-4c0 .73.2 1.41.55 2H9.45c.35-.59.55-1.27.55-2a4 4 0 00-4-4zm0 2a2 2 0 110 4 2 2 0 010-4zm8 0a2 2 0 110 4 2 2 0 010-4zM6 10a2 2 0 110 4 2 2 0 010-4z',
updates:'M21 10.12h-6.78l2.74-2.82c-2.73-2.7-7.15-2.8-9.88-.1-2.73 2.71-2.73 7.08 0 9.79s7.15 2.71 9.88 0C18.32 15.65 19 14.08 19 12.1h2c0 2.48-.94 4.96-2.82 6.86-3.72 3.72-9.76 3.72-13.48 0s-3.72-9.76 0-13.48c3.72-3.72 9.76-3.72 13.48 0L21 2v8.12z',
prune:'M15 16h4v2h-4zm0-8h7v2h-7zm0 4h6v2h-6zM3 18c0 1.1.9 2 2 2h6c1.1 0 2-.9 2-2V8H3v10zM14 5h-3l-1-1H6L5 5H2v2h12V5z',
backup:'M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM14 13v4h-4v-4H7l5-5 5 5h-3z',
clusters:'M17 16l-4-4V8.82C14.16 8.4 15 7.3 15 6c0-1.66-1.34-3-3-3S9 4.34 9 6c0 1.3.84 2.4 2 2.82V12l-4 4H2v5h5v-3.05l4-4.2 4 4.2V21h5v-5h-3z',
teams:'M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z',
alerts:'M12 22c1.1 0 2-.9 2-2h-4c0 1.1.89 2 2 2zm6-6v-5c0-3.07-1.64-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.63 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z',
audit:'M13 3c-4.97 0-9 4.03-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42C8.27 19.99 10.51 21 13 21c4.97 0 9-4.03 9-9s-4.03-9-9-9zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z'
};
var ITEMS=[
{id:'gitops',label:'GitOps',icon:ICONS.gitops},
{id:'updates',label:'Image Updates',icon:ICONS.updates},
{id:'prune',label:'System Prune',icon:ICONS.prune},
{id:'backup',label:'S3 Backup',icon:ICONS.backup},
{id:'clusters',label:'Clusters',icon:ICONS.clusters},
{id:'teams',label:'Teams',icon:ICONS.teams},
{id:'alerts',label:'Alerts',icon:ICONS.alerts},
{id:'audit',label:'Audit Log',icon:ICONS.audit}
];

function getToken(){try{var t=localStorage.getItem('token')||sessionStorage.getItem('token');return t?t.replace(/^"|"$/g,''):'';}catch(e){return '';}}
function api(method,path,body){
  var token=getToken();
  var opts={method:method,headers:{'Content-Type':'application/json'}};
  if(token)opts.headers['Authorization']=token;
  if(body)opts.body=JSON.stringify(body);
  return fetch(path,opts).then(function(r){return r.ok?r.json():r.json().then(function(e){throw e})});
}
function heading(t){return '<h5 style="margin:0 0 4px;font-size:1.25rem;font-weight:500">'+t+'</h5>';}
function desc(t){return '<p style="margin:0 0 16px;color:#666;font-size:.85rem">'+t+'</p>';}
function btn(id,label,color){return '<button id="'+id+'" style="padding:6px 16px;margin:4px;border:none;border-radius:4px;background:'+(color||'#1976d2')+';color:#fff;cursor:pointer;font-size:.875rem">'+label+'</button>';}
function loading(id){return '<div id="'+id+'" style="display:none;padding:8px"><em>Loading...</em></div>';}
function table(headers,rows){
  var h=headers.map(function(x){return '<th style="padding:8px 12px;text-align:left;border-bottom:2px solid #e0e0e0">'+x+'</th>'}).join('');
  var r=rows.map(function(row){return '<tr>'+row.map(function(c){return '<td style="padding:8px 12px;border-bottom:1px solid #eee">'+c+'</td>'}).join('')+'</tr>'}).join('');
  return '<table style="width:100%;border-collapse:collapse">'+h+r+'</table>';
}
function statusBadge(s){var c=s==='active'||s==='synced'||s==='up-to-date'?'#4caf50':s==='error'||s==='failed'?'#f44336':'#ff9800';return '<span style="color:'+c+';font-weight:500">'+s+'</span>';}

/* Sidebar injection */
function makeSidebarItem(item){
  return '<a class="MuiButtonBase-root MuiListItem-root Swarmpit-drawer-item MuiListItem-dense MuiListItem-gutters MuiListItem-button" href="#/xpx/'+item.id+'">'
    +'<div class="MuiListItemIcon-root Swarmpit-drawer-item-icon" color="primary">'
    +'<svg class="MuiSvgIcon-root" focusable="false" viewBox="0 0 24 24" aria-hidden="true" role="presentation"><path d="'+item.icon+'"/></svg>'
    +'</div><div class="MuiListItemText-root Swarmpit-drawer-item-text MuiListItemText-dense">'
    +'<h6 class="MuiTypography-root MuiTypography-subtitle2">'+item.label+'</h6></div></a>';
}
function makeSidebar(){
  var h='<li class="MuiListItem-root Swarmpit-drawer-category MuiListItem-gutters Mui-disabled" disabled="">'
    +'<div class="MuiListItemText-root Swarmpit-drawer-category-text">'
    +'<span class="MuiTypography-root MuiListItemText-primary MuiTypography-body1">TOOLS</span></div></li>';
  return h+ITEMS.map(makeSidebarItem).join('');
}
function injectSidebar(){
  if(document.getElementById('xpx-tools'))return ensureToolsAtEnd();
  var nav=document.querySelector('nav');if(!nav)return;
  var content=nav.querySelector('.Swarmpit-drawer-content');if(!content)return;
  var navList=content.children[1];if(!navList)return;
  var wrap=document.createElement('div');wrap.id='xpx-tools';wrap.innerHTML=makeSidebar();
  navList.appendChild(wrap);
}
function ensureToolsAtEnd(){
  var wrap=document.getElementById('xpx-tools');if(!wrap)return;
  var parent=wrap.parentNode;if(parent&&parent.lastElementChild!==wrap)parent.appendChild(wrap);
}
setInterval(injectSidebar,2000);

/* Page rendering */
function getPage(){
  var p=document.getElementById('xpx-page');
  if(!p){p=document.createElement('div');p.id='xpx-page';p.style.cssText='position:absolute;top:0;left:0;right:0;bottom:0;background:#fff;z-index:1200;padding:24px;overflow:auto';
    var main=document.querySelector('main')||document.body;main.style.position='relative';main.appendChild(p);}
  p.style.display='block';return p;
}
function hidePage(){var p=document.getElementById('xpx-page');if(p)p.style.display='none';}

function renderPage(){
  var hash=location.hash;
  if(hash.indexOf('#/xpx/')!==0){hidePage();return;}
  var route=hash.replace('#/xpx/','');
  var p=getPage();
  var views={gitops:viewGitOps,updates:viewUpdates,prune:viewPrune,backup:viewBackup,clusters:viewClusters,teams:viewTeams,alerts:viewAlerts,audit:viewAudit};
  if(views[route])views[route](p);else p.innerHTML=heading('Not Found');
}
window.addEventListener('hashchange',renderPage);

/* GitOps */
function viewGitOps(p){
  p.innerHTML=heading('GitOps')+desc('<b>Deploy Docker Swarm stacks directly from Git repositories.</b><br><br><b>How it works:</b> Add a Git repo containing a <code>docker-compose.yml</code>, and Swarmpit will deploy/update the stack on each sync.<br><br><b>Setup:</b><br>1. Enter a <b>Stack Name</b> (used as the Swarm stack name)<br>2. Provide the <b>Repository URL</b> (e.g., <code>https://github.com/org/repo.git</code> or <code>git@github.com:org/repo.git</code>)<br>3. Set the <b>Branch</b> (default: <code>main</code>)<br>4. Set the <b>Compose Path</b> (e.g., <code>docker-compose.yml</code> or <code>deploy/stack.yml</code>)<br>5. Set <b>Sync Interval</b> in seconds (<code>300</code> = every 5 min, <code>0</code> = manual only)<br><br><b>Webhook support:</b> Trigger sync via <code>POST /api/webhooks/git/{id}</code> from your CI/CD pipeline.<br><b>Private repos:</b> Configure credentials (username/token) when creating the repository entry.')+btn('go-sync-all','Refresh','#1976d2')+loading('go-load')
    +'<div id="go-list"></div><hr style="margin:16px 0">'
    +'<h6>Add Repository</h6>'
    +'<input id="go-stack" placeholder="Stack Name" style="margin:4px;padding:6px">'
    +'<input id="go-repo" placeholder="Repository URL" style="margin:4px;padding:6px;width:300px">'
    +'<input id="go-branch" placeholder="Branch" value="main" style="margin:4px;padding:6px">'
    +'<input id="go-path" placeholder="Compose Path" value="docker-compose.yml" style="margin:4px;padding:6px">'
    +'<input id="go-interval" placeholder="Sync Interval (s)" value="300" style="margin:4px;padding:6px;width:100px">'
    +btn('go-create','Create','#4caf50');
  loadGitOps();
  p.querySelector('#go-sync-all').onclick=loadGitOps;
  p.querySelector('#go-create').onclick=function(){
    api('POST','/api/gitops',{stackName:g('go-stack'),repoUrl:g('go-repo'),branch:g('go-branch'),composePath:g('go-path'),syncInterval:parseInt(g('go-interval')),credentials:{}})
      .then(loadGitOps).catch(alert);
  };
}
function g(id){return document.getElementById(id).value;}
function loadGitOps(){
  var el=document.getElementById('go-list');if(!el)return;el.innerHTML='<em>Loading...</em>';
  api('GET','/api/gitops').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No repositories configured.</p>';return;}
    el.innerHTML=table(['Stack','Repository','Branch','Status','Last Sync','Actions'],
      data.map(function(r){return [r.stackName,r.repoUrl,r.branch,statusBadge(r.status||'unknown'),r.lastSync||'-',
        btn('','Sync','#1976d2').replace('<button','<button onclick="window._goSync(\''+r.id+'\')"')
        +btn('','Delete','#f44336').replace('<button','<button onclick="window._goDel(\''+r.id+'\')"')
      ]}));
  }).catch(function(e){el.innerHTML='<p style="color:red">Error loading</p>';});
}
window._goSync=function(id){api('POST','/api/gitops/'+id+'/sync').then(loadGitOps).catch(alert);};
window._goDel=function(id){if(confirm('Delete this repo?'))api('DELETE','/api/gitops/'+id).then(loadGitOps).catch(alert);};

/* Image Updates */
function viewUpdates(p){
  p.innerHTML=heading('Image Updates')+desc('<b>Scan all running services for newer image versions in their registries.</b><br><br><b>How it works:</b> Compares the image digest of each running service against the latest available in the configured registry. Services with available updates are listed with their current and available status.<br><br><b>Prerequisites:</b> Registries must be configured under <a href="#/registries">Registries</a> (Docker Hub, GHCR, private registries, etc.).<br><br><b>Results:</b><br>• <span style="color:#4caf50;font-weight:500">up-to-date</span> — service is running the latest image<br>• <span style="color:#ff9800;font-weight:500">update available</span> — a newer image exists in the registry<br><br><b>Auto-check:</b> Runs periodically in the background. Click <b>Check Now</b> to trigger an immediate scan.')+btn('upd-check','Check Now','#1976d2')+loading('upd-load')+'<div id="upd-list"></div>';
  p.querySelector('#upd-check').onclick=function(){
    document.getElementById('upd-load').style.display='block';
    api('POST','/api/services/check-updates').then(function(){
      return api('GET','/api/services/update-status');
    }).then(function(data){
      document.getElementById('upd-load').style.display='none';
      var el=document.getElementById('upd-list');
      if(!data||!data.length){el.innerHTML='<p>All images are up to date.</p>';return;}
      el.innerHTML=table(['Service','Image','Status'],data.map(function(r){return [r.service,r.image,statusBadge(r.status||'unknown')]}));
    }).catch(function(e){document.getElementById('upd-load').style.display='none';alert(e);});
  };
}

/* System Prune */
function viewPrune(p){
  p.innerHTML=heading('System Prune')+desc('<b>Remove unused Docker resources to reclaim disk space across the cluster.</b><br><br><b>What gets removed:</b><br>• <b>Images</b> — dangling and unused images not referenced by any container<br>• <b>Volumes</b> — orphan volumes not attached to any container (<b>⚠️ unchecked by default</b> to prevent data loss)<br>• <b>Networks</b> — orphan networks not used by any service<br><br><b>Usage:</b><br>1. Select resource types to clean (checkboxes above)<br>2. Click <b>Preview</b> to see what <em>would</em> be removed — this is safe and makes no changes<br>3. Click <b>Prune Now</b> to execute — <b>this is destructive and cannot be undone</b><br><br><b>Results:</b> Shows count of removed resources and space reclaimed in MB.')
    +'<label><input type="checkbox" id="pr-img" checked> Images</label> '
    +'<label><input type="checkbox" id="pr-vol"> Volumes</label> '
    +'<label><input type="checkbox" id="pr-net" checked> Networks</label><br><br>'
    +btn('pr-preview','Preview','#ff9800')+btn('pr-run','Prune Now','#f44336')+loading('pr-load')+'<div id="pr-result"></div>';
  p.querySelector('#pr-preview').onclick=function(){doPrune(true)};
  p.querySelector('#pr-run').onclick=function(){if(confirm('This will remove unused resources. Continue?'))doPrune(false)};
}
function doPrune(dry){
  var el=document.getElementById('pr-result');el.innerHTML='<em>Working...</em>';
  api('POST','/api/system/prune',{images:document.getElementById('pr-img').checked,volumes:document.getElementById('pr-vol').checked,networks:document.getElementById('pr-net').checked,dryRun:dry})
    .then(function(data){
      var label=dry?'Preview:':'Done:';var lines=[];
      if(data.images)lines.push('Images: '+data.images.count+' removed'+(data.images.spaceReclaimed?' ('+(data.images.spaceReclaimed/1048576).toFixed(1)+' MB)':''));
      if(data.volumes)lines.push('Volumes: '+data.volumes.count+' removed'+(data.volumes.spaceReclaimed?' ('+(data.volumes.spaceReclaimed/1048576).toFixed(1)+' MB)':''));
      if(data.networks)lines.push('Networks: '+data.networks.count+' removed');
      el.innerHTML=(dry?'<strong>Preview:</strong><br>':'<strong>Done:</strong><br>')+lines.join('<br>');
    }).catch(function(e){el.innerHTML='<p style="color:red">Error: '+e+'</p>';});
}

/* S3 Backup */
function viewBackup(p){
  p.innerHTML=heading('S3 Backup')+desc('<b>Backup and restore the Swarmpit SQLite database to S3-compatible storage.</b><br><br><b>Required environment variables:</b><br>• <code>BACKUP_S3_ENDPOINT</code> — S3 endpoint URL (e.g., <code>https://s3.amazonaws.com</code> or <code>https://minio.example.com</code>)<br>• <code>BACKUP_S3_BUCKET</code> — bucket name (e.g., <code>swarmpit-backups</code>)<br>• <code>BACKUP_S3_REGION</code> — AWS region (e.g., <code>us-east-1</code>)<br>• <code>BACKUP_S3_ACCESS_KEY</code> — access key ID<br>• <code>BACKUP_S3_SECRET_KEY</code> — secret access key<br><br><b>Optional:</b><br>• <code>BACKUP_RETENTION_DAYS</code> — days to keep backups (default: <code>30</code>)<br><br><b>Usage:</b> Click <b>Backup Now</b> to create an immediate backup. Use <b>Restore</b> on any listed backup to restore the database to that point in time.')+btn('bk-now','Backup Now','#1976d2')+loading('bk-load')+'<div id="bk-list"></div>';
  loadBackups();
  p.querySelector('#bk-now').onclick=function(){
    document.getElementById('bk-load').style.display='block';
    api('POST','/api/backup/s3').then(function(){document.getElementById('bk-load').style.display='none';loadBackups();}).catch(function(e){document.getElementById('bk-load').style.display='none';alert(e);});
  };
}
function loadBackups(){
  var el=document.getElementById('bk-list');if(!el)return;el.innerHTML='<em>Loading...</em>';
  api('GET','/api/backup/s3').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No backups found.</p>';return;}
    el.innerHTML=table(['Key','Date','Size','Actions'],data.map(function(r){return [r.key,r.date||r.lastModified||'-',r.size||'-',
      btn('','Restore','#ff9800').replace('<button','<button onclick="window._bkRestore(\''+r.key+'\')"')]}));
  }).catch(function(e){
    var msg=(e&&(e.message||e.error||''))||'';
    if(msg.toLowerCase().indexOf('not configured')!==-1){
      el.innerHTML='<p style="color:#ff9800"><b>S3 backup is not configured.</b></p><p>Set the following environment variables on the Swarmpit service:</p><pre style="background:#f5f5f5;padding:12px;border-radius:4px;font-size:.85rem">'
        +'BACKUP_S3_ENDPOINT=https://s3.amazonaws.com\nBACKUP_S3_BUCKET=swarmpit-backups\nBACKUP_S3_REGION=us-east-1\nBACKUP_S3_ACCESS_KEY=your-access-key\nBACKUP_S3_SECRET_KEY=your-secret-key\nBACKUP_RETENTION_DAYS=30  # optional, default 30</pre>';
    } else {
      el.innerHTML='<p style="color:red">Error loading backups: '+(msg||'unknown error')+'</p>';
    }
  });
}
window._bkRestore=function(key){if(confirm('Restore from '+key+'?'))api('POST','/api/restore/s3',{key:key}).then(function(){alert('Restore initiated')}).catch(alert);};

/* Clusters */
function viewClusters(p){
  p.innerHTML=heading('Clusters')+desc('<b>Manage multiple Docker Swarm clusters from a single Swarmpit instance.</b><br><br><b>How it works:</b> Add remote clusters by providing their Docker API endpoint URL. Only one cluster is active at a time — switching clusters changes the context for all operations (services, stacks, nodes).<br><br><b>Setup:</b><br>1. Ensure the remote Docker daemon exposes its API (e.g., <code>https://remote-host:2376</code>)<br>2. Configure TLS certificates if required for remote access<br>3. Click <b>Add</b> with the cluster name and URL<br>4. Click <b>Activate</b> to switch to that cluster<br><br><b>Current cluster:</b> The active cluster is shown with a green status indicator.')+btn('cl-refresh','Refresh','#1976d2')+'<div id="cl-list"></div><hr style="margin:16px 0">'
    +'<h6>Add Cluster</h6>'
    +'<input id="cl-name" placeholder="Name" style="margin:4px;padding:6px">'
    +'<input id="cl-url" placeholder="URL" style="margin:4px;padding:6px;width:300px">'
    +btn('cl-create','Add','#4caf50');
  loadClusters();
  p.querySelector('#cl-refresh').onclick=loadClusters;
  p.querySelector('#cl-create').onclick=function(){
    api('POST','/api/clusters',{name:g('cl-name'),url:g('cl-url')}).then(loadClusters).catch(alert);
  };
}
function loadClusters(){
  var el=document.getElementById('cl-list');if(!el)return;el.innerHTML='<em>Loading...</em>';
  api('GET','/api/clusters').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No clusters configured.</p>';return;}
    el.innerHTML=table(['Name','URL','Status','Actions'],data.map(function(r){return [r.name,r.url,statusBadge(r.status||'unknown'),
      btn('','Activate','#4caf50').replace('<button','<button onclick="window._clAct(\''+r.id+'\')"')
      +btn('','Delete','#f44336').replace('<button','<button onclick="window._clDel(\''+r.id+'\')"')]}));
  }).catch(function(e){el.innerHTML='<p style="color:red">Error</p>';});
}
window._clAct=function(id){api('POST','/api/clusters/'+id+'/activate').then(loadClusters).catch(alert);};
window._clDel=function(id){if(confirm('Delete cluster?'))api('DELETE','/api/clusters/'+id).then(loadClusters).catch(alert);};

/* Teams */
function viewTeams(p){
  p.innerHTML=heading('Teams')+desc('<b>Role-Based Access Control (RBAC) via team-based permissions.</b><br><br><b>How it works:</b> Teams define stack-level permissions. Users assigned to a team inherit its access rights.<br><br><b>Permission model:</b><br>• Each team can be granted <b>read</b> (view) or <b>read/write</b> (manage) access per stack<br>• Users not in any team have no access to restricted stacks<br>• Admin users bypass team restrictions<br><br><b>Setup:</b><br>1. Create a team with a descriptive name (e.g., <code>backend-devs</code>)<br>2. Assign stack permissions to the team via the team detail view<br>3. Assign users to teams from the <a href="#/users">Users</a> management page<br><br><b>Tip:</b> Use teams to isolate environments (dev, staging, prod) or by project ownership.')+btn('tm-refresh','Refresh','#1976d2')+'<div id="tm-list"></div><hr style="margin:16px 0">'
    +'<h6>Create Team</h6>'
    +'<input id="tm-name" placeholder="Team Name" style="margin:4px;padding:6px">'
    +btn('tm-create','Create','#4caf50');
  loadTeams();
  p.querySelector('#tm-refresh').onclick=loadTeams;
  p.querySelector('#tm-create').onclick=function(){
    api('POST','/api/teams',{name:g('tm-name')}).then(loadTeams).catch(alert);
  };
}
function loadTeams(){
  var el=document.getElementById('tm-list');if(!el)return;el.innerHTML='<em>Loading...</em>';
  api('GET','/api/teams').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No teams.</p>';return;}
    el.innerHTML=table(['Name','Members','Actions'],data.map(function(r){return [r.name,(r.members||[]).length,
      btn('','Delete','#f44336').replace('<button','<button onclick="window._tmDel(\''+r.id+'\')"')]}));
  }).catch(function(e){el.innerHTML='<p style="color:red">Error</p>';});
}
window._tmDel=function(id){if(confirm('Delete team?'))api('DELETE','/api/teams/'+id).then(loadTeams).catch(alert);};

/* Alerts */
function viewAlerts(p){
  p.innerHTML=heading('Alerts')+desc('<b>Monitor service health and resource usage with configurable alert rules.</b><br><br><b>Rule types:</b><br>• <b>Service down</b> — triggers when a service has 0 running replicas<br>• <b>CPU threshold</b> — triggers when CPU usage exceeds a percentage (e.g., <code>cpu > 80</code>)<br>• <b>Memory threshold</b> — triggers when memory usage exceeds a limit (e.g., <code>memory > 512MB</code>)<br><br><b>Setup:</b><br>1. Create a rule with a name and condition expression<br>2. Configure notification channels (webhook, email) in settings<br>3. Rules are evaluated continuously against live metrics<br><br><b>History:</b> Click <b>History</b> to view all past triggered alerts with timestamps, rule names, and details. Useful for post-incident analysis.')+btn('al-refresh','Refresh','#1976d2')+btn('al-history','History','#ff9800')
    +'<div id="al-list"></div><div id="al-hist" style="display:none"></div><hr style="margin:16px 0">'
    +'<h6>Create Rule</h6>'
    +'<input id="al-name" placeholder="Rule Name" style="margin:4px;padding:6px">'
    +'<input id="al-cond" placeholder="Condition" style="margin:4px;padding:6px;width:300px">'
    +btn('al-create','Create','#4caf50');
  loadAlerts();
  p.querySelector('#al-refresh').onclick=function(){document.getElementById('al-hist').style.display='none';loadAlerts();};
  p.querySelector('#al-history').onclick=loadAlertHistory;
  p.querySelector('#al-create').onclick=function(){
    api('POST','/api/alerts',{name:g('al-name'),condition:g('al-cond')}).then(loadAlerts).catch(alert);
  };
}
function loadAlerts(){
  var el=document.getElementById('al-list');if(!el)return;el.innerHTML='<em>Loading...</em>';
  api('GET','/api/alerts').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No alert rules.</p>';return;}
    el.innerHTML=table(['Name','Condition','Status','Actions'],data.map(function(r){return [r.name,r.condition||'-',statusBadge(r.status||'active'),
      btn('','Delete','#f44336').replace('<button','<button onclick="window._alDel(\''+r.id+'\')"')]}));
  }).catch(function(e){el.innerHTML='<p style="color:red">Error</p>';});
}
function loadAlertHistory(){
  var el=document.getElementById('al-hist');el.style.display='block';el.innerHTML='<em>Loading history...</em>';
  api('GET','/api/alerts/history').then(function(data){
    if(!data||!data.length){el.innerHTML='<p>No alert history.</p>';return;}
    el.innerHTML='<h6>Alert History</h6>'+table(['Time','Rule','Status','Details'],data.map(function(r){return [r.timestamp||'-',r.rule||'-',statusBadge(r.status||'-'),r.details||'-']}));
  }).catch(function(e){el.innerHTML='<p style="color:red">Error</p>';});
}
window._alDel=function(id){if(confirm('Delete rule?'))api('DELETE','/api/alerts/'+id).then(loadAlerts).catch(alert);};

/* Audit Log */
function viewAudit(p){
  p.innerHTML=heading('Audit Log')+desc('<b>Chronological record of all user actions performed in the system.</b><br><br><b>What gets logged:</b><br>• Service deployments and updates<br>• Stack create/update/delete operations<br>• Configuration changes (secrets, configs, networks)<br>• System prune operations<br>• Backup and restore events<br>• User login/logout and permission changes<br><br><b>Each entry includes:</b> timestamp, username, action type, resource type, and resource name.<br><br><b>Use cases:</b> Compliance auditing, debugging deployment issues, tracking who changed what and when. Entries are immutable and cannot be deleted.')+'<div id="au-list"><em>Loading...</em></div>';
  api('GET','/api/audit').then(function(data){
    var el=document.getElementById('au-list');
    if(!data||!data.length){el.innerHTML='<p>No audit entries.</p>';return;}
    el.innerHTML=table(['Timestamp','User','Action','Resource Type','Resource Name'],data.map(function(r){
      var ts=r.timestamp?new Date(r.timestamp*1000).toLocaleString():'-';
      return [ts,r.username||'-',r.action||'-',r.resource_type||'-',r.resource_name||'-'];
    }));
  }).catch(function(e){document.getElementById('au-list').innerHTML='<p style="color:red">Error</p>';});
}

/* Init */
setTimeout(function(){injectSidebar();renderPage();},1000);
})();
