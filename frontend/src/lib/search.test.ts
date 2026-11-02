import { describe, expect, it } from 'vitest'
import { comparatorFor, installedOnlyPackage, relevance, statsForSort } from '@/lib/search'
import { pkg } from '@/test/fixtures'

describe('relevance', () => {
  const candidates = [
    pkg('described', { description: 'Adds a ruler tool' }),
    pkg('by-author', { upstreamAuthor: 'ruler' }),
    pkg('my-ruler-tool'),
    pkg('rulerplus'),
    pkg('ruler'),
  ]

  it('ranks name matches above author matches above description matches', () => {
    const ranked = [...candidates].sort((a, b) => relevance(b, 'ruler') - relevance(a, 'ruler')).map(p => p.name)

    expect(ranked).toEqual(['ruler', 'rulerplus', 'my-ruler-tool', 'by-author', 'described'])
  })

  it('is zero for no match', () => {
    expect(relevance(pkg('clock'), 'ruler')).toBe(0)
  })
})

describe('comparatorFor', () => {
  it('sorts downloads high to low and names case-insensitively', () => {
    const packages = [pkg('Banana', { downloads30d: 5 }), pkg('apple', { downloads30d: 0 }), pkg('Cherry', { downloads30d: 50 })]

    expect([...packages].sort(comparatorFor('popular')).map(p => p.name)).toEqual(['Cherry', 'Banana', 'apple'])
    expect([...packages].sort(comparatorFor('name')).map(p => p.name)).toEqual(['apple', 'Banana', 'Cherry'])
  })

  it('sorts dates newest first', () => {
    const packages = [pkg('old', { released: '2025-01-01' }), pkg('new', { released: '2026-01-01' })]

    expect([...packages].sort(comparatorFor('recent')).map(p => p.name)).toEqual(['new', 'old'])
  })
})

describe('statsForSort', () => {
  it('shows the stat the list is ranked by', () => {
    expect(statsForSort('popular')).toEqual({ dlField: 'downloads30d', timeField: 'released' })
    expect(statsForSort('added')).toEqual({ dlField: 'downloadsAll', timeField: 'firstReleased' })
  })
})

describe('installedOnlyPackage', () => {
  it('stays searchable and compatible with almost no metadata', () => {
    const p = installedOnlyPackage('local-mod', '0.1.0')

    expect(p).toMatchObject({ name: 'local-mod', version: '0.1.0', compatible: true, hidden: false })
    expect(relevance(p, 'local-mod')).toBe(100)
  })
})
