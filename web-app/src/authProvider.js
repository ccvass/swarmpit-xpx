export default {
  login: async ({ username, password }) => {
    const res = await fetch('/login', { method: 'POST', headers: { Authorization: 'Basic ' + btoa(username + ':' + password) } });
    if (!res.ok) throw new Error('Invalid credentials');
    const { token } = await res.json();
    localStorage.setItem('token', token);
  },
  logout: () => { localStorage.removeItem('token'); return Promise.resolve(); },
  checkAuth: () => localStorage.getItem('token') ? Promise.resolve() : Promise.reject(),
  checkError: (error) => error.status === 401 ? Promise.reject() : Promise.resolve(),
  getIdentity: () => Promise.resolve({ id: 'admin', fullName: 'Admin' }),
  getPermissions: () => Promise.resolve(['admin']),
};
