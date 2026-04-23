import { useEffect, useState } from 'react';
import { Card, CardContent, Typography, Box, Grid } from '@mui/material';
import { ImageUpdatesDashboard, PruneDashboard, S3BackupDashboard } from './features';

const headers = () => ({ Authorization: localStorage.getItem('token') });

const safe = (fn, fallback = 0) => { try { return fn(); } catch { return fallback; } };

export default function Dashboard() {
  const [stats, setStats] = useState(null);

  useEffect(() => {
    Promise.all([
      fetch('/api/nodes', { headers: headers() }).then(r => r.json()).catch(() => []),
      fetch('/api/services', { headers: headers() }).then(r => r.json()).catch(() => []),
      fetch('/api/tasks', { headers: headers() }).then(r => r.json()).catch(() => []),
    ]).then(([nodes, services, tasks]) => {
      const n = Array.isArray(nodes) ? nodes : [];
      const s = Array.isArray(services) ? services : [];
      const t = Array.isArray(tasks) ? tasks : [];
      setStats({ nodes: n.length, services: s.length, tasks: t.length });
    });
  }, []);

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" sx={{ mb: 2, fontWeight: 600 }}>Dashboard</Typography>
      {stats && (
        <Grid container spacing={2} sx={{ mb: 3 }}>
          {[
            ['Nodes', stats.nodes],
            ['Services', stats.services],
            ['Tasks', stats.tasks],
          ].map(([label, value]) => (
            <Grid item xs={6} sm={4} key={label}>
              <Card><CardContent>
                <Typography variant="caption" color="textSecondary">{label}</Typography>
                <Typography variant="h4" sx={{ fontWeight: 700 }}>{value}</Typography>
              </CardContent></Card>
            </Grid>
          ))}
        </Grid>
      )}
      <ImageUpdatesDashboard />
      <PruneDashboard />
      <S3BackupDashboard />
    </Box>
  );
}
