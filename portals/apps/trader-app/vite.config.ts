import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0', // Exposes Vite on the local network
    port: 5173,
    strictPort: true, // Prevents Vite from auto-switching ports if 5173 is busy
  }
})
