import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte()],
  server: {
    // Host file events do not cross the Colima bind mount, hot reload needs polling.
    watch: { usePolling: true },
    proxy: {
      // Object form on purpose: the string shorthand rewrites Host, and the API's same-origin check then refuses writes.
      '/api': { target: process.env.AXOLOT_API },
    },
  },
});
