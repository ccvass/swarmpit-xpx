import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: { outDir: '../web' },
  server: { proxy: { '/api': 'http://localhost:8080', '/login': 'http://localhost:8080', '/version': 'http://localhost:8080', '/initialize': 'http://localhost:8080', '/events': 'http://localhost:8080', '/health': 'http://localhost:8080' } }
});
