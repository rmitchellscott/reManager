import { describe, expect, it } from 'vitest'
import { IANA_TIMEZONES } from '@/lib/timezones'

describe('IANA_TIMEZONES', () => {
  it('names only zones the runtime recognizes, once each', () => {
    for (const zone of IANA_TIMEZONES) {
      expect(() => new Intl.DateTimeFormat('en-US', { timeZone: zone }), zone).not.toThrow()
    }
    expect(new Set(IANA_TIMEZONES).size).toBe(IANA_TIMEZONES.length)
  })
})
