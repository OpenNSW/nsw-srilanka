import type { ReactNode } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProfileProvider } from '@/services/ProfileContext'
import { getProfile, type UserProfile } from '@/services/profile'
import { useProfile } from '@/services/useProfile'

vi.mock('@/services/profile', () => ({ getProfile: vi.fn() }))

const mockGetProfile = vi.mocked(getProfile)

function makeProfile(id: string): UserProfile {
  return {
    id,
    email: `${id}@example.com`,
    phoneNumber: '',
    data: null,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  }
}

function wrapper({ children }: { children: ReactNode }) {
  return <ProfileProvider>{children}</ProfileProvider>
}

describe('ProfileProvider', () => {
  afterEach(() => {
    mockGetProfile.mockReset()
  })

  it('loads the profile on mount', async () => {
    mockGetProfile.mockResolvedValue(makeProfile('user-1'))

    const { result } = renderHook(() => useProfile(), { wrapper })

    expect(result.current.profile).toBeNull()
    expect(result.current.isLoading).toBe(true)
    expect(result.current.error).toBeNull()

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.profile).toEqual(makeProfile('user-1'))
    expect(result.current.error).toBeNull()
    expect(mockGetProfile).toHaveBeenCalledTimes(1)
  })

  it('exposes a failed load as an error', async () => {
    mockGetProfile.mockRejectedValue(new Error('unauthorized'))

    const { result } = renderHook(() => useProfile(), { wrapper })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.profile).toBeNull()
    expect(result.current.error?.message).toBe('unauthorized')
  })

  it('keeps the current profile while a refetch loads the new one', async () => {
    mockGetProfile.mockResolvedValueOnce(makeProfile('user-1'))

    const { result } = renderHook(() => useProfile(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    let resolveRefetch!: (profile: UserProfile) => void
    mockGetProfile.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRefetch = resolve
      }),
    )

    let refetchPromise!: Promise<void>
    act(() => {
      refetchPromise = result.current.refetch()
    })

    expect(result.current.isLoading).toBe(true)
    expect(result.current.profile).toEqual(makeProfile('user-1'))

    await act(async () => {
      resolveRefetch(makeProfile('user-2'))
      await refetchPromise
    })

    expect(result.current.isLoading).toBe(false)
    expect(result.current.profile).toEqual(makeProfile('user-2'))
    expect(mockGetProfile).toHaveBeenCalledTimes(2)
  })

  it('aborts the request when the provider unmounts', () => {
    mockGetProfile.mockReturnValue(new Promise(() => {}))

    const { unmount } = renderHook(() => useProfile(), { wrapper })
    const signal = mockGetProfile.mock.calls[0][0]

    expect(signal?.aborted).toBe(false)
    unmount()
    expect(signal?.aborted).toBe(true)
  })
})
