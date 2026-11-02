import type { PackageInfo } from '@/lib/types'

export function pkg(name: string, overrides: Partial<PackageInfo> = {}): PackageInfo {
  return {
    name,
    version: '1.0.0',
    description: '',
    upstreamAuthor: 'someone',
    categories: null,
    url: '',
    license: '',
    devices: null,
    depends: null,
    conflicts: null,
    provides: null,
    osMin: null,
    osMax: null,
    osConstraints: null,
    compatible: true,
    status: '',
    donateUrl: null,
    readmeUrl: null,
    changelogUrl: null,
    released: '2026-01-01',
    firstReleased: '2026-01-01',
    downloads30d: 0,
    downloadsAll: 0,
    isFramework: false,
    hidden: false,
    ...overrides,
  }
}
