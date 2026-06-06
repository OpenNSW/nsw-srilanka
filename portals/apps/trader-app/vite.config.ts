import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: process.env.HOST || 'localhost', // Exposes Vite on the local network if HOST is set
    port: 5173,
    strictPort: true, // Prevents Vite from auto-switching ports if 5173 is busy
  }
})
