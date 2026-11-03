import { describe, expect, it } from 'vitest'
import { pullsFocus, systemAlertLevel } from '@/lib/alerts'
import type { StatusRow } from '@/lib/systemStatus'

const quiet = { guideNews: false, shellRunning: false }

const shown = (severity: StatusRow['severity']): StatusRow => ({ shown: true, severity })

describe('systemAlertLevel', () => {
  it('is none when no row is shown', () => {
    expect(systemAlertLevel([], quiet)).toBeNull()
    expect(systemAlertLevel([{ shown: false, severity: null }], quiet)).toBeNull()
  })

  it('is none when every shown row is healthy', () => {
    expect(systemAlertLevel([shown(null), shown(null)], quiet)).toBeNull()
  })

  it('is the highest severity among the rows', () => {
    expect(systemAlertLevel([shown('info'), shown('warning'), shown(null)], quiet)).toBe('warning')
    expect(systemAlertLevel([shown('warning'), shown('destructive'), shown('info')], quiet)).toBe('destructive')
    expect(systemAlertLevel([shown('info')], quiet)).toBe('info')
  })

  it('raises guide news and a running shell to info without outranking a row', () => {
    expect(systemAlertLevel([], { guideNews: true, shellRunning: false })).toBe('info')
    expect(systemAlertLevel([], { guideNews: false, shellRunning: true })).toBe('info')
    expect(systemAlertLevel([shown('warning')], { guideNews: true, shellRunning: true })).toBe('warning')
  })
})

describe('pullsFocus', () => {
  it('opens the tab for a fault but not for news', () => {
    expect(pullsFocus('destructive')).toBe(true)
    expect(pullsFocus('warning')).toBe(true)
    expect(pullsFocus('pending')).toBe(false)
    expect(pullsFocus('info')).toBe(false)
    expect(pullsFocus(null)).toBe(false)
  })
})
