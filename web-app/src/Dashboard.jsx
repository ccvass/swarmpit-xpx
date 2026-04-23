import { useEffect, useState } from 'react';
import { Card, CardContent, Typography, Box, Grid, Button, Alert } from '@mui/material';

const headers = () => ({ Authorization: localStorage.getItem('token'), 'Content-Type': 'application/json' });

export default function Dashboard() {
  const [stats, setStats] = useState(null);
  const [updates, setUpdates] = useState(null);
  const [pruneResult, setPruneResult] = useState(null);
  const [loading, setLoading] = useState({});

  useEffect(() => {
    Promise.all([
      fetch('/api/nodes', { headers: headers() }).then(r => r.ok ? r.json() : []).catch(() => []),
      fetch('/api/services', { headers: headers() }).then(r => r.ok ? r.json() : []).catch(() => []),
      fetch('/api/tasks', { headers: headers() }).then(r => r.ok ? r.json() : []).catch(() => []),
    ]).then(([nodes, services, tasks]) => {
      setStats({
        nodes: Array.isArray(nodes) ? nodes.length : 0,
        services: Array.isArray(services) ? services.length : 0,
        tasks: Array.isArray(tasks) ? tasks.length : 0,
      });
    });
  }, []);

  const checkUpdates = async () => {
    setLoading(l => ({ ...l, updates: true }));
    try {
      const res = await fetch('/api/services/check-updates', { method: 'POST', headers: headers() });
      setUpdates(res.ok ? await res.json() : []);
    } catch { setUpdates([]); }
    setLoading(l => ({ ...l, updates: false }));
  };

  const runPrune = async (dryRun) => {
    setLoading(l => ({ ...l, prune: true }));
    try {
      const res = await fetch('/api/system/prune', {
        method: 'POST', headers: headers(),
        body: JSON.stringify({ images: true, volumes: true, networks: true, dryRun }),
      });
      setPruneResult(res.ok ? await res.json() : null);
    } catch { setPruneResult(null); }
    setLoading(l => ({ ...l, prune: false }));
  };

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" sx={{ mb: 2, fontWeight: 600 }}>Dashboard</Typography>

      {stats && (
        <Grid container spacing={2} sx={{ mb: 3 }}>
          {[['Nodes', stats.nodes], ['Services', stats.services], ['Tasks', stats.tasks]].map(([label, value]) => (
            <Grid item xs={6} sm={4} key={label}>
              <Card><CardContent>
                <Typography variant="caption" color="textSecondary">{label}</Typography>
                <Typography variant="h4" sx={{ fontWeight: 700 }}>{value}</Typography>
              </CardContent></Card>
            </Grid>
          ))}
        </Grid>
      )}

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>Image Updates</Typography>
          <Button variant="contained" size="small" disabled={loading.updates} onClick={checkUpdates}>
            {loading.updates ? 'Checking...' : 'Check Now'}
          </Button>
          {updates && updates.length === 0 && <Alert severity="success" sx={{ mt: 1 }}>All images up to date</Alert>}
          {updates && updates.length > 0 && (
            <Box sx={{ mt: 1 }}>
              <Alert severity="warning">{updates.length} update(s) available</Alert>
              {updates.map((u, i) => <Typography key={i} variant="body2" sx={{ mt: 0.5 }}>{u.serviceName}: {u.image}</Typography>)}
            </Box>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>System Cleanup</Typography>
          <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
            <Button variant="outlined" size="small" disabled={loading.prune} onClick={() => runPrune(true)}>Preview</Button>
            <Button variant="contained" color="error" size="small" disabled={loading.prune}
              onClick={() => { if (confirm('Remove all unused resources?')) runPrune(false); }}>Prune Now</Button>
          </Box>
          {pruneResult && (
            <Box>
              <Alert severity={pruneResult.dryRun ? 'info' : 'success'} sx={{ mb: 1 }}>
                {pruneResult.dryRun ? 'Preview — no changes made' : 'Cleanup completed'}
              </Alert>
              <Typography variant="body2">Images: {pruneResult.images?.count || 0} ({((pruneResult.images?.spaceReclaimed || 0) / 1e9).toFixed(1)} GB)</Typography>
              <Typography variant="body2">Volumes: {pruneResult.volumes?.count || 0}</Typography>
              <Typography variant="body2">Networks: {pruneResult.networks?.count || 0}</Typography>
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
