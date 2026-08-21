import { useState, useEffect, useRef, type ReactNode } from 'react'
import { ProfileContext } from './profileContextCore'
import { getProfile, type UserProfile } from './profile'

export function ProfileProvider({ children }: { children: ReactNode }) {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const requestIdRef = useRef(0)

  useEffect(() => {
    async function fetchProfile() {
      const requestId = ++requestIdRef.current
      setIsLoading(true)
      try {
        const data = await getProfile()
        if (requestId !== requestIdRef.current) return
        setProfile(data)
        setError(null)
      } catch (err) {
        if (requestId !== requestIdRef.current) return
        setError(err instanceof Error ? err : new Error(String(err)))
      } finally {
        if (requestId === requestIdRef.current) setIsLoading(false)
      }
    }

    void fetchProfile()
  }, [])

  const refetch = async () => {
    const requestId = ++requestIdRef.current
    setIsLoading(true)
    try {
      const data = await getProfile()
      if (requestId !== requestIdRef.current) return
      setProfile(data)
      setError(null)
    } catch (err) {
      if (requestId !== requestIdRef.current) return
      setError(err instanceof Error ? err : new Error(String(err)))
    } finally {
      if (requestId === requestIdRef.current) setIsLoading(false)
    }
  }

  return <ProfileContext.Provider value={{ profile, isLoading, error, refetch }}>{children}</ProfileContext.Provider>
}
