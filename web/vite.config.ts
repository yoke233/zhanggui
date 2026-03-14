import path from "path";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const proxyTarget = env.VITE_API_PROXY_TARGET || "http://127.0.0.1:8080";

  const vendorChunkGroups: Array<{ name: string; patterns: string[] }> = [
    {
      name: "react-vendor",
      patterns: ["react", "react-dom", "scheduler"],
    },
    {
      name: "router-vendor",
      patterns: ["react-router", "@remix-run/router"],
    },
    {
      name: "i18n-vendor",
      patterns: ["i18next", "react-i18next"],
    },
    {
      name: "graph-vendor",
      patterns: ["@xyflow/react"],
    },
    {
      name: "render-vendor",
      patterns: ["react-syntax-highlighter", "diff2html"],
    },
    {
      name: "state-vendor",
      patterns: ["zustand"],
    },
  ];

  return {
    plugins: [react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    build: {
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes("node_modules")) {
              return undefined;
            }
            for (const group of vendorChunkGroups) {
              if (group.patterns.some((pattern) => id.includes(pattern))) {
                return group.name;
              }
            }
            return "vendor";
          },
        },
      },
    },
    server: {
      proxy: {
        "/api": {
          target: proxyTarget,
          changeOrigin: true,
          ws: true
        }
      }
    }
  };
});
