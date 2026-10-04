import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Proxying keeps the dashboard same-origin with the API, so the session
    // cookie (SameSite=Lax) and the double-submit CSRF cookie both work with
    // no CORS configuration at all.
    proxy: {
      '/api': 'http://localhost:8080',
      // Proxied too, so the snippet the dashboard shows works verbatim from a
      // page served by the dev server instead of 404ing into the SPA fallback.
      '/script.js': 'http://localhost:8080',
    },
  },
})