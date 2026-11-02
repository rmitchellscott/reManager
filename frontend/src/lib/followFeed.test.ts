import { describe, expect, it } from 'vitest'
import { followFeedPackages } from '@/lib/followFeed'
import { pkg } from '@/test/fixtures'
import type { FollowInfo } from '@/lib/types'

const follow: FollowInfo = { name: 'dev', followedAt: '2026-06-01', seen: [] }
const byDev = (name: string, firstReleased: string, compatible = true) =>
  pkg(name, { upstreamAuthor: 'dev', firstReleased, compatible })

describe('followFeedPackages', () => {
  it('lists new work since the follow, newest first', () => {
    const packages = [byDev('july', '2026-07-01'), byDev('august', '2026-08-01'), byDev('back-catalog', '2026-05-01')]

    expect(followFeedPackages(packages, [follow], new Map(), false).map(p => p.name)).toEqual(['august', 'july'])
  })

  it('does not count a release on the follow day itself', () => {
    expect(followFeedPackages([byDev('same-day', '2026-06-01')], [follow], new Map(), false)).toEqual([])
  })

  it('is empty with no follows', () => {
    expect(followFeedPackages([byDev('july', '2026-07-01')], [], new Map(), false)).toEqual([])
  })

  it('only drops incompatible packages when a tablet is attached', () => {
    const packages = [byDev('other-os', '2026-07-01', false)]

    expect(followFeedPackages(packages, [follow], new Map(), false)).toEqual([])
    expect(followFeedPackages(packages, [follow], new Map(), true).map(p => p.name)).toEqual(['other-os'])
  })
})
