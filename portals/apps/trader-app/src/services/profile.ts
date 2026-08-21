import { http } from '@/services/http'
import { API_BASE_URL } from '@/constants'

export interface CompanyProfile {
  id: string
  name: string
  hasCha: boolean
}

export interface UserProfile {
  id: string
  email: string
  phoneNumber: string
  data: unknown
  createdAt: string
  updatedAt: string
  company?: CompanyProfile
}

export async function getProfile(): Promise<UserProfile> {
  const { data } = await http.request<UserProfile>({
    url: `${API_BASE_URL}/api/v1/users/me`,
    attachToken: true,
  })
  return data
}
