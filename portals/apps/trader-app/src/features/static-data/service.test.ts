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
const params = (id: string, extra: Record<string, unknown> = {}) => ({ id, version: '1', ...extra })

const sriLanka = { const: 'LK', title: 'Sri Lanka', id: 'LK', name: 'Sri Lanka' }

const fooBooRows = [{ foo: 'A1', boo: 'Alpha' }, { foo: 'B2', boo: 'Beta' }, { boo: 'No foo' }]

// The query parameters of the first request the service made.
function requestParams(): Record<string, unknown> {
  const [config] = request.mock.calls[0] as [{ params: Record<string, unknown> }]
  return config.params
}

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

  it('keeps id and name meaning the value and title when a row has keys of those names', async () => {
    request.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka', id: 'x', name: 'y' }] } })

    const { options } = await staticDataSearchService.search({ query: '', signal, params: params('reserved') })

    expect(options[0]).toMatchObject({ id: 'LK', name: 'Sri Lanka' })
  })

  it('stores the valueKey value and drops rows without one when listing', async () => {
    request.mockResolvedValue({ data: { data: fooBooRows } })

    const { options } = await staticDataSearchService.search({
      query: '',
      signal,
      params: params('foo-list', { valueKey: 'foo' }),
    })

    expect(options).toEqual([
      { foo: 'A1', boo: 'Alpha', id: 'A1', name: 'A1' },
      { foo: 'B2', boo: 'Beta', id: 'B2', name: 'B2' },
    ])
  })

  it('uses the row title as name when the row has one', async () => {
    request.mockResolvedValue({ data: { data: [{ foo: 'A1', title: 'Alpha' }] } })

    const { options } = await staticDataSearchService.search({
      query: '',
      signal,
      params: params('foo-title', { valueKey: 'foo' }),
    })

    expect(options).toEqual([{ foo: 'A1', title: 'Alpha', id: 'A1', name: 'Alpha' }])
  })

  it('resolves a stored value by valueKey', async () => {
    request.mockResolvedValue({ data: { data: fooBooRows } })

    const option = await staticDataSearchService.resolve?.('B2', params('foo-resolve', { valueKey: 'foo' }))

    expect(option).toEqual({ foo: 'B2', boo: 'Beta', id: 'B2', name: 'B2' })
  })

  it('sends searchKeys on a typed search and maps the page by valueKey', async () => {
    request.mockResolvedValue({ data: { items: [{ foo: 'A1', boo: 'Alpha' }], total: 1 } })

    const { options } = await staticDataSearchService.search({
      query: 'alp',
      signal,
      params: params('foo-page', { valueKey: 'foo', searchKeys: ['boo', 'foo'] }),
    })

    expect(requestParams()).toMatchObject({ q: 'alp', searchKeys: 'boo,foo' })
    expect(options).toEqual([{ foo: 'A1', boo: 'Alpha', id: 'A1', name: 'A1' }])
  })

  it('leaves searchKeys to the API when the field names none', async () => {
    request.mockResolvedValue({ data: { items: [], total: 0 } })

    await staticDataSearchService.search({ query: 'sri', signal, params: params('default-keys') })

    expect(requestParams().searchKeys).toBeUndefined()
  })

  it('counts every returned row in the cursor, offered or not', async () => {
    request.mockResolvedValue({ data: { items: [{ foo: 'A1' }, { boo: 'No foo' }], total: 5 } })

    const { options, nextCursor } = await staticDataSearchService.search({
      query: 'a',
      signal,
      params: params('foo-cursor', { valueKey: 'foo' }),
    })

    expect(options).toHaveLength(1)
    expect(nextCursor).toBe(2)
  })

  it.each([
    ['an empty valueKey', { valueKey: '' }, /valueKey/],
    ['a non-string valueKey', { valueKey: 1 }, /valueKey/],
    ['searchKeys that are not a list', { searchKeys: 'boo,foo' }, /searchKeys/],
    ['searchKeys with an empty name', { searchKeys: ['boo', ''] }, /searchKeys/],
  ])('rejects %s', async (_case, extra, message) => {
    await expect(
      staticDataSearchService.search({ query: '', signal, params: params('invalid', extra) }),
    ).rejects.toThrow(message)
    expect(request).not.toHaveBeenCalled()
  })
})
