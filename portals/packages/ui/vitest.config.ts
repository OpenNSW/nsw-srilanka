import { defineConfig } from 'vitest/config'

// Tests sit beside the code they cover as *.test.ts(x) and import from
// 'vitest' explicitly, so no globals are configured.
export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
  },
})
