import { render, screen } from '@testing-library/react'
import { UploadProvider } from '@opennsw/jsonforms-renderers'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MarkdownRenderer } from '@/features/zone/components/renderers/MarkdownRenderer'

const barcodeRef = '0f8e7c1a-0000-4000-8000-000000000001.png'

function renderMarkdown(content: string, getDownloadUrl?: (key: string) => Promise<{ url: string }>) {
  return render(
    <UploadProvider getDownloadUrl={getDownloadUrl as never}>
      <MarkdownRenderer payload={{ content }} />
    </UploadProvider>,
  )
}

describe('MarkdownRenderer images', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows a stored image from the download url issued for it', async () => {
    const getDownloadUrl = vi.fn().mockResolvedValue({ url: 'https://files.example/barcode.png?sig=1' })

    renderMarkdown(`![Gate pass barcode](files:${barcodeRef})`, getDownloadUrl)

    const img = await screen.findByRole('img', { name: 'Gate pass barcode' })
    expect(img.getAttribute('src')).toBe('https://files.example/barcode.png?sig=1')
    expect(getDownloadUrl).toHaveBeenCalledWith(barcodeRef)
  })

  it('shows the alt text while the url is on its way', () => {
    renderMarkdown(`![Gate pass barcode](files:${barcodeRef})`, () => new Promise(() => {}))

    expect(screen.getByText('Gate pass barcode')).toBeTruthy()
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('keeps the alt text when the url cannot be had', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const getDownloadUrl = vi.fn().mockRejectedValue(new Error('forbidden'))

    renderMarkdown(`![Gate pass barcode](files:${barcodeRef})`, getDownloadUrl)

    await vi.waitFor(() => expect(console.error).toHaveBeenCalled())
    expect(screen.getByText('Gate pass barcode')).toBeTruthy()
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('shows nothing for an image whose file field is empty', () => {
    const getDownloadUrl = vi.fn()

    const { container } = renderMarkdown('![Gate pass barcode](files:)', getDownloadUrl)

    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByText('Gate pass barcode')).toBeNull()
    expect(getDownloadUrl).not.toHaveBeenCalled()
  })

  it('shows the alt text without an upload context', () => {
    renderMarkdown(`![Gate pass barcode](files:${barcodeRef})`)

    expect(screen.getByText('Gate pass barcode')).toBeTruthy()
  })

  it('shows any other image from its own url', () => {
    const getDownloadUrl = vi.fn()

    renderMarkdown('![Logo](https://example.org/logo.png)', getDownloadUrl)

    expect(screen.getByRole('img', { name: 'Logo' }).getAttribute('src')).toBe('https://example.org/logo.png')
    expect(getDownloadUrl).not.toHaveBeenCalled()
  })
})
