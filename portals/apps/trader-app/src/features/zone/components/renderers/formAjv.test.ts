import { describe, expect, it } from 'vitest'
import { formAjv } from './formAjv'

/** Local calendar day, YYYY-MM-DD — matches how withRelativeDates resolves "today". */
function localISODate(now = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

function shiftDays(isoDate: string, days: number): string {
  const [y, m, d] = isoDate.split('-').map(Number)
  const date = new Date(y, m - 1, d)
  date.setDate(date.getDate() + days)
  return localISODate(date)
}

const todayMinimumSchema = {
  type: 'object',
  properties: {
    shipmentDate: { type: 'string', format: 'date', formatMinimum: 'today' },
  },
  required: ['shipmentDate'],
} as const

const fixedMinimumSchema = {
  type: 'object',
  properties: {
    shipmentDate: { type: 'string', format: 'date', formatMinimum: '2026-01-15' },
  },
  required: ['shipmentDate'],
} as const

describe('formAjv formatMinimum today', () => {
  it('accepts today and a later day', () => {
    const today = localISODate()
    expect(formAjv.validate(todayMinimumSchema, { shipmentDate: today })).toBe(true)
    expect(formAjv.validate(todayMinimumSchema, { shipmentDate: shiftDays(today, 1) })).toBe(true)
  })

  it('rejects a day before today', () => {
    const yesterday = shiftDays(localISODate(), -1)
    expect(formAjv.validate(todayMinimumSchema, { shipmentDate: yesterday })).toBe(false)
    expect(formAjv.errors?.some((e) => e.keyword === 'formatMinimum')).toBe(true)
  })

  it('still compares a fixed formatMinimum as a calendar date', () => {
    expect(formAjv.validate(fixedMinimumSchema, { shipmentDate: '2026-01-15' })).toBe(true)
    expect(formAjv.validate(fixedMinimumSchema, { shipmentDate: '2026-01-14' })).toBe(false)
  })
})
