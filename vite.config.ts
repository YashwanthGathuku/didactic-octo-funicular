import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Port 3000 matches nginx.conf (listen 3000), Containerfile.ui (EXPOSE 3000),
// the gateway's default SENTINEL_ALLOWED_ORIGIN, and the README.
//
// A vite.config.js previously sat alongside this file. Vite resolves .js first,
// so this config -- and with it @vitejs/plugin-react -- was never loaded, and
// Fast Refresh was silently off. The .js has been removed.
export default defineConfig({
  // Tailwind is a build plugin rather than a PostCSS step. It was added when
  // the operations console was written: the console's markup was written
  // against Tailwind utilities and the project had no Tailwind, so every class
  // was inert. The build was green and the screens rendered unstyled -- a
  // reminder that a passing build says nothing about whether the thing works.
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    host: true,
  },
  preview: {
    port: 3000,
  },
});
