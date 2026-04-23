import { List, Datagrid, TextField, BooleanField, DateField, Show, SimpleShowLayout,
  Create, SimpleForm, TextInput, NumberInput, BooleanInput, Edit, useRecordContext,
  useNotify, useRefresh, Button, TopToolbar, FunctionField } from 'react-admin';
import SyncIcon from '@mui/icons-material/Sync';
import DeleteSweepIcon from '@mui/icons-material/DeleteSweep';
import BackupIcon from '@mui/icons-material/Backup';
import UpdateIcon from '@mui/icons-material/Update';
import { Card, CardContent, Typography, Box, Chip, Alert } from '@mui/material';
import { useState } from 'react';

const headers = () => ({ Authorization: localStorage.getItem('token'), 'Content-Type': 'application/json' });

/* ── GitOps ── */

const SyncButton = () => {
  const record = useRecordContext();
  const notify = useNotify();
  const refresh = useRefresh();
  return (
    <Button label="Sync" onClick={async (e) => {
      e.stopPropagation();
      await fetch(`/api/gitops/${record.id}/sync`, { method: 'POST', headers: headers() });
      notify('Sync triggered'); refresh();
    }}><SyncIcon /></Button>
  );
};

const GitOpsListActions = () => (
  <TopToolbar>
    <Button label="Check Image Updates" onClick={async () => {
      await fetch('/api/services/check-updates', { method: 'POST', headers: headers() });
    }}><UpdateIcon /></Button>
  </TopToolbar>
);

export const GitOpsList = () => (
  <List actions={<GitOpsListActions />}>
    <Datagrid rowClick="show">
      <TextField source="stackName" label="Stack" />
      <TextField source="repoUrl" label="Repository" />
      <TextField source="branch" />
      <TextField source="composePath" label="Compose File" />
      <FunctionField label="Sync" render={r => r.syncInterval > 0 ? `${r.syncInterval}s` : 'manual'} />
      <TextField source="lastHash" label="Hash" />
      <TextField source="lastSync" label="Last Sync" />
      <FunctionField label="Status" render={r => r.lastError ? <Chip label="error" color="error" size="small" /> : r.lastHash ? <Chip label="ok" color="success" size="small" /> : <Chip label="pending" size="small" />} />
      <BooleanField source="enabled" />
      <SyncButton />
    </Datagrid>
  </List>
);

export const GitOpsShow = () => (
  <Show>
    <SimpleShowLayout>
      <TextField source="stackName" />
      <TextField source="repoUrl" />
      <TextField source="branch" />
      <TextField source="composePath" />
      <TextField source="syncInterval" />
      <TextField source="lastHash" />
      <TextField source="lastSync" />
      <TextField source="lastError" />
      <BooleanField source="enabled" />
    </SimpleShowLayout>
  </Show>
);

export const GitOpsCreate = () => (
  <Create>
    <SimpleForm>
      <TextInput source="stackName" required />
      <TextInput source="repoUrl" required fullWidth />
      <TextInput source="branch" defaultValue="main" />
      <TextInput source="composePath" defaultValue="docker-compose.yml" fullWidth />
      <TextInput source="credentials" label="Token (for private repos)" type="password" fullWidth />
      <NumberInput source="syncInterval" label="Sync interval (seconds, 0=manual)" defaultValue={0} />
      <BooleanInput source="enabled" defaultValue={true} />
    </SimpleForm>
  </Create>
);

export const GitOpsEdit = () => (
  <Edit>
    <SimpleForm>
      <TextInput source="repoUrl" fullWidth />
      <TextInput source="branch" />
      <TextInput source="composePath" fullWidth />
      <TextInput source="credentials" type="password" fullWidth />
      <NumberInput source="syncInterval" />
      <BooleanInput source="enabled" />
    </SimpleForm>
  </Edit>
);

/* ── System Prune ── */

export const PruneDashboard = () => {
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);

  const runPrune = async (dryRun) => {
    setLoading(true);
    const res = await fetch('/api/system/prune', {
      method: 'POST', headers: headers(),
      body: JSON.stringify({ images: true, volumes: true, networks: true, dryRun })
    });
    setResult(await res.json());
    setLoading(false);
  };

  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Typography variant="h6" gutterBottom><DeleteSweepIcon sx={{ mr: 1, verticalAlign: 'middle' }} />System Cleanup</Typography>
        <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
          <Button label="Preview" onClick={() => runPrune(true)} disabled={loading} variant="outlined" />
          <Button label="Prune Now" onClick={() => { if (confirm('Remove all unused resources?')) runPrune(false); }} disabled={loading} color="error" />
        </Box>
        {result && (
          <Box>
            {result.dryRun && <Alert severity="info" sx={{ mb: 1 }}>Preview — no changes made</Alert>}
            {!result.dryRun && <Alert severity="success" sx={{ mb: 1 }}>Cleanup completed</Alert>}
            <Typography>Images: {result.images?.count || 0} ({((result.images?.spaceReclaimed || 0) / 1e9).toFixed(1)} GB)</Typography>
            <Typography>Volumes: {result.volumes?.count || 0}</Typography>
            <Typography>Networks: {result.networks?.count || 0}</Typography>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};

/* ── S3 Backup ── */

export const S3BackupDashboard = () => {
  const [backups, setBackups] = useState(null);
  const [status, setStatus] = useState('');

  const loadBackups = async () => {
    const res = await fetch('/api/backup/s3', { headers: headers() });
    const data = await res.json();
    if (data.error) { setStatus(data.error); setBackups(null); }
    else { setBackups(data); setStatus(''); }
  };

  const runBackup = async () => {
    setStatus('Backing up...');
    const res = await fetch('/api/backup/s3', { method: 'POST', headers: headers() });
    const data = await res.json();
    setStatus(data.error || `Backup saved: ${data.key}`);
    loadBackups();
  };

  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Typography variant="h6" gutterBottom><BackupIcon sx={{ mr: 1, verticalAlign: 'middle' }} />S3 Backups</Typography>
        <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
          <Button label="Backup Now" onClick={runBackup} />
          <Button label="List Backups" onClick={loadBackups} variant="outlined" />
        </Box>
        {status && <Alert severity="info" sx={{ mb: 1 }}>{status}</Alert>}
        {backups && backups.map((b, i) => <Typography key={i} variant="body2">{b.key}</Typography>)}
      </CardContent>
    </Card>
  );
};

/* ── Image Updates ── */

export const ImageUpdatesDashboard = () => {
  const [updates, setUpdates] = useState(null);
  const [loading, setLoading] = useState(false);

  const check = async () => {
    setLoading(true);
    const res = await fetch('/api/services/check-updates', { method: 'POST', headers: headers() });
    setUpdates(await res.json());
    setLoading(false);
  };

  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Typography variant="h6" gutterBottom><UpdateIcon sx={{ mr: 1, verticalAlign: 'middle' }} />Image Updates</Typography>
        <Button label={loading ? 'Checking...' : 'Check Now'} onClick={check} disabled={loading} />
        {updates && updates.length === 0 && <Alert severity="success" sx={{ mt: 1 }}>All images up to date</Alert>}
        {updates && updates.length > 0 && (
          <Box sx={{ mt: 1 }}>
            <Alert severity="warning">{updates.length} update(s) available</Alert>
            {updates.map((u, i) => (
              <Typography key={i} variant="body2" sx={{ mt: 0.5 }}>{u.serviceName}: {u.image}</Typography>
            ))}
          </Box>
        )}
      </CardContent>
    </Card>
  );
};
