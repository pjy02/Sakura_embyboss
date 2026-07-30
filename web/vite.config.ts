import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import { resolve } from "node:path";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const area = env.SAKURA_BUILD_AREA || (mode === "admin" ? "admin" : "portal");

  return {
    base: "./",
    plugins: [vue()],
    define: {
      __SAKURA_BUILD_AREA__: JSON.stringify(area),
    },
    resolve: {
      alias: {
        "@": resolve(__dirname, "src"),
      },
    },
    build: {
      outDir: resolve(__dirname, `dist/${area}`),
      emptyOutDir: true,
      sourcemap: false,
      target: "es2022",
    },
    server: {
      port: area === "admin" ? 5174 : 5173,
      proxy: {
        "/api": "http://127.0.0.1:8838",
      },
    },
  };
});
