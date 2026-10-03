import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    name: 'pebblepost-web',
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/lib/**/*.{test,spec}.{ts,tsx}', 'src/store/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['node_modules', 'dist', 'src/store/tabStore.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: ['src/lib/**', 'src/store/**'],
      exclude: ['src/test/**', '**/*.d.ts'],
    },
  },
})
