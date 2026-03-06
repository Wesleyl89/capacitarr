import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  plugins: [vue()],
  define: {
    // Nuxt uses import.meta.client / import.meta.server at build time.
    // Define them for Vitest so composables can detect client-side context.
    'import.meta.client': true,
    'import.meta.server': false,
  },
  test: {
    environment: 'happy-dom',
    globals: true,
    include: ['app/**/*.test.ts', 'app/**/*.spec.ts'],
  },
  resolve: {
    alias: {
      '~': fileURLToPath(new URL('./app', import.meta.url)),
      '@': fileURLToPath(new URL('./app', import.meta.url)),
    },
  },
});
