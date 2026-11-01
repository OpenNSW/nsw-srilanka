import { render, screen } from '@testing-library/react'
import { UploadProvider } from '@opennsw/jsonforms-renderers'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownRenderer } from '@/features/zone/components/renderers/MarkdownRenderer'
import type { ZoneRendererProps } from '@/features/zone/types'

function renderMarkdown(content: string, getDownloadUrl = vi.fn()) {
  const props = { payload: { content } } as ZoneRendererProps<'MARKDOWN'>
  render(
    <UploadProvider onUpload={vi.fn()} getDownloadUrl={getDownloadUrl}>
      <MarkdownRenderer {...props} />
    </UploadProvider>,
  )
  return getDownloadUrl
}

describe('MarkdownRenderer images', () => {
  it('shows a stored image from the download URL resolved for it', async () => {
    const getDownloadUrl = renderMarkdown(
      '![Gate pass barcode](files:ref-123)',
      vi.fn().mockResolvedValue({ url: 'https://storage.example/signed/ref-123' }),
    )

    const img = await screen.findByRole('img', { name: 'Gate pass barcode' })
    expect(img.getAttribute('src')).toBe('https://storage.example/signed/ref-123')
    expect(getDownloadUrl).toHaveBeenCalledWith('ref-123')
  })

  it('shows the alt text when the image cannot be resolved', async () => {
    const getDownloadUrl = renderMarkdown(
      '![Gate pass barcode](files:ref-123)',
      vi.fn().mockRejectedValue(new Error('gone')),
    )

    await vi.waitFor(() => expect(getDownloadUrl).toHaveBeenCalled())
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('Gate pass barcode')).toBeTruthy()
  })

  it('shows nothing for a files: image with no reference', () => {
    const getDownloadUrl = renderMarkdown('![Gate pass barcode](files:)')

    expect(screen.queryByRole('img')).toBeNull()
    expect(getDownloadUrl).not.toHaveBeenCalled()
  })

  it('shows any other image as it is', () => {
    renderMarkdown('![Gate pass barcode](data:image/png;base64,iVBORw0KGgo=)')

    expect(screen.getByRole('img', { name: 'Gate pass barcode' }).getAttribute('src')).toBe(
      'data:image/png;base64,iVBORw0KGgo=',
    )
  })
})

describe('MarkdownRenderer inline images', () => {
  it('drops an inline SVG image, which can carry script', () => {
    renderMarkdown('![x](data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)')

    expect(screen.queryByRole('img')?.getAttribute('src') ?? '').toBe('')
  })

  it('does not keep an inline image as a link', () => {
    renderMarkdown('[open](data:image/png;base64,iVBORw0KGgo=)')

    expect(screen.getByText('open').getAttribute('href') ?? '').toBe('')
  })
})
