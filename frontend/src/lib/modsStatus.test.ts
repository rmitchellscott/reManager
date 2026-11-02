import { describe, expect, it } from 'vitest'
import { deriveModsStatus, noPlanSolveFacts, type ModsFacts } from '@/lib/modsStatus'
import { pkg } from '@/test/fixtures'
import type { FollowInfo, UpgradeCandidate } from '@/lib/types'

function facts(overrides: Partial<ModsFacts> = {}): ModsFacts {
  return {
    packages: [],
    installedPackages: new Map(),
    upgradeCandidates: null,
    follows: [],
    installQueue: new Set(),
    upgradeQueue: new Set(),
    uninstallQueue: new Set(),
    solve: noPlanSolveFacts,
    ...overrides,
  }
}

function candidates(...names: string[]): Map<string, UpgradeCandidate> {
  return new Map(names.map(name => [name, { from: '1.0.0', to: '2.0.0' }]))
}

const follow: FollowInfo = { name: 'dev', followedAt: '2026-06-01', seen: null }

describe('deriveModsStatus', () => {
  it('is quiet with nothing installed, followed or staged', () => {
    const status = deriveModsStatus(facts())

    expect(status).toEqual({
      discover: { count: 0 },
      installed: { count: 0 },
      upgrades: { count: 0, unstaged: 0 },
      plan: { count: 0, level: null },
      dot: null,
      dotWithoutPlan: null,
    })
  })

  it('counts installed mods', () => {
    const status = deriveModsStatus(facts({ installedPackages: new Map([['a', '1'], ['b', '2']]) }))

    expect(status.installed.count).toBe(2)
  })

  describe('upgrades', () => {
    it('takes the solver candidates when the solve has returned, framework packages included', () => {
      const status = deriveModsStatus(facts({
        installedPackages: new Map([['a', '1.0.0']]),
        upgradeCandidates: candidates('a', 'xovi'),
      }))

      expect(status.upgrades).toEqual({ count: 2, unstaged: 2 })
      expect(status.dot).toBe('info')
    })

    it('falls back to the index versions of compatible installed packages before the solve returns', () => {
      const status = deriveModsStatus(facts({
        packages: [pkg('newer', { version: '2.0.0' }), pkg('same'), pkg('blocked', { version: '2.0.0', compatible: false })],
        installedPackages: new Map([['newer', '1.0.0'], ['same', '1.0.0'], ['blocked', '1.0.0']]),
      }))

      expect(status.upgrades.count).toBe(1)
    })

    it('stops lighting the dot once every upgrade is in the plan', () => {
      const status = deriveModsStatus(facts({
        upgradeCandidates: candidates('a', 'b'),
        upgradeQueue: new Set(['a', 'b']),
      }))

      expect(status.upgrades).toEqual({ count: 2, unstaged: 0 })
      expect(status.dotWithoutPlan).toBeNull()
      expect(status.dot).toBe('pending')
    })

    it('keeps the dot for the upgrades still outside the plan', () => {
      const status = deriveModsStatus(facts({
        upgradeCandidates: candidates('a', 'b'),
        upgradeQueue: new Set(['a']),
      }))

      expect(status.upgrades.unstaged).toBe(1)
      expect(status.dotWithoutPlan).toBe('info')
    })
  })

  describe('plan', () => {
    it('counts staged work and what the solve implies', () => {
      const status = deriveModsStatus(facts({
        installQueue: new Set(['a']),
        upgradeQueue: new Set(['b']),
        uninstallQueue: new Set(['c']),
        solve: { implied: 2, impliedRemovals: 1, conflict: false, openQuestions: 0 },
      }))

      expect(status.plan).toEqual({ count: 6, level: 'pending' })
    })

    it('has no level while nothing is staged', () => {
      const status = deriveModsStatus(facts({ solve: { implied: 2, impliedRemovals: 0, conflict: true, openQuestions: 1 } }))

      expect(status.plan.level).toBeNull()
      expect(status.dot).toBeNull()
    })

    it('is a warning while the solve has open questions and destructive on a conflict', () => {
      const staged = { installQueue: new Set(['a']) }

      expect(deriveModsStatus(facts({ ...staged, solve: { ...noPlanSolveFacts, openQuestions: 1 } })).plan.level).toBe('warning')
      expect(deriveModsStatus(facts({ ...staged, solve: { ...noPlanSolveFacts, openQuestions: 1, conflict: true } })).plan.level).toBe('destructive')
    })

    it('sets the dot over an unstaged upgrade, and leaves the dot without the plan to the upgrade', () => {
      const status = deriveModsStatus(facts({
        upgradeCandidates: candidates('a'),
        installQueue: new Set(['b']),
        solve: { ...noPlanSolveFacts, conflict: true },
      }))

      expect(status.dot).toBe('destructive')
      expect(status.dotWithoutPlan).toBe('info')
    })
  })

  describe('discover', () => {
    const fresh = pkg('fresh', { upstreamAuthor: 'dev', firstReleased: '2026-07-01' })

    it('counts what a followed developer first published after the follow', () => {
      const status = deriveModsStatus(facts({
        packages: [fresh, pkg('old', { upstreamAuthor: 'dev', firstReleased: '2026-05-01' }), pkg('other', { firstReleased: '2026-07-01' })],
        follows: [follow],
      }))

      expect(status.discover.count).toBe(1)
      expect(status.dot).toBe('unread')
    })

    it('leaves out hidden, installed, incompatible and acknowledged packages', () => {
      const status = deriveModsStatus(facts({
        packages: [
          { ...fresh, name: 'hidden', hidden: true },
          { ...fresh, name: 'installed' },
          { ...fresh, name: 'incompatible', compatible: false },
          { ...fresh, name: 'seen' },
        ],
        installedPackages: new Map([['installed', '1.0.0']]),
        follows: [{ ...follow, seen: ['seen'] }],
      }))

      expect(status.discover.count).toBe(0)
      expect(status.dot).toBeNull()
    })

    it('ranks an unstaged upgrade above unread follows', () => {
      const status = deriveModsStatus(facts({
        packages: [fresh],
        follows: [follow],
        upgradeCandidates: candidates('a'),
      }))

      expect(status.discover.count).toBe(1)
      expect(status.dot).toBe('info')
    })
  })
})
