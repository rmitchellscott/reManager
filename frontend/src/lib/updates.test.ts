import { describe, expect, it } from 'vitest'
import { availableUpdateNames, installedVersionOf, upgradeCandidateMap, withoutUninstalled } from '@/lib/updates'
import { pkg } from '@/test/fixtures'
import type { UpgradeCandidate } from '@/lib/types'

const candidate: UpgradeCandidate = { from: '1.0.0', to: '2.0.0' }

describe('installedVersionOf', () => {
  it('falls back to the solver for a framework package the installed map leaves out', () => {
    const installed = new Map([['mod', '1.2.0']])
    const candidates = new Map([['xovi', candidate]])

    expect(installedVersionOf('mod', installed, candidates)).toBe('1.2.0')
    expect(installedVersionOf('xovi', installed, candidates)).toBe('1.0.0')
    expect(installedVersionOf('xovi', installed, null)).toBeUndefined()
  })
})

describe('availableUpdateNames', () => {
  it('lists installed packages whose compatible index version differs', () => {
    const packages = [pkg('newer', { version: '2.0.0' }), pkg('same'), pkg('blocked', { version: '2.0.0', compatible: false })]
    const installed = new Map([['newer', '1.0.0'], ['same', '1.0.0'], ['blocked', '1.0.0'], ['unindexed', '1.0.0']])

    expect(availableUpdateNames(packages, installed)).toEqual(['newer'])
  })
})

describe('withoutUninstalled', () => {
  it('drops candidates for packages that left the installed map', () => {
    const candidates = new Map([['gone', candidate], ['kept', candidate]])
    const result = withoutUninstalled(candidates, new Map([['gone', '1'], ['kept', '1']]), new Map([['kept', '1']]))

    expect([...result!.keys()]).toEqual(['kept'])
    expect(candidates.size).toBe(2)
  })

  it('keeps a framework package, which is never in the installed map, for the solve to settle', () => {
    const candidates = new Map([['xovi', candidate]])

    expect(withoutUninstalled(candidates, new Map([['mod', '1']]), new Map())).toBe(candidates)
  })

  it('passes null through', () => {
    expect(withoutUninstalled(null, new Map(), new Map())).toBeNull()
  })
})

describe('upgradeCandidateMap', () => {
  it('skips missing entries and accepts a null record', () => {
    expect([...upgradeCandidateMap({ a: candidate, b: undefined })]).toEqual([['a', candidate]])
    expect(upgradeCandidateMap(null).size).toBe(0)
  })
})
