/// <reference types="vitest/config" />
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  // MISE_API points the dev server at another backend, e.g. the deployed one.
  const api = loadEnv(mode, '.', 'MISE_').MISE_API || 'http://localhost:8080';
  return {
    plugins: [react()],
    server: {
      proxy: {
        '/api': api,
        '/images': api,
      },
    },
    test: {
      environment: 'node',
    },
  };
});
