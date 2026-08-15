import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Port 3000 matches nginx.conf (listen 3000), Containerfile.ui (EXPOSE 3000),
// the gateway's default SENTINEL_ALLOWED_ORIGIN, and the README.
//
// A vite.config.js previously sat alongside this file. Vite resolves .js first,
// so this config -- and with it @vitejs/plugin-react -- was never loaded, and
// Fast Refresh was silently off. The .js has been removed.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    host: true,
  },
  preview: {
    port: 3000,
  },
});
