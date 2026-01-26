import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  build: {
    // Keep placeholder.txt so the Go embed patterns always have at least one match.
    // The root build script cleans web/dist before running vite build.
    emptyOutDir: false,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:5174",
        changeOrigin: true,
      },
    },
  },
});
