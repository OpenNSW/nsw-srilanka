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

// A container type with keys beyond const and title.
const ventilated = {
  const: '22V4',
  title: '20x8 6, Mechanical',
  description: '20x8 6, Mechanical ventilation system',
  group: 'Ventilated',
}
const ventilatedOption = { ...ventilated, id: '22V4', name: '20x8 6, Mechanical' }

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

  it('keeps extra row keys when listing', async () => {
    request.mockResolvedValue({ data: { data: [ventilated] } })

    const { options } = await staticDataSearchService.search({ query: '', signal, params: params('extra-list') })

    expect(options).toEqual([ventilatedOption])
  })

  it('keeps extra row keys on a typed-search page', async () => {
    request.mockResolvedValue({ data: { items: [ventilated], total: 1 } })

    const { options } = await staticDataSearchService.search({ query: '22v', signal, params: params('extra-page') })

    expect(options).toEqual([ventilatedOption])
  })

  it('keeps extra row keys when resolving a stored value', async () => {
    request.mockResolvedValue({ data: { data: [ventilated] } })

    const option = await staticDataSearchService.resolve?.('22V4', params('extra-resolve'))

    expect(option).toEqual(ventilatedOption)
  })

  it('drops list entries without a string const and title', async () => {
    request.mockResolvedValue({
      data: { data: [{ const: 'LK', title: 'Sri Lanka' }, { const: 'GB' }, { title: 'No const' }, 'LK', null] },
    })

    const { options } = await staticDataSearchService.search({ query: '', signal, params: params('malformed') })

    expect(options).toEqual([sriLanka])
  })

  it('sends the field searchKeys on a typed search', async () => {
    request.mockResolvedValue({ data: { items: [ventilated], total: 1 } })

    await staticDataSearchService.search({
      query: 'ventilation',
      signal,
      params: params('search-keys', { searchKeys: ['title', 'description'] }),
    })

    expect(requestParams()).toMatchObject({ q: 'ventilation', searchKeys: 'title,description' })
  })

  it('leaves searchKeys to the API when the field names none', async () => {
    request.mockResolvedValue({ data: { items: [], total: 0 } })

    await staticDataSearchService.search({ query: 'sri', signal, params: params('default-keys') })

    expect(requestParams().searchKeys).toBeUndefined()
  })

  it.each([
    ['searchKeys that are not a list', { searchKeys: 'title,description' }],
    ['searchKeys with an empty name', { searchKeys: ['title', ''] }],
    ['searchKeys with a whitespace-only name', { searchKeys: ['title', '  '] }],
    ['searchKeys with a comma inside a name', { searchKeys: ['title,description'] }],
    ['searchKeys with a name that has a space', { searchKeys: ['title', 'my key'] }],
  ])('rejects %s', async (_case, extra) => {
    await expect(
      staticDataSearchService.search({ query: '', signal, params: params('invalid', extra) }),
    ).rejects.toThrow(/searchKeys/)
    expect(request).not.toHaveBeenCalled()
  })
})
