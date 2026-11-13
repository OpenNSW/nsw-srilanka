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

    expect(result.options).toEqual([{ id: 'SIN', name: 'Singapore' }])
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
    expect(result).toEqual({ options: [{ id: 'CMB', name: 'Colombo' }], nextCursor: 1 })
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

    expect(result.options).toEqual([{ id: 'B', name: 'Bee' }])
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
