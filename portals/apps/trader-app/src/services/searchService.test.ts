import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http } from '@/services/http'
import { searchServices, taskSearchServices } from './searchService'

vi.mock('@/services/http', () => ({ http: { request: vi.fn() } }))
vi.mock('@/constants', () => ({ API_BASE_URL: 'https://api.test' }))

const mockRequest = vi.mocked(http.request)

beforeEach(() => {
  mockRequest.mockReset()
  mockRequest.mockResolvedValue({ data: { data: [] } })
})

describe('taskSearchServices', () => {
  it('is searchServices without a manifest version', () => {
    expect(taskSearchServices(undefined)).toBe(searchServices)
  })

  it('keeps every other service', () => {
    const services = taskSearchServices(4)

    expect(Object.keys(services).sort()).toEqual(Object.keys(searchServices).sort())
    expect(services.cha).toBe(searchServices.cha)
  })

  it('loads static data at the manifest version', async () => {
    await taskSearchServices(4)['static-data'].search({
      query: '',
      signal: new AbortController().signal,
      params: { id: 'task-search-ports', version: '1' },
    })

    expect(mockRequest).toHaveBeenCalledWith(expect.objectContaining({ params: { manifestVersion: 4 } }))
  })
})
