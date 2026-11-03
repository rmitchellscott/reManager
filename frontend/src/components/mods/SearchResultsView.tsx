import { CaretDown, List, SquaresFour } from '@phosphor-icons/react'
import { Fragment, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { PackageCard, type CardContext, type CardStatus } from './PackageCard'
import { PackageInfo } from '@/lib/types'
import { useMediaQuery, COMPACT_VIEWPORT } from '@/hooks/useMediaQuery'
import {
  SEARCH_SORT_GROUPS, SORT_LABELS, type SearchSortKey,
  comparatorFor, installedOnlyPackage, relevance, statsForSort,
} from '@/lib/search'
import { cn } from '@/lib/utils'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'

interface SearchResultsViewProps {
  packages: PackageInfo[]
  installedPackages: Map<string, string>
  installTimes: Map<string, number>
  installQueue: Set<string>
  uninstallQueue: Set<string>
  impliedPackages: Set<string>
  search: string
  onSelectPackage: (pkg: PackageInfo) => void
  starred: Set<string>
  onToggleStar: (name: string) => void
  onToggleInstallQueue: (name: string) => void
  onToggleUninstallQueue: (name: string) => void
  getConflict: (pkg: PackageInfo) => string | null
  disabled: boolean
  viewMode: 'grid' | 'list'
  onViewModeChange: (mode: 'grid' | 'list') => void
}

export function SearchResultsView({
  onToggleStar,
  starred,
  packages, installedPackages, installTimes, installQueue, uninstallQueue, impliedPackages,
  search, onSelectPackage, onToggleInstallQueue, onToggleUninstallQueue,
  getConflict, disabled, viewMode, onViewModeChange,
}: SearchResultsViewProps) {
  const [sortBy, setSortBy] = useState<SearchSortKey>('relevance')
  const [showBlocked, setShowBlocked] = useState(false)

  const corpus = useMemo(() => {
    const catalogNames = new Set(packages.map(p => p.name))
    const orphans = [...installedPackages.entries()]
      .filter(([name]) => !catalogNames.has(name))
      .map(([name, version]) => installedOnlyPackage(name, version))
    return [...packages, ...orphans]
  }, [packages, installedPackages])

  const results = useMemo(() => {
    const q = search.toLowerCase()
    const hits = corpus.filter(p => relevance(p, q) > 0)
    if (sortBy === 'relevance') {
      return hits.sort((a, b) =>
        relevance(b, q) - relevance(a, q) ||
        (b.downloads30d || 0) - (a.downloads30d || 0)
      )
    }
    return hits.sort(comparatorFor(sortBy))
  }, [corpus, search, sortBy])

  const installed = useMemo(() => results.filter(p => installedPackages.has(p.name)), [results, installedPackages])
  const available = useMemo(() => results.filter(p => !installedPackages.has(p.name)), [results, installedPackages])
  const availableUsable = available.filter(p => p.compatible)
  const availableBlocked = available.filter(p => !p.compatible)

  const installedStatus = (pkg: PackageInfo): CardStatus => {
    if (uninstallQueue.has(pkg.name)) return 'remove'
    if (!pkg.compatible) return 'incompatible'
    return 'installed'
  }

  const availableStatus = (pkg: PackageInfo): CardStatus => {
    if (installQueue.has(pkg.name)) return 'queued'
    if (impliedPackages.has(pkg.name)) return 'implied'
    if (!pkg.compatible) return 'incompatible'
    return 'none'
  }

  const compact = useMediaQuery(COMPACT_VIEWPORT)
  const effectiveView = compact ? 'grid' : viewMode

  const viewToggleLabel = viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'
  const stats = statsForSort(sortBy)

  const renderGroup = (pkgs: PackageInfo[], context: CardContext, status: (p: PackageInfo) => CardStatus) => {
    const cards = pkgs.map(p => (
      <PackageCard
        key={p.name} pkg={p} status={status(p)} context={context}
        starred={starred.has(p.name)}
        onToggleStar={onToggleStar}
        variant={effectiveView === 'list' ? 'row' : 'grid'}
        onClick={onSelectPackage}
        onAction={context === 'installed' ? onToggleUninstallQueue : onToggleInstallQueue}
        conflict={context === 'installed' ? null : getConflict(p)}
        disabled={disabled}
        installedVersion={installedPackages.get(p.name)}
        installTime={installTimes.get(p.name)}
        dlField={stats.dlField} timeField={stats.timeField}
      />
    ))
    return effectiveView === 'list'
      ? <div className="rounded-[var(--radius)] border border-border bg-card">{cards}</div>
      : <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">{cards}</div>
  }

  const groupHeading = (label: string, count: number) => (
    <h3 className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2.5">
      {label} ({count})
    </h3>
  )

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 min-h-8">
        <h3 className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground">
          {results.length} result{results.length !== 1 ? 's' : ''} for "{search}"
        </h3>
        <div className="flex items-center gap-2 ml-auto">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" className="h-7 px-2.5 text-xs gap-1 max-w-full min-w-0 touch:hit-target">
                <span className="truncate">{SORT_LABELS[sortBy]}</span>
                <CaretDown className="h-4 w-4 shrink-0" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-[11rem]">
              <DropdownMenuRadioGroup value={sortBy} onValueChange={v => setSortBy(v as SearchSortKey)}>
                {SEARCH_SORT_GROUPS.map((group, i) => (
                  <Fragment key={group[0]}>
                    {i > 0 && <DropdownMenuSeparator />}
                    {group.map(key => (
                      <DropdownMenuRadioItem key={key} value={key}>{SORT_LABELS[key]}</DropdownMenuRadioItem>
                    ))}
                  </Fragment>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
          <button
            className="hidden sm:inline-flex items-center justify-center w-7 h-7 hit-target bg-transparent border-none text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => onViewModeChange(viewMode === 'grid' ? 'list' : 'grid')}
            aria-label={viewToggleLabel}
          >
            {viewMode === 'grid' ? <List className="h-3.5 w-3.5" /> : <SquaresFour className="h-3.5 w-3.5" />}
          </button>
            </TooltipTrigger>
            <TooltipContent>{viewToggleLabel}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {results.length === 0 ? (
        <p className="text-center text-muted-foreground py-8 text-sm">No mods found.</p>
      ) : (
        <>
          {installed.length > 0 && (
            <div>
              {groupHeading('Installed', installed.length)}
              {renderGroup(installed, 'installed', installedStatus)}
            </div>
          )}
          {availableUsable.length > 0 && (
            <div>
              {groupHeading('Available', availableUsable.length)}
              {renderGroup(availableUsable, 'discover', availableStatus)}
            </div>
          )}
          {availableBlocked.length > 0 && (
            <div>
              <button
                onClick={() => setShowBlocked(v => !v)}
                aria-expanded={showBlocked}
                className="flex w-full items-center gap-1.5 py-2 text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer"
              >
                <CaretDown className={cn('h-3.5 w-3.5 transition-transform', !showBlocked && '-rotate-90')} />
                {availableBlocked.length} incompatible
              </button>
              {showBlocked && <div className="mt-1">{renderGroup(availableBlocked, 'discover', availableStatus)}</div>}
            </div>
          )}
        </>
      )}
    </div>
  )
}
