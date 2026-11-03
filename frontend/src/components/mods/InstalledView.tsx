import { List, Package, SquaresFour } from '@phosphor-icons/react'
import { useMemo } from 'react'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { PackageCard, type CardStatus } from './PackageCard'
import { PackageInfo } from '@/lib/types'
import { useMediaQuery, COMPACT_VIEWPORT } from '@/hooks/useMediaQuery'
import { installedOnlyPackage } from '@/lib/search'
interface InstalledViewProps {
  packages: PackageInfo[]
  installedPackages: Map<string, string>
  installTimes: Map<string, number>
  uninstallQueue: Set<string>
  onSelectPackage: (pkg: PackageInfo) => void
  starred: Set<string>
  onToggleStar: (name: string) => void
  onToggleUninstallQueue: (name: string) => void
  disabled: boolean
  viewMode: 'grid' | 'list'
  onViewModeChange: (mode: 'grid' | 'list') => void
}

export function InstalledView({
  onToggleStar,
  starred,
  packages, installedPackages, installTimes, uninstallQueue,
  onSelectPackage, onToggleUninstallQueue, disabled,
  viewMode, onViewModeChange,
}: InstalledViewProps) {
  const installedPkgs = useMemo(() => {
    const inCatalog = packages.filter(p => installedPackages.has(p.name))
    const catalogNames = new Set(packages.map(p => p.name))
    const installedOnly = Array.from(installedPackages.entries())
      .filter(([name]) => !catalogNames.has(name))
      .map(([name, version]) => installedOnlyPackage(name, version))
    return [...inCatalog, ...installedOnly].sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
  }, [packages, installedPackages])

  const getCardStatus = (pkg: PackageInfo): CardStatus => {
    if (uninstallQueue.has(pkg.name)) return 'remove'
    if (!pkg.compatible) return 'incompatible'
    return 'installed'
  }

  const compact = useMediaQuery(COMPACT_VIEWPORT)
  const effectiveView = compact ? 'grid' : viewMode

  const viewToggleIcon = viewMode === 'grid'
    ? <List className="h-3.5 w-3.5" />
    : <SquaresFour className="h-3.5 w-3.5" />
  const viewToggleLabel = viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'

  return (
    <div>
      <div className="flex items-center justify-end mb-2.5 min-h-8">
        <Tooltip>
          <TooltipTrigger asChild>
          <button
            className="hidden sm:inline-flex items-center justify-center w-7 h-7 hit-target bg-transparent border-none text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => onViewModeChange(viewMode === 'grid' ? 'list' : 'grid')}
            aria-label={viewToggleLabel}
          >
            {viewToggleIcon}
          </button>
          </TooltipTrigger>
          <TooltipContent>{viewToggleLabel}</TooltipContent>
        </Tooltip>
      </div>

      {installedPkgs.length === 0 ? (
        <div className="text-center py-12">
          <Package className="h-4 w-4 inline-block text-muted-foreground mb-3" />
          <p className="text-sm font-medium mb-1">No mods installed</p>
          <p className="text-xs text-muted-foreground">Browse the Discover tab to find mods.</p>
        </div>
      ) : effectiveView === 'list' ? (
        <div className="rounded-[var(--radius)] border border-border bg-card">
          {installedPkgs.map(p => (
            <PackageCard
              key={p.name} pkg={p} status={getCardStatus(p)} context="installed" variant="row"
              starred={starred.has(p.name)}
              onToggleStar={onToggleStar}
              onClick={onSelectPackage} onAction={onToggleUninstallQueue}
              disabled={disabled} installedVersion={installedPackages.get(p.name)} installTime={installTimes.get(p.name)}
            />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {installedPkgs.map(p => (
            <PackageCard
              key={p.name} pkg={p} status={getCardStatus(p)} context="installed" variant="grid"
              starred={starred.has(p.name)}
              onToggleStar={onToggleStar}
              onClick={onSelectPackage} onAction={onToggleUninstallQueue}
              disabled={disabled} installedVersion={installedPackages.get(p.name)} installTime={installTimes.get(p.name)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
