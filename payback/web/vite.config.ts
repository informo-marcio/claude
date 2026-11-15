import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// A interface compilada vai para dentro do pacote Go que a embute no binário.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "../internal/webui/dist", emptyOutDir: false },
  server: {
    port: 5173,
    strictPort: true,
    proxy: { "/api": "http://127.0.0.1:8080", "/auth": "http://127.0.0.1:8080" },
  },
});
