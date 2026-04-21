import { useEffect, useState } from 'react';
import { Card, CardContent, Typography, Box, Grid } from '@mui/material';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts';

const COLORS = ['#4caf50', '#f44336', '#ff9800', '#2196f3', '#9c27b0'];
const headers = () => ({ Authorization: localStorage.getItem('token') });

export default function Dashboard() {
  const [nodes, setNodes] = useState([]);
  const [services, setServices] = useState([]);
  const [tasks, setTasks] = useState([]);

  useEffect(() => {
    fetch('/api/nodes', { headers: headers() }).then(r => r.json()).then(d => setNodes(d || []));
    fetch('/api/services', { headers: headers() }).then(r => r.json()).then(d => setServices(d || []));
    fetch('/api/tasks', { headers: headers() }).then(r => r.json()).then(d => setTasks(d || []));
  }, []);

  const nodeStates = nodes.reduce((a, n) => { const s = n.Status?.State || 'unknown'; a[s] = (a[s] || 0) + 1; return a; }, {});
  const taskStates = tasks.reduce((a, t) => { const s = t.Status?.State || 'unknown'; a[s] = (a[s] || 0) + 1; return a; }, {});
  const stacks = new Set(services.map(s => s.Spec?.Labels?.['com.docker.stack.namespace']).filter(Boolean));
  const totalCPU = nodes.reduce((a, n) => a + (n.Description?.Resources?.NanoCPUs || 0) / 1e9, 0);
  const totalMem = nodes.reduce((a, n) => a + (n.Description?.Resources?.MemoryBytes || 0), 0);
  const managers = nodes.filter(n => n.ManagerStatus).length;

  const pieData = (obj) => Object.entries(obj).map(([name, value]) => ({ name, value }));

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" sx={{ mb: 2, fontWeight: 600 }}>Dashboard</Typography>
      <Grid container spacing={2} sx={{ mb: 3 }}>
        {[
          ['Nodes', nodes.length, `${managers} managers`],
          ['Services', services.length, `${stacks.size} stacks`],
          ['Tasks', tasks.length, `${taskStates.running || 0} running`],
          ['CPU Cores', totalCPU.toFixed(0), 'total cluster'],
          ['Memory', (totalMem / 1e9).toFixed(1) + ' GB', 'total cluster'],
        ].map(([label, value, sub]) => (
          <Grid item xs={6} sm={4} md={2.4} key={label}>
            <Card><CardContent>
              <Typography variant="caption" color="textSecondary">{label}</Typography>
              <Typography variant="h4" sx={{ fontWeight: 700 }}>{value}</Typography>
              <Typography variant="caption" color="textSecondary">{sub}</Typography>
            </CardContent></Card>
          </Grid>
        ))}
      </Grid>
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <Card><CardContent>
            <Typography variant="h6" sx={{ mb: 1 }}>Node Status</Typography>
            <ResponsiveContainer width="100%" height={200}>
              <PieChart><Pie data={pieData(nodeStates)} dataKey="value" nameKey="name" cx="50%" cy="50%" outerRadius={70} label={({ name, value }) => `${name}: ${value}`}>
                {pieData(nodeStates).map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
              </Pie><Tooltip /></PieChart>
            </ResponsiveContainer>
          </CardContent></Card>
        </Grid>
        <Grid item xs={12} md={6}>
          <Card><CardContent>
            <Typography variant="h6" sx={{ mb: 1 }}>Task Status</Typography>
            <ResponsiveContainer width="100%" height={200}>
              <PieChart><Pie data={pieData(taskStates)} dataKey="value" nameKey="name" cx="50%" cy="50%" outerRadius={70} label={({ name, value }) => `${name}: ${value}`}>
                {pieData(taskStates).map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
              </Pie><Tooltip /></PieChart>
            </ResponsiveContainer>
          </CardContent></Card>
        </Grid>
      </Grid>
    </Box>
  );
}
