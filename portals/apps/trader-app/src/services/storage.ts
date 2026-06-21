import { http } from './http'
import { API_BASE_URL } from '../constants'

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

export async function uploadFile(file: File): Promise<UploadResponse> {
  const { data: metadata } = await http.request<UploadMetadataResponse>({
    url: `${API_BASE_URL}/api/v1/storage`,
    method: 'POST',
    data: {
      filename: file.name,
      mime_type: file.type || 'application/octet-stream',
      size: file.size,
    } satisfies UploadMetadataRequest,
    attachToken: true,
  })

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
  const { data } = await http.request<DownloadMetadataResponse>({
    url: `${API_BASE_URL}/api/v1/storage/${key}`,
    attachToken: true,
  })

  const url = data.download_url.startsWith('/')
    ? new URL(data.download_url, API_BASE_URL).toString()
    : data.download_url

  return { url, expiresAt: data.expires_at }
}
