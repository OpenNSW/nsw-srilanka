import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Testing Library unmounts rendered trees after each test only when Vitest's
// globals are on; they're off here, so do it explicitly.
afterEach(() => {
  cleanup()
})
