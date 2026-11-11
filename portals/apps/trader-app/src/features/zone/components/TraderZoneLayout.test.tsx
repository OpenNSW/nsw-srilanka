import { render } from '@testing-library/react'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SearchServiceRegistry } from '@opennsw/jsonforms-renderers'
import '@/i18n'
import { http } from '@/services/http'
import { searchServices } from '@/services/searchService'
import type { ZoneView } from '@/features/zone/types'
import { TraderZoneLayout } from './TraderZoneLayout'

// config.js sets this in the app; @/constants reads it as it loads.
vi.hoisted(() => {
  window.__APP_CONFIG__ = { API_BASE_URL: 'http://api.test' }
})

vi.mock('@/services/http', () => ({ http: { request: vi.fn() } }))

// Records the registry each SearchServiceProvider is given, then renders its children as the
// real one would.
const provided = vi.hoisted(() => [] as SearchServiceRegistry[])
vi.mock('@opennsw/jsonforms-renderers', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@opennsw/jsonforms-renderers')>()
  return {
    ...actual,
    SearchServiceProvider: (props: { services: SearchServiceRegistry; children: ReactNode }) => {
      provided.push(props.services)
      return <actual.SearchServiceProvider {...props} />
    },
  }
})

const mockRequest = vi.mocked(http.request)

function task(manifestVersion?: number): ZoneView {
  return {
    task_id: 't-1',
    task_type: 'APPLICATION',
    state: 'PENDING_USER',
    manifest_version: manifestVersion,
    view: [{ id: 'note', type: 'MARKDOWN', payload: { content: 'Hello' } }],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

beforeEach(() => {
  provided.length = 0
  mockRequest.mockReset()
  mockRequest.mockResolvedValue({ data: { data: [] } })
})

describe('TraderZoneLayout', () => {
  it("gives the task's forms static data at the task's manifest version", async () => {
    render(
      <Theme>
        <TraderZoneLayout task={task(6)} />
      </Theme>,
    )

    const services = provided.at(-1)
    expect(services).toBeDefined()
    await services!['static-data'].search({
      query: '',
      signal: new AbortController().signal,
      params: { id: 'layout-ports', version: '1' },
    })
    expect(mockRequest).toHaveBeenCalledWith(expect.objectContaining({ params: { manifestVersion: 6 } }))
  })

  it('gives the app-wide services to a task without a manifest version', () => {
    render(
      <Theme>
        <TraderZoneLayout task={task()} />
      </Theme>,
    )

    expect(provided.at(-1)).toBe(searchServices)
  })
})
