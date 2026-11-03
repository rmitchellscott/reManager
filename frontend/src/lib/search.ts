import { PackageInfo } from '@/lib/types'

export type SortKey = 'popular' | 'downloads' | 'added' | 'recent' | 'name'
export type SearchSortKey = 'relevance' | SortKey

export const SORT_GROUPS: SortKey[][] = [['popular', 'downloads'], ['added', 'recent'], ['name']]
export const SEARCH_SORT_GROUPS: SearchSortKey[][] = [['relevance'], ...SORT_GROUPS]

export const SORT_LABELS: Record<SearchSortKey, string> = {
  relevance: 'Relevance',
  popular: 'Popular',
  downloads: 'Downloads',
  added: 'Newest',
  recent: 'Updated',
  name: 'A–Z',
}

export function statsForSort(key: SearchSortKey): { dlField: 'downloads30d' | 'downloadsAll'; timeField: 'released' | 'firstReleased' } {
  return {
    dlField: key === 'popular' ? 'downloads30d' : 'downloadsAll',
    timeField: key === 'added' ? 'firstReleased' : 'released',
  }
}

export function comparatorFor(key: SortKey) {
  return (a: PackageInfo, b: PackageInfo) => {
    switch (key) {
      case 'popular': return (b.downloads30d || 0) - (a.downloads30d || 0)
      case 'downloads': return (b.downloadsAll || 0) - (a.downloadsAll || 0)
      case 'added': return (b.firstReleased || '').localeCompare(a.firstReleased || '')
      case 'recent': return (b.released || '').localeCompare(a.released || '')
      case 'name': return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    }
  }
}

export function relevance(p: PackageInfo, q: string): number {
  const name = p.name.toLowerCase()
  const author = (p.upstreamAuthor || '').toLowerCase()
  if (name === q) return 100
  if (name.startsWith(q)) return 80
  if (name.includes(q)) return 60
  if (author === q) return 50
  if (author.includes(q)) return 40
  if (p.description.toLowerCase().includes(q)) return 20
  return 0
}

export function installedOnlyPackage(name: string, version: string): PackageInfo {
  return unindexedPackage(name, version)
}

function unindexedPackage(name: string, version: string): PackageInfo {
  return {
    name,
    version,
    description: '',
    upstreamAuthor: '',
    categories: [],
    url: '',
    license: '',
    devices: [],
    depends: [],
    conflicts: [],
    provides: [],
    osMin: null,
    osMax: null,
    osConstraints: null,
    compatible: true,
    status: '',
    donateUrl: null,
    readmeUrl: null,
    changelogUrl: null,
    released: '',
    firstReleased: '',
    downloads30d: 0,
    downloadsAll: 0,
    isFramework: false,
    hidden: false,
  }
}
