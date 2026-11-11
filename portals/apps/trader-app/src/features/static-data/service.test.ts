import { beforeEach, describe, expect, it, vi } from 'vitest'
import { staticDataSearchService } from '@/features/static-data/service'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

const request = vi.hoisted(() => vi.fn())
vi.mock('@/services/http', () => ({ http: { request } }))

const signal = new AbortController().signal

// The service keeps a fetched artifact per (id, version) for the life of the module, so each
// test uses its own id.
const params = (id: string) => ({ id, version: '1' })

const sriLanka = { const: 'LK', title: 'Sri Lanka', id: 'LK', name: 'Sri Lanka' }

describe('staticDataSearchService', () => {
  beforeEach(() => {
    request.mockReset()
  })

  it('keeps the row keys next to id and name when listing', async () => {
    request.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka' }] } })

    const { options } = await staticDataSearchService.search({ query: '', signal, params: params('list') })

    expect(options).toEqual([sriLanka])
  })

  it('keeps the row keys next to id and name on a typed-search page', async () => {
    request.mockResolvedValue({ data: { items: [{ const: 'LK', title: 'Sri Lanka' }], total: 1 } })

    const { options, nextCursor } = await staticDataSearchService.search({
      query: 'sri',
      signal,
      params: params('page'),
    })

    expect(options).toEqual([sriLanka])
    expect(nextCursor).toBeUndefined()
  })

  it('keeps the row keys next to id and name when resolving a stored value', async () => {
    request.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka' }] } })

    const option = await staticDataSearchService.resolve?.('LK', params('resolve'))

    expect(option).toEqual(sriLanka)
  })

  it('keeps id and name meaning const and title when a row has keys of those names', async () => {
    request.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka', id: 'x', name: 'y' }] } })

    const { options } = await staticDataSearchService.search({ query: '', signal, params: params('reserved') })

    expect(options[0]).toMatchObject({ id: 'LK', name: 'Sri Lanka' })
  })
})
