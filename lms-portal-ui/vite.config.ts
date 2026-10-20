import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import path from "path";

// https://vitejs.dev/config/
export default defineConfig(() => ({
  server: {
    host: "::",
    port: 8080,
    hmr: {
      overlay: false,
    },
    proxy: {
      "/proxy/auth": {
        target: "http://localhost:28086",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/auth/, ""),
      },
      "/proxy/products": {
        target: "http://localhost:28087",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/products/, ""),
      },
      "/proxy/loan-applications": {
        target: "http://localhost:28088",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/loan-applications/, ""),
      },
      "/proxy/loans": {
        target: "http://localhost:28089",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/loans/, ""),
      },
      "/proxy/payments": {
        target: "http://localhost:28090",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/payments/, ""),
      },
      "/proxy/accounting": {
        target: "http://localhost:28091",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/accounting/, ""),
      },
      "/proxy/float": {
        target: "http://localhost:28092",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/float/, ""),
      },
      "/proxy/collections": {
        target: "http://localhost:28093",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/collections/, ""),
      },
      "/proxy/compliance": {
        target: "http://localhost:28094",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/compliance/, ""),
      },
      "/proxy/reporting": {
        target: "http://localhost:28095",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/reporting/, ""),
      },
      "/proxy/scoring": {
        target: "http://localhost:28096",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/scoring/, ""),
      },
      "/proxy/fraud": {
        target: "http://localhost:28100",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/fraud/, ""),
      },
      "/proxy/fraud-ml": {
        target: "http://localhost:18101",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/fraud-ml/, ""),
      },
      "/proxy/overdraft": {
        target: "http://localhost:28097",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/overdraft/, ""),
      },
      "/proxy/media": {
        target: "http://localhost:28098",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/media/, ""),
      },
      "/proxy/notifications": {
        target: "http://localhost:28099",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/proxy\/notifications/, ""),
      },
    },
  },
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
    dedupe: ["react", "react-dom", "react/jsx-runtime", "@tanstack/react-query"],
  },
  optimizeDeps: {
    include: ["react", "react-dom", "@tanstack/react-query"],
  },
}));
