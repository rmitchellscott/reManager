import { FollowInfo, PackageInfo } from '@/lib/types'

export function followFeedPackages(
  packages: PackageInfo[],
  follows: FollowInfo[],
  installedPackages: Map<string, string>,
  browseOnly: boolean,
): PackageInfo[] {
  if (follows.length === 0) return []
  const baselines = new Map(follows.map(f => [f.name, f.followedAt]))
  const acknowledged = new Set(follows.flatMap(f => f.seen))
  return packages.filter(p => {
    const followedAt = baselines.get(p.upstreamAuthor)
    if (!followedAt) return false
    if (installedPackages.has(p.name)) return false
    if (!browseOnly && !p.compatible) return false
    if (!p.firstReleased || p.firstReleased <= followedAt) return false
    return !acknowledged.has(p.name)
  }).sort((a, b) => (b.firstReleased || '').localeCompare(a.firstReleased || ''))
}
