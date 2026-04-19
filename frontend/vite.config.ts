import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "path";

export default defineConfig({
  plugins: [tailwindcss(), react()],
  server: { port: 5173 },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
});
