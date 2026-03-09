import { getEnv } from '../runtimeConfig'

const API_BASE_URL = getEnv('VITE_API_BASE_URL', 'http://localhost:8080/api/v1')!

export interface FileMetadata {
  id: string
  name: string
  key: string
  url: string
  size: number
  mimeType: string
}

export async function uploadFile(file: File): Promise<FileMetadata> {
  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${API_BASE_URL}/uploads`, {
    method: 'POST',
    body: formData,
  })

  if (!response.ok) {
    const errorText = await response.text()
    console.error(`Upload error ${response.status}: ${errorText}`)
    throw new Error(`Failed to upload file: ${response.status} ${response.statusText}`)
  }

  const metadata = await response.json() as FileMetadata
  return metadata
}

export function getFileUrl(key: string): string {
  return `${API_BASE_URL}/uploads/${key}`
}
