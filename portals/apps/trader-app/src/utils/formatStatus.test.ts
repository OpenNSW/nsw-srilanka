import { describe, expect, it } from 'vitest'
import { humanizeStatus } from '@/utils/formatStatus'

describe('humanizeStatus', () => {
  it('turns SCREAMING_SNAKE_CASE into title case with spaces', () => {
    expect(humanizeStatus('PENDING_USER')).toBe('Pending User')
    expect(humanizeStatus('OGA_REVIEWING')).toBe('Oga Reviewing')
  })

  it('capitalizes a single word', () => {
    expect(humanizeStatus('COMPLETED')).toBe('Completed')
  })

  it('returns an empty string unchanged', () => {
    expect(humanizeStatus('')).toBe('')
  })
})
