const apiUrl = '';

const getToken = () => localStorage.getItem('token');
const headers = () => ({ Authorization: getToken(), 'Content-Type': 'application/json' });

const resourceMap = {
  services: { endpoint: '/api/services', getId: r => r.ID, getName: r => r.Spec?.Name },
  nodes: { endpoint: '/api/nodes', getId: r => r.ID, getName: r => r.Description?.Hostname },
  tasks: { endpoint: '/api/tasks', getId: r => r.ID },
  networks: { endpoint: '/api/networks', getId: r => r.Id || r.ID, getName: r => r.Name },
  volumes: { endpoint: '/api/volumes', getId: r => r.Name, getName: r => r.Name },
  secrets: { endpoint: '/api/secrets', getId: r => r.ID, getName: r => r.Spec?.Name },
  configs: { endpoint: '/api/configs', getId: r => r.ID, getName: r => r.Spec?.Name },
  stacks: { endpoint: '/api/stacks', getId: r => r.stackName, getName: r => r.stackName },
  audit: { endpoint: '/api/audit', getId: r => r.id },
  gitops: { endpoint: '/api/gitops', getId: r => r.id },
};

export default {
  getList: async (resource, params) => {
    const map = resourceMap[resource];
    if (!map) return { data: [], total: 0 };
    const res = await fetch(map.endpoint, { headers: headers() });
    if (!res.ok) return { data: [], total: 0 };
    let data = await res.json();
    if (!Array.isArray(data)) data = data?.Volumes || data?.services || [];
    data = data.map(r => ({ ...r, id: map.getId(r) }));
    // Client-side filter
    if (params.filter?.q) {
      const q = params.filter.q.toLowerCase();
      data = data.filter(r => JSON.stringify(r).toLowerCase().includes(q));
    }
    // Client-side sort
    if (params.sort?.field) {
      const f = params.sort.field;
      const dir = params.sort.order === 'ASC' ? 1 : -1;
      data.sort((a, b) => (JSON.stringify(a[f]) || '').localeCompare(JSON.stringify(b[f]) || '') * dir);
    }
    // Client-side pagination
    const { page = 1, perPage = 25 } = params.pagination || {};
    const total = data.length;
    data = data.slice((page - 1) * perPage, page * perPage);
    return { data, total };
  },

  getOne: async (resource, params) => {
    const map = resourceMap[resource];
    if (resource === 'nodes') {
      const res = await fetch(`/api/nodes/${params.id}`, { headers: headers() });
      const data = await res.json();
      return { data: { ...data.node, id: data.node.ID, tasks: data.tasks } };
    }
    const res = await fetch(`${map.endpoint}/${params.id}`, { headers: headers() });
    const data = await res.json();
    return { data: { ...data, id: map.getId(data) } };
  },

  delete: async (resource, params) => {
    const map = resourceMap[resource];
    await fetch(`${map.endpoint}/${params.id}`, { method: 'DELETE', headers: headers() });
    return { data: { id: params.id } };
  },

  create: async (resource, params) => {
    const map = resourceMap[resource];
    if (!map) return { data: { id: '' } };
    const res = await fetch(map.endpoint, { method: 'POST', headers: headers(), body: JSON.stringify(params.data) });
    const data = await res.json();
    return { data: { ...data, id: map.getId(data) } };
  },
  update: async (resource, params) => {
    const map = resourceMap[resource];
    if (!map) return { data: { id: '' } };
    const res = await fetch(`${map.endpoint}/${params.id}`, { method: 'PUT', headers: headers(), body: JSON.stringify(params.data) });
    const data = await res.json();
    return { data: { ...params.data, id: params.id } };
  },
  updateMany: async () => ({ data: [] }),
  deleteMany: async (resource, params) => {
    for (const id of params.ids) {
      const map = resourceMap[resource];
      await fetch(`${map.endpoint}/${id}`, { method: 'DELETE', headers: headers() });
    }
    return { data: params.ids };
  },
};
