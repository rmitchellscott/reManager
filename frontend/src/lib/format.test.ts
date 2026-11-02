import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  formatBytes,
  formatBytesPair,
  formatCount,
  formatDuration,
  formatOsRange,
  incompatibleReasonLabel,
  launcherSelected,
  relativeTime,
} from '@/lib/format'

describe('formatCount', () => {
  it('abbreviates thousands and drops a trailing .0', () => {
    expect(formatCount(0)).toBe('0')
    expect(formatCount(999)).toBe('999')
    expect(formatCount(1000)).toBe('1k')
    expect(formatCount(1949)).toBe('1.9k')
    expect(formatCount(40_812)).toBe('40.8k')
  })
})

describe('formatBytes', () => {
  it('steps up a unit at each 1024', () => {
    expect(formatBytes(1023)).toBe('1023 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
    expect(formatBytes(3 * 1024 ** 3)).toBe('3.00 GB')
  })
})

describe('formatBytesPair', () => {
  it("shows both sides in the total's unit", () => {
    expect(formatBytesPair(512 * 1024, 3 * 1024 ** 3)).toBe('0.00 / 3.00 GB')
    expect(formatBytesPair(0, 1536)).toBe('0.0 / 1.5 KB')
  })
})

describe('formatDuration', () => {
  it('is empty for nothing to show', () => {
    expect(formatDuration(0)).toBe('')
    expect(formatDuration(-5)).toBe('')
    expect(formatDuration(Infinity)).toBe('')
  })

  it('scales from seconds to hours', () => {
    expect(formatDuration(42.4)).toBe('42s')
    expect(formatDuration(125)).toBe('2m 5s')
    expect(formatDuration(3 * 3600 + 7 * 60)).toBe('3h 7m')
  })
})

describe('relativeTime', () => {
  const now = new Date('2026-09-30T12:00:00Z').getTime()
  const ago = (seconds: number) => now / 1000 - seconds

  beforeEach(() => { vi.useFakeTimers({ now }) })
  afterEach(() => { vi.useRealTimers() })

  it('reads an unset time as never', () => {
    expect(relativeTime(0)).toBe('never')
  })

  it('names the largest whole unit', () => {
    expect(relativeTime(ago(30))).toBe('just now')
    expect(relativeTime(ago(5 * 60))).toBe('5m ago')
    expect(relativeTime(ago(3 * 3600))).toBe('3h ago')
    expect(relativeTime(ago(2 * 86400))).toBe('2d ago')
    expect(relativeTime(ago(14 * 86400))).toBe('2w ago')
    expect(relativeTime(ago(90 * 86400))).toBe('3mo ago')
    expect(relativeTime(ago(800 * 86400))).toBe('2y ago')
  })
})

describe('formatOsRange', () => {
  it('shows an exclusive upper bound as the release before it', () => {
    expect(formatOsRange({ osMin: '3.20', osMax: '3.24' })).toBe('3.20 – 3.23')
    expect(formatOsRange({ osMax: '3.24' })).toBe('≤ 3.23')
  })

  it('keeps a two-digit minor whole', () => {
    expect(formatOsRange({ osMax: '3.10' })).toBe('≤ 3.9')
  })

  it('shows a bound ending in 0 as written, since no release comes before it', () => {
    expect(formatOsRange({ osMax: '4.0' })).toBe('< 4.0')
    expect(formatOsRange({ osMin: '3.20', osMax: '4.0' })).toBe('≥ 3.20, < 4.0')
  })

  it('collapses a range of one release to that release', () => {
    expect(formatOsRange({ osMin: '3.22', osMax: '3.23' })).toBe('3.22')
  })

  it('prefers the constraints over the min and max', () => {
    expect(formatOsRange({
      osConstraints: [{ operator: '>=', version: '3.18' }, { operator: '<', version: '3.21' }],
      osMin: '1.0',
    })).toBe('3.18 – 3.20')
    expect(formatOsRange({ osConstraints: [{ operator: '=', version: '3.22.4.2' }] })).toBe('3.22.4.2')
  })

  it('is open-ended with only a minimum, and null with no bound', () => {
    expect(formatOsRange({ osMin: '3.20' })).toBe('3.20+')
    expect(formatOsRange({ osConstraints: [], osMin: null, osMax: null })).toBeNull()
  })
})

describe('incompatibleReasonLabel', () => {
  it('names which axis failed', () => {
    expect(incompatibleReasonLabel({ incompatibleReason: 'device', osMin: '3.20' })).toBe('Other models')
    expect(incompatibleReasonLabel({ incompatibleReason: 'os', osMin: '3.20' })).toBe('Needs OS 3.20+')
    expect(incompatibleReasonLabel({ incompatibleReason: 'os' })).toBe('Other OS')
  })
})

describe('launcherSelected', () => {
  it('treats empty and "none" as the stock interface', () => {
    expect(launcherSelected('')).toBe(false)
    expect(launcherSelected('none')).toBe(false)
    expect(launcherSelected('appload')).toBe(true)
  })
})
