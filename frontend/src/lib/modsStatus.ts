import { followFeedPackages } from '@/lib/followFeed'
import { availableUpdateNames } from '@/lib/updates'
import type { FollowInfo, PackageInfo, UpgradeCandidate } from '@/lib/types'

export type PlanLevel = 'pending' | 'warning' | 'destructive'
export type ModsDotLevel = 'unread' | 'info' | PlanLevel

export interface PlanSolveFacts {
  implied: number
  impliedRemovals: number
  conflict: boolean
  openQuestions: number
}

export const noPlanSolveFacts: PlanSolveFacts = { implied: 0, impliedRemovals: 0, conflict: false, openQuestions: 0 }

export interface ModsFacts {
  packages: PackageInfo[]
  installedPackages: Map<string, string>
  upgradeCandidates: Map<string, UpgradeCandidate> | null
  follows: FollowInfo[]
  installQueue: Set<string>
  upgradeQueue: Set<string>
  uninstallQueue: Set<string>
  solve: PlanSolveFacts
}

export interface ModsStatus {
  discover: { count: number }
  installed: { count: number }
  upgrades: { count: number; unstaged: number }
  plan: { count: number; level: PlanLevel | null }
  dot: ModsDotLevel | null
  dotWithoutPlan: ModsDotLevel | null
}

export function deriveModsStatus(facts: ModsFacts): ModsStatus {
  const { packages, installedPackages, upgradeCandidates, follows, installQueue, upgradeQueue, uninstallQueue, solve } = facts

  const unread = followFeedPackages(packages.filter(p => !p.hidden), follows, installedPackages, false).length

  const upgradable = upgradeCandidates ? [...upgradeCandidates.keys()] : availableUpdateNames(packages, installedPackages)
  const unstaged = upgradable.filter(name => !upgradeQueue.has(name)).length

  const staged = installQueue.size + upgradeQueue.size + uninstallQueue.size
  const planCount = staged + solve.implied + solve.impliedRemovals
  const planLevel: PlanLevel | null = staged === 0 ? null
    : solve.conflict ? 'destructive'
    : solve.openQuestions > 0 ? 'warning'
    : 'pending'

  const dotWithoutPlan: ModsDotLevel | null = unstaged > 0 ? 'info' : unread > 0 ? 'unread' : null

  return {
    discover: { count: unread },
    installed: { count: installedPackages.size },
    upgrades: { count: upgradable.length, unstaged },
    plan: { count: planCount, level: planLevel },
    dot: planLevel ?? dotWithoutPlan,
    dotWithoutPlan,
  }
}
