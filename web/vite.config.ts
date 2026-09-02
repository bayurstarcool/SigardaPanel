import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    host: '0.0.0.0',
    allowedHosts: ['panel.sigarda.com', 'panel.sigarda.dev', 'localhost', '127.0.0.1'],
    proxy: {
      '/api/proxy': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => '/api/v1/' + path.slice('/api/proxy/'.length),
        timeout: 300000,
      },
      '/api/auth': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => '/api/v1' + path.slice('/api'.length),
        timeout: 300000,
      },
      '/api/metrics': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => '/api/v1' + path.slice('/api'.length),
        timeout: 300000,
      }
    }
  }
});
