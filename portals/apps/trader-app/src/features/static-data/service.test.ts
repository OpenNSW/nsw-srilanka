import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SearchService } from '@opennsw/jsonforms-renderers'
import { http } from '@/services/http'
import { staticDataSearchService, withManifestVersion } from './service'

vi.mock('@/services/http', () => ({ http: { request: vi.fn() } }))
vi.mock('@/constants', () => ({ API_BASE_URL: 'https://api.test' }))

const mockRequest = vi.mocked(http.request)

// The service caches each artifact for the page's lifetime, so each test names its own id.
let seq = 0
function freshId() {
  seq += 1
  return `list-${seq}`
}

const options = [
  { const: 'CMB', title: 'Colombo', parents: ['LK'] },
  { const: 'HRI', title: 'Hambantota', parents: ['LK'] },
  { const: 'SIN', title: 'Singapore', parents: ['SG'] },
]

function signal() {
  return new AbortController().signal
}

beforeEach(() => {
  mockRequest.mockReset()
})

// Params for a field that names its artifact id and runs on manifest version 1.
const params = (id: string, extra: Record<string, unknown> = {}) => ({ id: `keys-${id}`, manifestVersion: 1, ...extra })

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
  const [config] = mockRequest.mock.calls[0] as unknown as [{ params: Record<string, unknown> }]
  return config.params
}

describe('staticDataSearchService row keys', () => {
  it('keeps the row keys next to id and name when listing', async () => {
    mockRequest.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka' }] } })

    const { options } = await staticDataSearchService.search({ query: '', signal: signal(), params: params('list') })

    expect(options).toEqual([sriLanka])
  })

  it('keeps the row keys next to id and name on a typed-search page', async () => {
    mockRequest.mockResolvedValue({ data: { items: [{ const: 'LK', title: 'Sri Lanka' }], total: 1 } })

    const { options, nextCursor } = await staticDataSearchService.search({
      query: 'sri',
      signal: signal(),
      params: params('page'),
    })

    expect(options).toEqual([sriLanka])
    expect(nextCursor).toBeUndefined()
  })

  it('keeps the row keys next to id and name when resolving a stored value', async () => {
    mockRequest.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka' }] } })

    const option = await staticDataSearchService.resolve?.('LK', params('resolve'))

    expect(option).toEqual(sriLanka)
  })

  it('keeps id and name meaning the value and title when a row has keys of those names', async () => {
    mockRequest.mockResolvedValue({ data: { data: [{ const: 'LK', title: 'Sri Lanka', id: 'x', name: 'y' }] } })

    const { options } = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: params('reserved'),
    })

    expect(options[0]).toMatchObject({ id: 'LK', name: 'Sri Lanka' })
  })

  it('keeps extra row keys when listing', async () => {
    mockRequest.mockResolvedValue({ data: { data: [ventilated] } })

    const { options } = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: params('extra-list'),
    })

    expect(options).toEqual([ventilatedOption])
  })

  it('keeps extra row keys on a typed-search page', async () => {
    mockRequest.mockResolvedValue({ data: { items: [ventilated], total: 1 } })

    const { options } = await staticDataSearchService.search({
      query: '22v',
      signal: signal(),
      params: params('extra-page'),
    })

    expect(options).toEqual([ventilatedOption])
  })

  it('keeps extra row keys when resolving a stored value', async () => {
    mockRequest.mockResolvedValue({ data: { data: [ventilated] } })

    const option = await staticDataSearchService.resolve?.('22V4', params('extra-resolve'))

    expect(option).toEqual(ventilatedOption)
  })

  it('drops list entries without a non-empty string const and title', async () => {
    mockRequest.mockResolvedValue({
      data: {
        data: [
          { const: 'LK', title: 'Sri Lanka' },
          { const: 'GB' },
          { title: 'No const' },
          { const: '', title: 'Unnamed' },
          { const: 'A', title: '' },
          'LK',
          null,
        ],
      },
    })

    const { options } = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: params('malformed'),
    })

    expect(options).toEqual([sriLanka])
  })

  it('sends the field searchKeys on a typed search', async () => {
    mockRequest.mockResolvedValue({ data: { items: [ventilated], total: 1 } })

    await staticDataSearchService.search({
      query: 'ventilation',
      signal: signal(),
      params: params('search-keys', { searchKeys: ['title', 'description'] }),
    })

    expect(requestParams()).toMatchObject({ q: 'ventilation', searchKeys: 'title,description' })
  })

  it('leaves searchKeys to the API when the field names none', async () => {
    mockRequest.mockResolvedValue({ data: { items: [], total: 0 } })

    await staticDataSearchService.search({ query: 'sri', signal: signal(), params: params('default-keys') })

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
      staticDataSearchService.search({ query: '', signal: signal(), params: params('invalid', extra) }),
    ).rejects.toThrow(/searchKeys/)
    expect(mockRequest).not.toHaveBeenCalled()
  })
})

describe('staticDataSearchService', () => {
  it('loads the artifact at the manifest version', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: options } })

    const result = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: { id, manifestVersion: 3 },
    })

    expect(mockRequest).toHaveBeenCalledWith({
      url: `https://api.test/api/v1/static-data/${id}`,
      params: { manifestVersion: 3 },
      attachToken: true,
    })
    expect(result.options.map((o) => o.id)).toEqual(['CMB', 'HRI', 'SIN'])
  })

  it("ignores a form's own x-search.params.version", async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: options } })

    await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: { id, manifestVersion: 2, version: '1' },
    })

    expect(mockRequest).toHaveBeenCalledWith(expect.objectContaining({ params: { manifestVersion: 2 } }))
  })

  it.each([undefined, 0, -1, 1.5, '3'])('rejects manifest version %j without a request', async (manifestVersion) => {
    await expect(
      staticDataSearchService.search({
        query: '',
        signal: signal(),
        params: { id: freshId(), manifestVersion, version: '1' },
      }),
    ).rejects.toThrow(/manifest version/)
    await expect(staticDataSearchService.resolve?.('CMB', { id: freshId(), manifestVersion })).rejects.toThrow(
      /manifest version/,
    )
    expect(mockRequest).not.toHaveBeenCalled()
  })

  it('rejects a field without an id', async () => {
    await expect(
      staticDataSearchService.search({ query: '', signal: signal(), params: { manifestVersion: 1 } }),
    ).rejects.toThrow(/params\.id/)
  })

  it('caches each manifest version separately', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: options } })

    await staticDataSearchService.search({ query: '', signal: signal(), params: { id, manifestVersion: 1 } })
    await staticDataSearchService.search({ query: '', signal: signal(), params: { id, manifestVersion: 1 } })
    await staticDataSearchService.search({ query: '', signal: signal(), params: { id, manifestVersion: 2 } })

    expect(mockRequest).toHaveBeenCalledTimes(2)
  })

  it('retries after a failed fetch', async () => {
    const id = freshId()
    mockRequest.mockRejectedValueOnce(new Error('boom')).mockResolvedValue({ data: { data: options } })

    await expect(
      staticDataSearchService.search({ query: '', signal: signal(), params: { id, manifestVersion: 1 } }),
    ).rejects.toThrow('boom')
    const result = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: { id, manifestVersion: 1 },
    })

    expect(result.options).toHaveLength(3)
    expect(mockRequest).toHaveBeenCalledTimes(2)
  })

  it('scopes the cached list by parent', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: options } })

    const result = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: { id, manifestVersion: 1, parent: 'SG' },
    })

    expect(result.options).toEqual([{ ...options[2], id: 'SIN', name: 'Singapore' }])
  })

  it('sends a typed query to the server at the manifest version', async () => {
    const id = freshId()
    const s = signal()
    mockRequest.mockResolvedValue({ data: { items: [options[0]], total: 25 } })

    const result = await staticDataSearchService.search({
      query: ' col ',
      signal: s,
      params: { id, manifestVersion: 4, parent: 'LK' },
    })

    expect(mockRequest).toHaveBeenCalledWith({
      url: `https://api.test/api/v1/static-data/${id}`,
      params: { manifestVersion: 4, q: 'col', offset: 0, limit: 20, parent: 'LK' },
      attachToken: true,
      signal: s,
    })
    expect(result).toEqual({ options: [{ ...options[0], id: 'CMB', name: 'Colombo' }], nextCursor: 1 })
  })

  it('pages a server query from the cursor', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { items: [], total: 0 } })

    const result = await staticDataSearchService.search({
      query: '',
      cursor: 20,
      signal: signal(),
      params: { id, manifestVersion: 1 },
    })

    expect(mockRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        params: { manifestVersion: 1, q: undefined, offset: 20, limit: 20, parent: undefined },
      }),
    )
    expect(result.nextCursor).toBeUndefined()
  })

  it('resolves a value at the manifest version', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: options } })

    await expect(staticDataSearchService.resolve?.('HRI', { id, manifestVersion: 5 })).resolves.toEqual({
      ...options[1],
      id: 'HRI',
      name: 'Hambantota',
    })
    await expect(staticDataSearchService.resolve?.('XXX', { id, manifestVersion: 5 })).resolves.toBeUndefined()
    expect(mockRequest).toHaveBeenCalledTimes(1)
    expect(mockRequest).toHaveBeenCalledWith(expect.objectContaining({ params: { manifestVersion: 5 } }))
  })

  it('drops malformed entries', async () => {
    const id = freshId()
    mockRequest.mockResolvedValue({ data: { data: ['bare', { const: 'A' }, { const: 'B', title: 'Bee' }] } })

    const result = await staticDataSearchService.search({
      query: '',
      signal: signal(),
      params: { id, manifestVersion: 1 },
    })

    expect(result.options).toEqual([{ const: 'B', title: 'Bee', id: 'B', name: 'Bee' }])
  })
})

describe('withManifestVersion', () => {
  function fakeService() {
    const search = vi.fn<SearchService['search']>().mockResolvedValue({ options: [] })
    const resolve = vi.fn<NonNullable<SearchService['resolve']>>().mockResolvedValue(undefined)
    return { service: { search, resolve } satisfies SearchService, search, resolve }
  }

  it("adds the manifest version to search params, keeping the field's own", async () => {
    const { service, search } = fakeService()
    const s = signal()

    await withManifestVersion(service, 7).search({
      query: 'q',
      cursor: 3,
      signal: s,
      params: { id: 'ports', version: '1' },
    })

    expect(search).toHaveBeenCalledWith({
      query: 'q',
      cursor: 3,
      signal: s,
      params: { id: 'ports', version: '1', manifestVersion: 7 },
    })
  })

  it('adds the manifest version to search params when the field has none', async () => {
    const { service, search } = fakeService()

    await withManifestVersion(service, 7).search({ query: '', signal: signal() })

    expect(search).toHaveBeenCalledWith(expect.objectContaining({ params: { manifestVersion: 7 } }))
  })

  it('adds the manifest version to resolve params', async () => {
    const { service, resolve } = fakeService()

    await withManifestVersion(service, 7).resolve?.('CMB', { id: 'ports' })

    expect(resolve).toHaveBeenCalledWith('CMB', { id: 'ports', manifestVersion: 7 })
  })

  it('has no resolve when the service has none', () => {
    const bound = withManifestVersion({ search: vi.fn() }, 7)

    expect('resolve' in bound).toBe(false)
  })
})
