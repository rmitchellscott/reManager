import { ArrowLeft, CaretDown, DownloadSimple, List, Package, SquaresFour, Star } from '@phosphor-icons/react'
import { Fragment, useEffect, useMemo, useState, useCallback } from 'react'
import type { MutableRefObject } from 'react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { PackageCard, StarToggle, cardActionHeight, cardFocusRing, cardTrigger, type CardStatus } from './PackageCard'
import { ScrollRow } from './ScrollRow'
import { CategoryChips, categoryLabel } from './CategoryChips'
import { BulkStageLine } from './BulkStageLine'
import { DeveloperCard } from './DeveloperCard'
import { FollowInfo, PackageInfo } from '@/lib/types'
import { followFeedPackages } from '@/lib/followFeed'
import { useMediaQuery, COMPACT_VIEWPORT } from '@/hooks/useMediaQuery'
import { useBackHandler } from '@/hooks/useBackHandler'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { formatCount } from '@/lib/format'
import { SORT_GROUPS, SORT_LABELS, type SortKey, comparatorFor, statsForSort } from '@/lib/search'
import { cn } from '@/lib/utils'

function compatibleFirst(a: PackageInfo, b: PackageInfo): number {
  if (a.compatible === b.compatible) return 0
  return a.compatible ? -1 : 1
}

function SortAndViewControls({ value, onChange, groups, viewMode, onViewModeChange }: {
  value: SortKey
  onChange: (v: SortKey) => void
  groups: SortKey[][]
  viewMode: 'grid' | 'list'
  onViewModeChange: (mode: 'grid' | 'list') => void
}) {
  const viewToggleLabel = viewMode === 'grid' ? 'Switch to list view' : 'Switch to grid view'
  return (
    <div className="flex items-center gap-2 ml-auto">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="h-7 px-2.5 text-xs gap-1 max-w-full min-w-0">
            <span className="truncate">{SORT_LABELS[value]}</span>
            <CaretDown className="h-4 w-4 shrink-0" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-[11rem]">
          <DropdownMenuRadioGroup value={value} onValueChange={v => onChange(v as SortKey)}>
            {groups.map((group, i) => (
              <Fragment key={group[0]}>
                {i > 0 && <DropdownMenuSeparator />}
                {group.map(key => (
                  <DropdownMenuRadioItem key={key} value={key}>
                    {SORT_LABELS[key]}
                  </DropdownMenuRadioItem>
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
  )
}

interface DiscoverViewProps {
  packages: PackageInfo[]
  catalogStatus?: 'loading' | 'ready' | 'error'
  installedPackages: Map<string, string>
  installQueue: Set<string>
  impliedPackages: Set<string>
  uninstallQueue: Set<string>
  onSelectPackage: (pkg: PackageInfo) => void
  onToggleInstallQueue: (name: string) => void
  getConflict: (pkg: PackageInfo) => string | null
  disabled: boolean
  followedDevs: Set<string>
  follows: FollowInfo[]
  onToggleFollow: (dev: string) => void
  onSetPackageSeen: (dev: string, pkg: string, seen: boolean) => void
  starred: Set<string>
  onToggleStar: (name: string) => void
  onInstallStarred: (names: string[]) => void
  onCancelStarred: (names: string[]) => void
  browseOnly?: boolean
  showDeveloperRef?: MutableRefObject<((dev: string) => void) | null>
  // Radix Tabs fires no onChange when its value is unchanged, so re-pressing the active Discover tab reaches the reset through this ref rather than a prop.
  resetToOverviewRef?: MutableRefObject<(() => void) | null>
  visible?: boolean
  viewMode: 'grid' | 'list'
  onViewModeChange: (mode: 'grid' | 'list') => void
}

export function DiscoverView({
  onToggleStar,
  packages, catalogStatus = 'ready', installedPackages, installQueue, impliedPackages, uninstallQueue,
  onSelectPackage, onToggleInstallQueue, getConflict, disabled,
  followedDevs, follows, onToggleFollow, onSetPackageSeen, starred, onInstallStarred, onCancelStarred, browseOnly = false, showDeveloperRef, resetToOverviewRef, viewMode, onViewModeChange, visible = true,
}: DiscoverViewProps) {
  const [selectedCategory, setSelectedCategory] = useState('overview')
  const [sortBy, setSortBy] = useState<SortKey>('popular')
  // Each entry captures its own label on the way in; deriving it from category would render two different views under the same name.
  const [returnStack, setReturnStack] = useState<{ label: string; category: string; overview: boolean; devView: boolean }[]>([])
  const returnTo = returnStack[returnStack.length - 1] ?? null
  const [filteredTitle, setFilteredTitle] = useState('All mods')
  const [inFilteredView, setInFilteredView] = useState(false)
  const [inDevView, setInDevView] = useState(false)
  const [showBlocked, setShowBlocked] = useState(false)
  const [seenThisVisit, setSeenThisVisit] = useState<Set<string>>(new Set())

  const compatiblePackages = useMemo(() => packages.filter(p => p.compatible), [packages])

  const { categoryNames, categoryCounts } = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const p of packages) {
      for (const c of (p.categories || [])) counts[c] = (counts[c] || 0) + 1
    }
    const names = Object.keys(counts).sort((a, b) => counts[b] - counts[a] || a.localeCompare(b))
    return { categoryNames: names, categoryCounts: counts }
  }, [packages])

  const developers = useMemo(() => {
    const devMap: Record<string, { name: string; pkgs: string[]; dl: number; latest: string }> = {}
    for (const p of packages) {
      const d = p.upstreamAuthor || 'unknown'
      if (!devMap[d]) devMap[d] = { name: d, pkgs: [], dl: 0, latest: '' }
      devMap[d].pkgs.push(p.name)
      devMap[d].dl += p.downloadsAll || 0
      if ((p.firstReleased || '') > devMap[d].latest) devMap[d].latest = p.firstReleased || ''
    }
    return Object.values(devMap).sort((a, b) => b.dl - a.dl || b.pkgs.length - a.pkgs.length)
  }, [packages])

  const developersByNewest = useMemo(
    () => [...developers].sort((a, b) => b.latest.localeCompare(a.latest)),
    [developers],
  )

  const missingStarred = useMemo(() => {
    const known = new Set(packages.map(p => p.name))
    return [...starred].filter(name => !known.has(name)).sort()
  }, [packages, starred])

  const sortedPackages = useMemo(() => {
    const pkgs = selectedCategory === 'starred'
      ? packages.filter(p => starred.has(p.name))
      : selectedCategory !== 'all' && selectedCategory !== 'overview'
        ? packages.filter(p => (p.categories || []).includes(selectedCategory))
        : [...packages]
    const installedLast = (a: PackageInfo, b: PackageInfo) =>
      selectedCategory === 'starred'
        ? Number(installedPackages.has(a.name)) - Number(installedPackages.has(b.name))
        : 0
    return pkgs.sort((a, b) => compatibleFirst(a, b) || installedLast(a, b) || comparatorFor(sortBy)(a, b))
  }, [packages, selectedCategory, sortBy, starred, installedPackages])

  const userFacingPackages = useMemo(() =>
    compatiblePackages.filter(p =>
      !p.isFramework &&
      p.status !== 'unmaintained' &&
      p.status !== 'deprecated' &&
      !installedPackages.has(p.name)
    ),
    [compatiblePackages, installedPackages]
  )

  const popular = useMemo(() =>
    [...userFacingPackages].sort((a, b) => (b.downloads30d || 0) - (a.downloads30d || 0)).slice(0, 8),
    [userFacingPackages]
  )

  const recentlyAdded = useMemo(() =>
    [...userFacingPackages].sort((a, b) => (b.firstReleased || '').localeCompare(a.firstReleased || '')).slice(0, 8),
    [userFacingPackages]
  )

  const followFeedItems = useMemo(() => {
    const unseen = followFeedPackages(packages, follows, installedPackages, browseOnly)
    if (seenThisVisit.size === 0) return unseen
    const shown = new Set(unseen.map(p => p.name))
    const lingering = packages.filter(p => seenThisVisit.has(p.name) && !shown.has(p.name))
    return [...unseen, ...lingering].sort((a, b) => (b.firstReleased || '').localeCompare(a.firstReleased || ''))
  }, [packages, follows, installedPackages, browseOnly, seenThisVisit])

  const seenPackages = useMemo(() => new Set(follows.flatMap(f => f.seen)), [follows])

  const markSeen = useCallback((name: string, seen: boolean) => {
    const pkg = packages.find(p => p.name === name)
    if (!pkg) return
    setSeenThisVisit(prev => {
      const next = new Set(prev)
      seen ? next.add(name) : next.delete(name)
      return next
    })
    onSetPackageSeen(pkg.upstreamAuthor, name, seen)
  }, [packages, onSetPackageSeen])

  const getCardStatus = useCallback((pkg: PackageInfo): CardStatus => {
    if (uninstallQueue.has(pkg.name)) return 'remove'
    if (installedPackages.has(pkg.name)) return 'installed'
    if (installQueue.has(pkg.name)) return 'queued'
    if (impliedPackages.has(pkg.name)) return 'implied'
    if (!pkg.compatible) return 'incompatible'
    return 'none'
  }, [installedPackages, installQueue, impliedPackages, uninstallQueue])

  const navigateTo = useCallback((title: string, opts: { cat?: string; devView?: boolean; sort?: SortKey }) => {
    setReturnStack(prev => [...prev, {
      label: inFilteredView ? filteredTitle : 'Overview',
      category: selectedCategory,
      overview: !inFilteredView,
      devView: inDevView,
    }])
    setFilteredTitle(title)
    if (opts.cat !== undefined) setSelectedCategory(opts.cat)
    setInDevView(opts.devView || false)
    setInFilteredView(true)
    if (opts.sort) setSortBy(opts.sort)
  }, [filteredTitle, selectedCategory, inFilteredView, inDevView])

  const navigateBack = () => {
    const target = returnStack[returnStack.length - 1]
    if (!target) return
    setReturnStack(s => s.slice(0, -1))
    setInDevView(target.devView)
    if (target.overview) {
      setSelectedCategory('overview')
      setInFilteredView(false)
      setFilteredTitle('All mods')
    } else {
      setSelectedCategory(target.category)
      setFilteredTitle(target.label)
    }
  }

  const { ref: viewRef, swipeHandlers: edgeSwipe } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: navigateBack,
    restoreAfterDismiss: true,
  })
  useBackHandler(navigateBack, visible && returnTo !== null, edgeSwipe)

  const handleCategorySelect = (cat: string) => {
    setReturnStack([])
    if (cat === 'overview') {
      setSelectedCategory('overview')
      setInFilteredView(false)
      setInDevView(false)
      return
    }
    setSelectedCategory(cat)
    setInFilteredView(true)
    setInDevView(false)
    setFilteredTitle(cat === 'all' ? 'All mods' : categoryLabel(cat))
  }

  const showDevPackages = useCallback((dev: string) => {
    navigateTo('Mods by ' + dev, { cat: selectedCategory, devView: true })
  }, [navigateTo, selectedCategory])

  useEffect(() => {
    if (!showDeveloperRef) return
    showDeveloperRef.current = showDevPackages
    return () => { showDeveloperRef.current = null }
  }, [showDeveloperRef, showDevPackages])

  const resetToOverview = useCallback(() => {
    setReturnStack([])
    setSelectedCategory('overview')
    setInFilteredView(false)
    setInDevView(false)
    setFilteredTitle('All mods')
    // Not scrollIntoView: block:'start' aligns under the sticky header and safe-area scrim, overshooting by their height and scrolling the page down.
    window.scrollTo({ top: 0 })
  }, [])

  useEffect(() => {
    if (!resetToOverviewRef) return
    resetToOverviewRef.current = resetToOverview
    return () => { resetToOverviewRef.current = null }
  }, [resetToOverviewRef, resetToOverview])

  const devViewName = inDevView && filteredTitle !== 'All developers'
    ? filteredTitle.replace('Mods by ', '')
    : null

  const devPackages = useMemo(() => {
    if (!inDevView) return []
    const devName = filteredTitle.replace('Mods by ', '')
    return packages.filter(p => p.upstreamAuthor === devName)
      .sort((a, b) => compatibleFirst(a, b) || (b.downloadsAll || 0) - (a.downloadsAll || 0))
  }, [inDevView, filteredTitle, packages])

  const isOverview = selectedCategory === 'overview' && !inFilteredView

  useEffect(() => {
    if (isOverview) return
    setSeenThisVisit(prev => (prev.size === 0 ? prev : new Set()))
  }, [isOverview])

  const activeCat = !isOverview && selectedCategory !== 'all' && selectedCategory !== 'overview' && selectedCategory !== 'starred' ? selectedCategory : undefined

  const starredCandidates = useMemo(
    () => sortedPackages.filter(p => !installedPackages.has(p.name) && p.compatible).map(p => p.name),
    [sortedPackages, installedPackages],
  )
  const starredStaged = starredCandidates.filter(name => installQueue.has(name))
  const starredInIndex = useMemo(
    () => packages.filter(p => starred.has(p.name)).length,
    [packages, starred],
  ) + missingStarred.length
  const showStarredLine = !browseOnly && selectedCategory === 'starred' && starredCandidates.length > 0

  const compact = useMediaQuery(COMPACT_VIEWPORT)
  const effectiveView = compact ? 'grid' : viewMode

  const renderCards = (pkgs: PackageInfo[], opts?: { dlField?: 'downloads30d' | 'downloadsAll'; timeField?: 'released' | 'firstReleased' }) => {
    if (effectiveView === 'list') {
      return (
        <div className="rounded-[var(--radius)] border border-border bg-card">
          {pkgs.map(p => (
            <PackageCard
              key={p.name} pkg={p} status={getCardStatus(p)} context="discover" variant="row"
              onClick={onSelectPackage} onAction={onToggleInstallQueue}
              onAuthorClick={showDevPackages} conflict={getConflict(p)} disabled={disabled}
              starred={starred.has(p.name)}
              hideAction={browseOnly}
              onToggleStar={onToggleStar}
              activeCategory={activeCat}
              dlField={opts?.dlField} timeField={opts?.timeField}
            />
          ))}
        </div>
      )
    }
    return (
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {pkgs.map(p => (
          <PackageCard
            key={p.name} pkg={p} status={getCardStatus(p)} context="discover" variant="grid"
            onClick={onSelectPackage} onAction={onToggleInstallQueue}
            onAuthorClick={showDevPackages} conflict={getConflict(p)} disabled={disabled}
            starred={starred.has(p.name)}
            hideAction={browseOnly}
            onToggleStar={onToggleStar}
            activeCategory={activeCat}
            dlField={opts?.dlField} timeField={opts?.timeField}
          />
        ))}
      </div>
    )
  }

  const renderPackageList = (pkgs: PackageInfo[], opts?: { dlField?: 'downloads30d' | 'downloadsAll'; timeField?: 'released' | 'firstReleased' }) => {
    const usable = pkgs.filter(p => p.compatible)
    const blocked = pkgs.filter(p => !p.compatible)
    return (
      <>
        {usable.length > 0 && renderCards(usable, opts)}
        {blocked.length > 0 && (
          <div className={usable.length > 0 ? 'mt-4' : ''}>
            <button
              onClick={() => setShowBlocked(v => !v)}
              aria-expanded={showBlocked}
              className="flex w-full items-center gap-1.5 py-2 text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer"
            >
              <CaretDown className={cn('h-3.5 w-3.5 transition-transform', !showBlocked && '-rotate-90')} />
              {blocked.length} incompatible
            </button>
            {showBlocked && <div className="mt-1">{renderCards(blocked, opts)}</div>}
          </div>
        )}
      </>
    )
  }

  const renderScrollCards = (pkgs: PackageInfo[], opts?: { dlField?: 'downloads30d' | 'downloadsAll'; timeField?: 'released' | 'firstReleased'; feed?: boolean }) => {
    return pkgs.map(p => (
      <PackageCard
        key={p.name} pkg={p} status={getCardStatus(p)} context="discover" variant="grid" wide
        onClick={onSelectPackage}
        onAction={name => { if (opts?.feed) markSeen(name, true); onToggleInstallQueue(name) }}
        onAuthorClick={showDevPackages} conflict={getConflict(p)} disabled={disabled}
        starred={starred.has(p.name)}
        hideAction={browseOnly}
        onToggleStar={name => { if (opts?.feed && !starred.has(name)) markSeen(name, true); onToggleStar(name) }}
        seen={opts?.feed && (seenPackages.has(p.name) || seenThisVisit.has(p.name))}
        onToggleSeen={opts?.feed ? name => markSeen(name, !(seenPackages.has(name) || seenThisVisit.has(name))) : undefined}
        dlField={opts?.dlField} timeField={opts?.timeField}
      />
    ))
  }

  if (packages.length === 0) {
    if (catalogStatus === 'loading') return null
    return (
      <p className="text-center text-muted-foreground py-8 text-sm">
        Mod catalog unavailable. Connect to the internet to browse and install mods.
      </p>
    )
  }

  return (
    <div>
      <CategoryChips
        categories={categoryNames}
        categoryCounts={categoryCounts}
        totalCount={packages.length}
        selected={isOverview ? 'overview' : selectedCategory}
        onSelect={handleCategorySelect}
        highlightActive={!inDevView}
        starredCount={starredInIndex}
      />



      <div ref={viewRef}>
        {isOverview ? (
          <>
            {followFeedItems.length > 0 && (
              <div className="mb-6">
                <div className="flex flex-wrap items-center justify-between gap-2 mb-2.5 min-h-8">
                  <h3 className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground cursor-pointer hover:text-foreground" onClick={() => navigateTo('All developers', { devView: true })}>From developers you follow</h3>
                  <button onClick={() => navigateTo('All developers', { devView: true })} className="text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer px-2 py-1">All developers</button>
                </div>
                <div className="flex gap-2.5 overflow-x-auto pb-1.5 scroll-smooth [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                  {renderScrollCards(followFeedItems, { timeField: 'firstReleased', feed: true })}
                </div>
              </div>
            )}
            {popular.length > 0 && (
              <ScrollRow title="Popular this month" onViewAll={() => navigateTo('All mods', { cat: 'all', sort: 'popular' })}>
                {renderScrollCards(popular, { dlField: 'downloads30d' })}
              </ScrollRow>
            )}
            {recentlyAdded.length > 0 && (
              <ScrollRow title="Recently added" onViewAll={() => navigateTo('All mods', { cat: 'all', sort: 'added' })}>
                {renderScrollCards(recentlyAdded, { timeField: 'firstReleased' })}
              </ScrollRow>
            )}
            {followFeedItems.length === 0 && developers.length > 0 && (
              <ScrollRow title="Developers" onViewAll={() => navigateTo('All developers', { devView: true })}>
                {(followedDevs.size > 0 ? developersByNewest : developers).slice(0, 8).map(dev => (
                  <DeveloperCard
                    key={dev.name}
                    name={dev.name}
                    packageCount={dev.pkgs.length}
                    totalDownloads={dev.dl}
                    isFollowing={followedDevs.has(dev.name)}
                    onToggleFollow={() => onToggleFollow(dev.name)}
                    onClick={() => showDevPackages(dev.name)}
                  />
                ))}
              </ScrollRow>
            )}
          </>
        ) : inDevView && filteredTitle === 'All developers' ? (
          <div>
            {returnTo && (
              <button
                onClick={navigateBack}
                className="mb-1.5 flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer p-0 m-0 touch:hit-target"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                {returnTo.label}
              </button>
            )}
            <div className="flex flex-wrap items-center justify-between gap-2 mb-2.5 min-h-8">
              <h3 className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground">{filteredTitle}</h3>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {developers.map(dev => (
                <div
                  key={dev.name}
                  className={cn(
                    'rounded-[var(--radius)] border border-border bg-card px-3 py-4 cursor-pointer hover:border-[var(--ring)] hover:shadow-[0_2px_8px_rgba(0,0,0,0.06)] transition-[border-color,box-shadow]',
                    cardFocusRing,
                  )}
                  onClick={() => showDevPackages(dev.name)}
                >
                  <div className="flex items-center justify-between gap-3 mb-2">
                    <button type="button" data-card-trigger aria-label={`Mods by ${dev.name}`} className={cn(cardTrigger, 'text-sm font-semibold truncate')}>{dev.name}</button>
                    <button
                      className={cn(
                        cardActionHeight,
                        'inline-flex items-center justify-center gap-1 text-xs font-medium w-[5rem] rounded-[calc(var(--radius)-4px)] cursor-pointer border transition-all flex-shrink-0',
                        followedDevs.has(dev.name)
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'bg-transparent text-muted-foreground border-border hover:bg-accent hover:text-foreground',
                      )}
                      aria-pressed={followedDevs.has(dev.name)}
                      aria-label={`Follow ${dev.name}`}
                      onClick={(e) => { e.stopPropagation(); onToggleFollow(dev.name) }}
                    >
                      {followedDevs.has(dev.name) ? 'Following' : 'Follow'}
                    </button>
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Package className="h-3 w-3" />{dev.pkgs.length}<span className="sr-only"> mods</span>
                    </span>
                    <span className="flex items-center gap-1">
                      <DownloadSimple className="h-3 w-3" />{formatCount(dev.dl)}<span className="sr-only"> downloads</span>
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div>
            {returnTo && (
              <button
                onClick={navigateBack}
                className="mb-1.5 flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer p-0 m-0 touch:hit-target"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                {returnTo.label}
              </button>
            )}
            <div className={cn('flex flex-wrap items-center justify-between gap-2 min-h-8', showStarredLine ? 'mb-0.5' : 'mb-2.5')}>
              <h3 className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground">{filteredTitle}</h3>
              {devViewName && (
                <button
                  className={cn(
                    cardActionHeight,
                    'inline-flex items-center justify-center gap-1 text-xs font-medium w-[5rem] rounded-[calc(var(--radius)-4px)] cursor-pointer border transition-all flex-shrink-0',
                    followedDevs.has(devViewName)
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'bg-transparent text-muted-foreground border-border hover:bg-accent hover:text-foreground',
                  )}
                  aria-pressed={followedDevs.has(devViewName)}
                  aria-label={`Follow ${devViewName}`}
                  onClick={() => onToggleFollow(devViewName)}
                >
                  {followedDevs.has(devViewName) ? 'Following' : 'Follow'}
                </button>
              )}
              {!inDevView && <SortAndViewControls value={sortBy} onChange={v => setSortBy(v as SortKey)} groups={SORT_GROUPS} viewMode={viewMode} onViewModeChange={onViewModeChange} />}
            </div>
            {showStarredLine && (
              <BulkStageLine
                className="mb-2.5 min-h-7"
                staged={starredStaged.length}
                total={starredCandidates.length}
                idleLabel={`${starredCandidates.length} ready to install`}
                onAddAll={() => onInstallStarred(starredCandidates)}
                onClear={() => onCancelStarred(starredStaged)}
                disabled={disabled}
              />
            )}
            {inDevView ? (
              devPackages.length > 0 ? renderPackageList(devPackages) : (
                <p className="text-center text-muted-foreground py-8 text-sm">No mods found.</p>
              )
            ) : (
              sortedPackages.length > 0 || missingStarred.length > 0 ? (
                <>
                  {sortedPackages.length > 0 && renderPackageList(sortedPackages, statsForSort(sortBy))}
                  {missingStarred.length > 0 && (
                    <div className={cn(
                      sortedPackages.length > 0 && 'mt-3',
                      effectiveView === 'list'
                        ? 'rounded-[var(--radius)] border border-border bg-card'
                        : 'grid grid-cols-1 sm:grid-cols-2 gap-3',
                    )}>
                      {missingStarred.map(name => (
                        <div
                          key={name}
                          className={cn(
                            'flex items-center justify-between gap-3 px-3 py-2.5',
                            effectiveView === 'list'
                              ? 'border-b border-border last:border-b-0'
                              : 'rounded-[var(--radius)] border border-border bg-card',
                          )}
                        >
                          <div className="min-w-0">
                            <p className="text-sm font-semibold truncate">{name}</p>
                            <p className="text-xs text-muted-foreground">No longer in the catalog</p>
                          </div>
                          <StarToggle starred onToggle={onToggleStar} name={name} />
                        </div>
                      ))}
                    </div>
                  )}
                </>
              ) : (
                selectedCategory === 'starred' ? (
                  <div className="text-center py-12">
                    <Star className="h-4 w-4 inline-block text-muted-foreground mb-3" />
                    <p className="text-sm font-medium mb-1">No starred mods</p>
                    <p className="text-xs text-muted-foreground">
                      Star mods to keep them here, ready to install on another reMarkable or after reinstalling Vellum.
                    </p>
                  </div>
                ) : (
                  <p className="text-center text-muted-foreground py-8 text-sm">No mods in this category.</p>
                )
              )
            )}
          </div>
        )}
      </div>
    </div>
  )
}
