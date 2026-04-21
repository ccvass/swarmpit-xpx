import { List, Datagrid, TextField, FunctionField, Show, SimpleShowLayout, useRecordContext, TopToolbar, DeleteButton, FilterButton, TextInput, SearchInput } from 'react-admin';
import { Chip, Typography, Box, Card, CardContent, Grid, Table, TableHead, TableRow, TableCell, TableBody, Tabs, Tab } from '@mui/material';
import { useState, useEffect } from 'react';

const bytes = b => b > 1e9 ? (b/1e9).toFixed(1)+' GB' : b > 1e6 ? (b/1e6).toFixed(0)+' MB' : '-';
const ago = d => { if(!d)return'-';const s=Math.floor((Date.now()-new Date(d))/1000);return s<60?s+'s':s<3600?Math.floor(s/60)+'m':s<86400?Math.floor(s/3600)+'h':Math.floor(s/86400)+'d' };
const Badge = ({ status }) => {
  const s = (status||'').toLowerCase();
  const color = s.includes('running')||s==='ready'||s==='active'?'success':s.includes('shutdown')||s==='down'||s==='drain'||s.includes('fail')?'error':s==='leader'||s==='reachable'?'info':'default';
  return <Chip label={status||'-'} color={color} size="small" variant="outlined" />;
};

const filters = [<SearchInput source="q" alwaysOn />];

// ── Services ──
export const ServiceList = () => (
  <List filters={filters} sort={{ field: 'Spec.Name', order: 'ASC' }}>
    <Datagrid rowClick="show" bulkActionButtons={false}>
      <FunctionField label="Name" render={r => r.Spec?.Name} />
      <FunctionField label="Image" render={r => r.Spec?.TaskTemplate?.ContainerSpec?.Image?.split('@')[0]?.replace(/^.*\//, '')} />
      <FunctionField label="Mode" render={r => r.Spec?.Mode?.Replicated ? 'Replicated' : 'Global'} />
      <FunctionField label="Replicas" render={r => r.Spec?.Mode?.Replicated?.Replicas || '-'} />
      <FunctionField label="Stack" render={r => r.Spec?.Labels?.['com.docker.stack.namespace'] || '-'} />
      <FunctionField label="Updated" render={r => ago(r.UpdatedAt) + ' ago'} />
    </Datagrid>
  </List>
);

export const ServiceShow = () => {
  const [tab, setTab] = useState(0);
  const [tasks, setTasks] = useState([]);
  const [logs, setLogs] = useState('');
  return (
    <Show actions={<TopToolbar><DeleteButton /></TopToolbar>}>
      <ServiceDetail tab={tab} setTab={setTab} tasks={tasks} setTasks={setTasks} logs={logs} setLogs={setLogs} />
    </Show>
  );
};

const ServiceDetail = ({ tab, setTab, tasks, setTasks, logs, setLogs }) => {
  const record = useRecordContext();
  useEffect(() => {
    if (!record) return;
    fetch('/api/tasks', { headers: { Authorization: localStorage.getItem('token') } })
      .then(r => r.json()).then(d => setTasks((d||[]).filter(t => t.ServiceID === record.ID).sort((a,b) => (b.UpdatedAt||'').localeCompare(a.UpdatedAt||''))));
  }, [record?.ID]);
  const loadLogs = () => fetch(`/api/services/${record.ID}/logs?tail=200`, { headers: { Authorization: localStorage.getItem('token') } }).then(r => r.json()).then(d => setLogs(d?.logs || 'No logs'));
  if (!record) return null;
  const s = record.Spec || {}, cs = s.TaskTemplate?.ContainerSpec || {}, res = s.TaskTemplate?.Resources || {};
  const env = (cs.Env || []).map(e => { const [k,...v] = e.split('='); return { k, v: v.join('=') }; });
  const ports = (record.Endpoint?.Ports || []);
  const mounts = (cs.Mounts || []);
  const labels = Object.entries(s.Labels || {});
  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" sx={{ mb: 2, fontWeight: 600 }}>{s.Name}</Typography>
      <Grid container spacing={2} sx={{ mb: 2 }}>
        {[['Image', cs.Image?.split('@')[0]], ['Mode', s.Mode?.Replicated ? `Replicated (${s.Mode.Replicated.Replicas})` : 'Global'],
          ['CPU Limit', res.Limits?.NanoCPUs ? (res.Limits.NanoCPUs/1e9).toFixed(2)+' cores' : '-'], ['Memory Limit', bytes(res.Limits?.MemoryBytes)],
          ['Updated', new Date(record.UpdatedAt).toLocaleString()], ['Stack', s.Labels?.['com.docker.stack.namespace'] || '-']
        ].map(([l, v]) => (
          <Grid item xs={6} md={4} key={l}><Card variant="outlined"><CardContent><Typography variant="caption" color="textSecondary">{l}</Typography><Typography variant="body1" sx={{ fontWeight: 500 }}>{v || '-'}</Typography></CardContent></Card></Grid>
        ))}
      </Grid>
      {ports.length > 0 && <Card variant="outlined" sx={{ mb: 2 }}><CardContent><Typography variant="subtitle2" sx={{ mb: 1 }}>Ports</Typography>
        <Table size="small"><TableHead><TableRow><TableCell>Published</TableCell><TableCell>Target</TableCell><TableCell>Protocol</TableCell><TableCell>Mode</TableCell></TableRow></TableHead>
        <TableBody>{ports.map((p, i) => <TableRow key={i}><TableCell>{p.PublishedPort}</TableCell><TableCell>{p.TargetPort}</TableCell><TableCell>{p.Protocol}</TableCell><TableCell>{p.PublishMode}</TableCell></TableRow>)}</TableBody></Table>
      </CardContent></Card>}
      {mounts.length > 0 && <Card variant="outlined" sx={{ mb: 2 }}><CardContent><Typography variant="subtitle2" sx={{ mb: 1 }}>Mounts</Typography>
        <Table size="small"><TableHead><TableRow><TableCell>Type</TableCell><TableCell>Source</TableCell><TableCell>Target</TableCell><TableCell>RO</TableCell></TableRow></TableHead>
        <TableBody>{mounts.map((m, i) => <TableRow key={i}><TableCell>{m.Type}</TableCell><TableCell>{m.Source}</TableCell><TableCell>{m.Target}</TableCell><TableCell>{m.ReadOnly ? 'Yes' : 'No'}</TableCell></TableRow>)}</TableBody></Table>
      </CardContent></Card>}
      {env.length > 0 && <Card variant="outlined" sx={{ mb: 2 }}><CardContent><Typography variant="subtitle2" sx={{ mb: 1 }}>Environment ({env.length})</Typography>
        <Table size="small"><TableBody>{env.map((e, i) => <TableRow key={i}><TableCell sx={{ fontFamily: 'monospace', fontSize: '.8rem', width: '30%' }}>{e.k}</TableCell><TableCell sx={{ fontFamily: 'monospace', fontSize: '.8rem' }}>{e.v}</TableCell></TableRow>)}</TableBody></Table>
      </CardContent></Card>}
      {labels.length > 0 && <Card variant="outlined" sx={{ mb: 2 }}><CardContent><Typography variant="subtitle2" sx={{ mb: 1 }}>Labels ({labels.length})</Typography>
        <Table size="small"><TableBody>{labels.map(([k, v], i) => <TableRow key={i}><TableCell sx={{ fontFamily: 'monospace', fontSize: '.78rem', width: '40%' }}>{k}</TableCell><TableCell sx={{ fontSize: '.78rem' }}>{v}</TableCell></TableRow>)}</TableBody></Table>
      </CardContent></Card>}
      <Tabs value={tab} onChange={(_, v) => { setTab(v); if (v === 1) loadLogs(); }} sx={{ mb: 1 }}>
        <Tab label={`Tasks (${tasks.length})`} /><Tab label="Logs" />
      </Tabs>
      {tab === 0 && <Table size="small"><TableHead><TableRow><TableCell>ID</TableCell><TableCell>Node</TableCell><TableCell>State</TableCell><TableCell>Message</TableCell><TableCell>Updated</TableCell></TableRow></TableHead>
        <TableBody>{tasks.map(t => <TableRow key={t.ID}><TableCell>{t.ID?.slice(0,12)}</TableCell><TableCell>{t.NodeID?.slice(0,12)}</TableCell><TableCell><Badge status={t.Status?.State} /></TableCell><TableCell>{t.Status?.Message}</TableCell><TableCell>{ago(t.UpdatedAt)} ago</TableCell></TableRow>)}</TableBody></Table>}
      {tab === 1 && <Box sx={{ background: '#1a1a2e', color: '#0f0', p: 2, borderRadius: 1, fontFamily: 'monospace', fontSize: '.75rem', maxHeight: 400, overflow: 'auto', whiteSpace: 'pre-wrap' }}>{logs || 'Loading...'}</Box>}
    </Box>
  );
};

// ── Nodes ──
export const NodeList = () => (
  <List filters={filters}>
    <Datagrid rowClick="show" bulkActionButtons={false}>
      <FunctionField label="Hostname" render={r => r.Description?.Hostname} />
      <FunctionField label="Role" render={r => r.ManagerStatus ? 'Manager' : 'Worker'} />
      <FunctionField label="State" render={r => <Badge status={r.Status?.State} />} />
      <FunctionField label="Availability" render={r => <Badge status={r.Spec?.Availability} />} />
      <FunctionField label="Manager" render={r => r.ManagerStatus ? <Badge status={r.ManagerStatus.Leader ? 'Leader' : r.ManagerStatus.Reachability} /> : '-'} />
      <FunctionField label="CPU" render={r => r.Description?.Resources?.NanoCPUs ? (r.Description.Resources.NanoCPUs/1e9).toFixed(0)+' cores' : '-'} />
      <FunctionField label="Memory" render={r => bytes(r.Description?.Resources?.MemoryBytes)} />
      <FunctionField label="Engine" render={r => r.Description?.Engine?.EngineVersion} />
    </Datagrid>
  </List>
);

export const NodeShow = () => (
  <Show><NodeDetail /></Show>
);

const NodeDetail = () => {
  const record = useRecordContext();
  if (!record) return null;
  const d = record.Description || {}, res = d.Resources || {}, tasks = record.tasks || [];
  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" sx={{ mb: 2, fontWeight: 600 }}>{d.Hostname}</Typography>
      <Grid container spacing={2} sx={{ mb: 2 }}>
        {[['Role', record.ManagerStatus ? 'Manager' : 'Worker'], ['State', record.Status?.State], ['Availability', record.Spec?.Availability],
          ['CPU', res.NanoCPUs ? (res.NanoCPUs/1e9).toFixed(0)+' cores' : '-'], ['Memory', bytes(res.MemoryBytes)],
          ['Engine', d.Engine?.EngineVersion], ['OS', `${d.Platform?.OS} ${d.Platform?.Architecture}`], ['Address', record.Status?.Addr],
        ].map(([l, v]) => (
          <Grid item xs={6} md={3} key={l}><Card variant="outlined"><CardContent><Typography variant="caption" color="textSecondary">{l}</Typography><Typography variant="body1" sx={{ fontWeight: 500 }}>{v || '-'}</Typography></CardContent></Card></Grid>
        ))}
      </Grid>
      {record.ManagerStatus && <Card variant="outlined" sx={{ mb: 2 }}><CardContent>
        <Typography variant="subtitle2">Manager Status</Typography>
        <Typography>Reachability: <Badge status={record.ManagerStatus.Reachability} /> {record.ManagerStatus.Leader && <Chip label="Leader" color="primary" size="small" />}</Typography>
        {record.ManagerStatus.Addr && <Typography variant="body2" color="textSecondary">Address: {record.ManagerStatus.Addr}</Typography>}
      </CardContent></Card>}
      <Typography variant="h6" sx={{ mb: 1 }}>Tasks on this node ({tasks.length})</Typography>
      <Table size="small"><TableHead><TableRow><TableCell>ID</TableCell><TableCell>Service</TableCell><TableCell>State</TableCell><TableCell>Image</TableCell><TableCell>Updated</TableCell></TableRow></TableHead>
        <TableBody>{tasks.map(t => <TableRow key={t.ID}><TableCell>{t.ID?.slice(0,12)}</TableCell><TableCell>{t.ServiceID?.slice(0,12)}</TableCell><TableCell><Badge status={t.Status?.State} /></TableCell><TableCell>{t.Spec?.ContainerSpec?.Image?.split('@')[0]?.split('/').pop()}</TableCell><TableCell>{ago(t.UpdatedAt)} ago</TableCell></TableRow>)}</TableBody></Table>
    </Box>
  );
};

// ── Simple lists ──
export const StackList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <TextField source="stackName" label="Name" />
    <TextField source="serviceCount" label="Services" />
  </Datagrid></List>
);

export const NetworkList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <FunctionField label="Name" render={r => r.Name} />
    <FunctionField label="Driver" render={r => r.Driver} />
    <FunctionField label="Scope" render={r => r.Scope} />
    <FunctionField label="Created" render={r => ago(r.Created) + ' ago'} />
  </Datagrid></List>
);

export const VolumeList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <FunctionField label="Name" render={r => r.Name} />
    <FunctionField label="Driver" render={r => r.Driver} />
    <FunctionField label="Scope" render={r => r.Scope} />
  </Datagrid></List>
);

export const SecretList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <FunctionField label="Name" render={r => r.Spec?.Name} />
    <FunctionField label="Created" render={r => new Date(r.CreatedAt).toLocaleString()} />
    <FunctionField label="Updated" render={r => ago(r.UpdatedAt) + ' ago'} />
  </Datagrid></List>
);

export const ConfigList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <FunctionField label="Name" render={r => r.Spec?.Name} />
    <FunctionField label="Created" render={r => new Date(r.CreatedAt).toLocaleString()} />
  </Datagrid></List>
);

export const TaskList = () => (
  <List filters={filters}><Datagrid bulkActionButtons={false}>
    <FunctionField label="ID" render={r => r.ID?.slice(0,12)} />
    <FunctionField label="Service" render={r => r.ServiceID?.slice(0,12)} />
    <FunctionField label="Node" render={r => r.NodeID?.slice(0,12)} />
    <FunctionField label="State" render={r => <Badge status={r.Status?.State} />} />
    <FunctionField label="Updated" render={r => ago(r.UpdatedAt) + ' ago'} />
  </Datagrid></List>
);

export const AuditList = () => (
  <List><Datagrid bulkActionButtons={false}>
    <FunctionField label="Time" render={r => new Date(r.timestamp * 1000).toLocaleString()} />
    <TextField source="username" />
    <TextField source="action" />
    <TextField source="resource_name" label="Resource" />
  </Datagrid></List>
);
