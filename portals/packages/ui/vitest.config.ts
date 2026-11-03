import { defineConfig } from 'vitest/config'

// Tests sit beside the code they cover as *.test.ts(x) and import from
// 'vitest' explicitly, so no globals are configured.
export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
    // `pnpm test:coverage` measures it; the report goes to coverage/. No
    // threshold: a failing number is the team's call, not this config's.
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/test/**', 'src/index.ts'],
    },
  },
})
