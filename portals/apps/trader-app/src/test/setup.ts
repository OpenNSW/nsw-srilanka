import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Testing Library unmounts rendered trees after each test only when Vitest's
// globals are on; they're off here, so do it explicitly.
afterEach(() => {
  cleanup()
})

// Radix components call these browser APIs, which jsdom leaves out.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
const elementStubs = { hasPointerCapture: () => false, releasePointerCapture: () => {}, scrollIntoView: () => {} }
for (const [name, stub] of Object.entries(elementStubs)) {
  if (!(name in Element.prototype)) {
    Object.defineProperty(Element.prototype, name, { value: stub, configurable: true, writable: true })
  }
}
