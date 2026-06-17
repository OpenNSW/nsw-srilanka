import { http } from './http'
import { API_BASE_URL, API_PATH_PREFIX } from '../constants'

interface UploadMetadataRequest {
  filename: string
  mime_type: string
  size: number
}

interface UploadMetadataResponse {
  key: string
  name: string
  upload_url: string
}

interface DownloadMetadataResponse {
  download_url: string
  expires_at: number
}

export interface UploadResponse {
  key: string
  name: string
}

const BASE = `${API_BASE_URL}${API_PATH_PREFIX}`

export async function uploadFile(file: File): Promise<UploadResponse> {
  const { data } = await http.request({
    url: `${BASE}/storage`,
    method: 'POST',
    data: {
      filename: file.name,
      mime_type: file.type || 'application/octet-stream',
      size: file.size,
    } satisfies UploadMetadataRequest,
    attachToken: true,
  })

  const metadata = data as UploadMetadataResponse

  const uploadResponse = await fetch(metadata.upload_url, {
    method: 'PUT',
    headers: { 'Content-Type': file.type || 'application/octet-stream' },
    body: file,
  })

  if (!uploadResponse.ok) {
    const errorText = await uploadResponse.text()
    console.error(`Direct storage upload error ${uploadResponse.status}: ${errorText}`)
    throw new Error(`Failed to upload file to storage: ${uploadResponse.status} ${uploadResponse.statusText}`)
  }

  return { key: metadata.key, name: metadata.name }
}

export async function getDownloadUrl(key: string): Promise<{ url: string; expiresAt: number }> {
  const { data } = await http.request({
    url: `${BASE}/storage/${key}`,
    attachToken: true,
  })

  const response = data as DownloadMetadataResponse

  const url = response.download_url.startsWith('/')
    ? new URL(response.download_url, API_BASE_URL).toString()
    : response.download_url

  return { url, expiresAt: response.expires_at }
}
