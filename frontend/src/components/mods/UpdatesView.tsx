import { Check, List, SquaresFour } from '@phosphor-icons/react'
import { useCallback, useMemo } from 'react'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { PackageCard } from './PackageCard'
import { PackageInfo, UpgradeCandidate } from '@/lib/types'
import { useMediaQuery, COMPACT_VIEWPORT } from '@/hooks/useMediaQuery'
import { BulkStageLine } from './BulkStageLine'
import { installedVersionOf } from '@/lib/updates'
interface UpdatesViewProps {
  packages: PackageInfo[]
  installedPackages: Map<string, string>
  upgradeQueue: Set<string>
  uninstallQueue: Set<string>
  upgradeCandidates: Map<string, UpgradeCandidate> | null
  onSelectPackage: (pkg: PackageInfo) => void
  starred: Set<string>
  onToggleStar: (name: string) => void
  onToggleUpgradeQueue: (name: string) => void
  onToggleUninstallQueue: (name: string) => void
  onUpdateAll: () => void
  onClearUpgradeQueue: () => void
  disabled: boolean
  viewMode: 'grid' | 'list'
  onViewModeChange: (mode: 'grid' | 'list') => void
}

function hasUpdate(pkg: PackageInfo, installedVersion: string | undefined): boolean {
  if (!installedVersion) return false
  return pkg.version !== installedVersion
}

export function UpdatesView({
  onToggleStar,
  starred,
  packages, installedPackages, upgradeQueue, uninstallQueue, upgradeCandidates,
  onSelectPackage, onToggleUpgradeQueue, onToggleUninstallQueue, onUpdateAll, onClearUpgradeQueue, disabled,
  viewMode, onViewModeChange,
}: UpdatesViewProps) {
  const updatable = useMemo(() => {
    const result = upgradeCandidates
      ? packages.filter(p => upgradeCandidates.has(p.name))
      : packages.filter(p => {
          const iv = installedPackages.get(p.name)
          return iv && hasUpdate(p, iv) && p.compatible
        })
    return result.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
  }, [packages, installedPackages, upgradeCandidates])


  const cardStatus = (name: string) =>
    uninstallQueue.has(name) ? 'remove' as const
      : upgradeQueue.has(name) ? 'queued' as const
        : 'update' as const
  const cardAction = useCallback((name: string) => {
    uninstallQueue.has(name) ? onToggleUninstallQueue(name) : onToggleUpgradeQueue(name)
  }, [uninstallQueue, onToggleUninstallQueue, onToggleUpgradeQueue])

  const currentVersion = (name: string) =>
    installedVersionOf(name, installedPackages, upgradeCandidates)

  const compact = useMediaQuery(COMPACT_VIEWPORT)
  const effectiveView = compact ? 'grid' : viewMode

  const viewToggleIcon = viewMode === 'grid'
    ? <List className="h-3.5 w-3.5" />
    : <SquaresFour className="h-3.5 w-3.5" />
  const viewToggleLabel = viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'

  const staged = updatable.filter(p => upgradeQueue.has(p.name)).length

  const viewToggle = (
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
  )

  return (
    <div>
      {updatable.length > 0 ? (
        <BulkStageLine
          className="mb-2.5 min-h-8"
          staged={staged}
          total={updatable.length}
          idleLabel={updatable.length === 1 ? '1 upgrade available' : `${updatable.length} upgrades available`}
          onAddAll={onUpdateAll}
          onClear={onClearUpgradeQueue}
          disabled={disabled}
        >
          {viewToggle}
        </BulkStageLine>
      ) : (
        <div className="flex items-center justify-end mb-2.5 min-h-8">{viewToggle}</div>
      )}

      {updatable.length === 0 ? (
        <div className="text-center py-12">
          <Check className="h-4 w-4 inline-block text-[var(--success-text)] mb-3" />
          <p className="text-sm font-medium mb-1">Everything is up to date</p>
          <p className="text-xs text-muted-foreground">All installed mods are on their latest versions.</p>
        </div>
      ) : effectiveView === 'list' ? (
        <div className="rounded-[var(--radius)] border border-border bg-card">
          {updatable.map(p => (
            <PackageCard
              key={p.name} pkg={p} status={cardStatus(p.name)} context="updates" variant="row"
              starred={starred.has(p.name)}
              onToggleStar={onToggleStar}
              onClick={onSelectPackage} onAction={cardAction}
              installedVersion={currentVersion(p.name)} targetVersion={upgradeCandidates?.get(p.name)?.to} disabled={disabled}
            />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {updatable.map(p => (
            <PackageCard
              key={p.name} pkg={p} status={cardStatus(p.name)} context="updates" variant="grid"
              starred={starred.has(p.name)}
              onToggleStar={onToggleStar}
              onClick={onSelectPackage} onAction={cardAction}
              installedVersion={currentVersion(p.name)} targetVersion={upgradeCandidates?.get(p.name)?.to} disabled={disabled}
            />
          ))}
        </div>
      )}
    </div>
  )
}
