import { PackageInfo, UpgradeCandidate } from '@/lib/types'

export function installedVersionOf(
  name: string,
  installedPackages: Map<string, string>,
  upgradeCandidates: Map<string, UpgradeCandidate> | null,
): string | undefined {
  return installedPackages.get(name) ?? upgradeCandidates?.get(name)?.from
}

export function availableUpdateNames(
  packages: PackageInfo[],
  installedPackages: Map<string, string>,
): string[] {
  const names: string[] = []
  for (const [name, installedVersion] of installedPackages) {
    const pkg = packages.find(p => p.name === name)
    if (pkg && pkg.version !== installedVersion && pkg.compatible) names.push(name)
  }
  return names
}

export function withoutUninstalled(
  upgradeCandidates: Map<string, UpgradeCandidate> | null,
  installedBefore: Map<string, string>,
  installedAfter: Map<string, string>,
): Map<string, UpgradeCandidate> | null {
  if (!upgradeCandidates) return upgradeCandidates
  const uninstalled = [...installedBefore.keys()].filter(name => !installedAfter.has(name) && upgradeCandidates.has(name))
  if (uninstalled.length === 0) return upgradeCandidates
  const remaining = new Map(upgradeCandidates)
  for (const name of uninstalled) remaining.delete(name)
  return remaining
}

export function upgradeCandidateMap(
  candidates: Record<string, UpgradeCandidate | undefined> | null | undefined,
): Map<string, UpgradeCandidate> {
  return new Map(Object.entries(candidates || {}).filter((e): e is [string, UpgradeCandidate] => e[1] !== undefined))
}
