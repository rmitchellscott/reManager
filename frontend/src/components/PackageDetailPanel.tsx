import { ArrowLeft, ArrowRight, ArrowUp, Check, DownloadSimple, Heart, Star, Trash, Warning, X } from '@phosphor-icons/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { PackageReadme } from '@/components/PackageReadme'
import { useBackHandler } from '@/hooks/useBackHandler'
import { COMPACT_VIEWPORT, useMediaQuery } from '@/hooks/useMediaQuery'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { PackageChangelog } from '@/components/mods/PackageChangelog'
import { PackageInfo } from '@/lib/types'
import { cn } from '@/lib/utils'
import { formatCount, formatOsRange } from '@/lib/format'
import { categoryLabel } from '@/components/mods/CategoryChips'
import { SystemService } from '@bindings/reManager'

type BodyTab = 'overview' | 'changelog'

const scrolledEdgeFade = 'linear-gradient(to bottom, transparent, black 1.5rem)'
const scrolledEdgeMask = { maskImage: scrolledEdgeFade, WebkitMaskImage: scrolledEdgeFade }

function constrainedPackageName(entry: string): string {
  const match = entry.match(/^(.+?)(>=|<=|=|>|<)/)
  return match ? match[1] : entry
}

function pkgver(version: string): string {
  return version.replace(/-r\d+$/, '')
}

function projectLabel(url: string): string {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '')
}

interface PackageDetailPanelProps {
  pkg: PackageInfo
  isInstalled: boolean
  detailsUnavailable?: boolean
  installedPackages: Map<string, string>
  installedVersion?: string
  onInstall: () => void
  onUninstall: () => void
  onUpdate: () => void
  queuedAction?: 'install' | 'upgrade' | 'remove'
  disabled: boolean
  onSelectPackage: (name: string) => void
  allPackages?: PackageInfo[]
  conflict?: string | null
  isOsCompatible?: boolean
  viewOnly?: boolean
  browseOnly?: boolean
  showIncompatible?: boolean
  onBack?: () => void
  isStarred?: boolean
  onToggleStar?: () => void
  onSelectDeveloper?: (name: string) => void
  initialTab?: BodyTab
}

const modelLabels: Record<string, string> = {
  rm1: 'reMarkable 1',
  rm2: 'reMarkable 2',
  rmpp: 'Paper Pro',
  rmppmove: 'Paper Pro Move',
  rmppure: 'Paper Pure',
}

export function PackageDetailPanel({
  pkg, isInstalled, detailsUnavailable = false, installedPackages, installedVersion,
  onInstall, onUninstall, onUpdate, queuedAction, disabled,
  onSelectPackage, allPackages = [], conflict,
  isOsCompatible = true, viewOnly = false, browseOnly = false, showIncompatible = false,
  onBack, isStarred, onToggleStar, onSelectDeveloper, initialTab = 'overview',
}: PackageDetailPanelProps) {
  const swipeable = useMediaQuery(COMPACT_VIEWPORT)
  const { ref: swipeRef, swipeHandlers } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: () => onBack?.(),
    restoreAfterDismiss: true,
    enabled: !!onBack && swipeable,
  })
  useBackHandler(() => onBack?.(), !!onBack, swipeHandlers)

  const bodyRef = useRef<HTMLDivElement>(null)
  const mainRef = useRef<HTMLDivElement>(null)
  const [panelRoot, setPanelRoot] = useState<HTMLDivElement | null>(null)
  const attachPanelRoot = useCallback((node: HTMLDivElement | null) => {
    swipeRef.current = node
    setPanelRoot(node)
  }, [swipeRef])
  const [tab, setTab] = useState<BodyTab>(initialTab)
  // Inactive bodies are hidden, not unmounted: both fetch, and unmounting would throw the result away and refetch.
  const [visited, setVisited] = useState<Set<BodyTab>>(() => new Set([initialTab]))
  useEffect(() => {
    setTab(initialTab)
    setVisited(new Set([initialTab]))
  }, [initialTab, pkg.name])

  const [bodyScrolled, setBodyScrolled] = useState(false)
  const [mainScrolled, setMainScrolled] = useState(false)
  const syncScrolledEdges = () => {
    setBodyScrolled((bodyRef.current?.scrollTop ?? 0) > 0)
    setMainScrolled((mainRef.current?.scrollTop ?? 0) > 0)
  }
  useEffect(syncScrolledEdges, [tab, pkg.name])

  const selectTab = (next: BodyTab) => {
    if (next === tab) {
      bodyRef.current?.scrollTo({ top: 0 })
      mainRef.current?.scrollTo({ top: 0 })
      return
    }
    setTab(next)
    setVisited(prev => prev.has(next) ? prev : new Set(prev).add(next))
  }

  const osRange = formatOsRange(pkg)
  const newerVersion = installedVersion && installedVersion !== pkg.version
  const updateAvailable = isInstalled && !!newerVersion && isOsCompatible
  const displayVersion = newerVersion && updateAvailable
    ? { old: installedVersion, new: pkg.version }
    : null
  const mismatched = showIncompatible || !pkg.compatible
  const modelMismatch = mismatched && pkg.incompatibleReason === 'device'
  const osMismatch = mismatched && pkg.incompatibleReason !== 'device'
  const showWarning = showIncompatible && isInstalled

  const facts: [string, React.ReactNode][] = [
    ['Version', displayVersion ? (
      <span className="inline-flex items-center gap-1">
        <span className="text-muted-foreground">{pkgver(displayVersion.old)}</span>
        <ArrowRight className="h-3 w-3" />
        <span className="text-[var(--info-text)]">{pkgver(displayVersion.new)}</span>
      </span>
    ) : pkgver(isInstalled && installedVersion ? installedVersion : pkg.version)],
    ...(pkg.released ? [['Updated', new Date(pkg.released).toLocaleDateString()] as [string, React.ReactNode]] : []),
    ['License', pkg.license || 'Unknown'],
    ['Status', (
      <span className={pkg.status === 'maintained' ? '' : 'text-destructive'}>
        {pkg.status === 'maintained' ? 'Maintained' : pkg.status === 'unmaintained' ? 'Unmaintained' : pkg.status === 'deprecated' ? 'Deprecated' : pkg.status}
      </span>
    )],
    ['Downloads', formatCount(pkg.downloadsAll)],
    ...(osRange ? [['OS', <span className={cn(osMismatch && 'text-destructive')}>{osRange}</span>] as [string, React.ReactNode]] : []),
    ...(pkg.devices && pkg.devices.length > 0
      ? [['Models', <span className={cn(modelMismatch && 'text-destructive')}>{pkg.devices.map(d => modelLabels[d] || d).join(', ')}</span>] as [string, React.ReactNode]]
      : []),
  ]

  const detailsColumn = (
    <>
      <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2">Details</div>
      <dl>
        {facts.map(([label, value]) => (
          <div key={label} className="flex items-start justify-between gap-6 py-1.5">
            <dt className="text-xs text-muted-foreground shrink-0">{label}</dt>
            <dd className="text-xs font-medium text-right">{value}</dd>
          </div>
        ))}
      </dl>

      {pkg.url && (
        <>
          <div className="h-px bg-border my-3" />
          <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2">Project</div>
          <button
            className="text-xs underline underline-offset-2 text-left break-words hover:text-foreground text-muted-foreground"
            onClick={() => SystemService.OpenExternalURL(pkg.url)}
          >
            {projectLabel(pkg.url)}
          </button>
        </>
      )}

      {pkg.depends && pkg.depends.length > 0 && (
        <>
          <div className="h-px bg-border my-3" />
          <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2">Dependencies</div>
          <ul className="space-y-1 text-xs">
            {pkg.depends.map((entry) => {
              const dep = constrainedPackageName(entry)
              const constraint = entry.slice(dep.length)
              const depPkg = allPackages.find(p => p.name === dep)
              const providers = depPkg ? [] : allPackages.filter(p => (p.provides || []).includes(dep))
              if (providers.length > 0) {
                const satisfiedBy = providers.find(p => installedPackages.has(p.name))
                return (
                  <li key={entry} className="flex items-center gap-1.5 flex-wrap">
                    <span>{dep}</span>
                    {constraint && <span className="text-muted-foreground font-mono">{constraint}</span>}
                    <span className="text-muted-foreground">
                      ({satisfiedBy ? satisfiedBy.name : providers.map(p => p.name).join(' or ')})
                    </span>
                    {satisfiedBy && <Check className="h-3 w-3 text-green-600" />}
                  </li>
                )
              }
              const depInstalled = installedPackages.has(dep)
              const depIncompatible = depPkg && !depPkg.compatible
              return (
                <li key={entry} className="flex items-center gap-1.5 flex-wrap">
                  <button
                    onClick={() => onSelectPackage(dep)}
                    className={cn('hover:underline text-left', depIncompatible ? 'text-destructive' : 'text-primary')}
                  >
                    {dep}
                  </button>
                  {constraint && <span className="text-muted-foreground font-mono">{constraint}</span>}
                  {!depInstalled && !isInstalled && !depIncompatible && (
                    <span className="text-muted-foreground">(will be installed)</span>
                  )}
                  {depInstalled && <Check className="h-3 w-3 text-green-600" />}
                </li>
              )
            })}
          </ul>
        </>
      )}

      {pkg.conflicts && pkg.conflicts.length > 0 && (
        <>
          <div className="h-px bg-border my-3" />
          <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2">Conflicts</div>
          <ul className="space-y-1 text-xs">
            {pkg.conflicts.map((entry) => {
              const name = constrainedPackageName(entry)
              const constraint = entry.slice(name.length)
              return (
                <li key={entry} className="flex items-center gap-1.5 flex-wrap">
                  <button onClick={() => onSelectPackage(name)} className="text-primary hover:underline text-left">
                    {name}
                  </button>
                  {constraint && <span className="text-muted-foreground font-mono">{constraint}</span>}
                  {installedPackages.has(name) && (
                    <span className="inline-flex items-center gap-1 text-destructive">
                      <Warning className="h-3 w-3" />
                      installed
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </>
      )}

      {pkg.categories && pkg.categories.length > 0 && (
        <>
          <div className="h-px bg-border my-3" />
          <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mb-2">Categories</div>
          <div className="flex flex-wrap gap-1">
            {pkg.categories.map(cat => <Badge key={cat} variant="outline">{categoryLabel(cat)}</Badge>)}
          </div>
        </>
      )}
    </>
  )

  const removeButton = (className: string) => (
    <Button variant="outline" className={className} onClick={onUninstall} disabled={disabled}>
      <Trash className="h-4 w-4 mr-2" />
      Remove
    </Button>
  )

  const unqueueButtons = {
    install: { label: "Don't install", onClick: onInstall },
    upgrade: { label: "Don't upgrade", onClick: onUpdate },
    remove: { label: 'Keep', onClick: onUninstall },
  }

  const primaryButton = (className: string) => queuedAction ? (
    <Button variant="secondary" className={className} onClick={unqueueButtons[queuedAction].onClick}>
      <X className="h-4 w-4 mr-2" />
      {unqueueButtons[queuedAction].label}
    </Button>
  ) : updateAvailable ? (
    <Button className={className} onClick={onUpdate} disabled={disabled}>
      <ArrowUp className="h-4 w-4 mr-2" />
      Add upgrade
    </Button>
  ) : isInstalled ? removeButton(className) : (
    <Button className={className} onClick={onInstall} disabled={disabled || !isOsCompatible || !!conflict}>
      <DownloadSimple className="h-4 w-4 mr-2" />
      Add
    </Button>
  )

  const donateButton = (className: string) => pkg.donateUrl ? (
    <Button variant="outline" className={className} onClick={() => SystemService.OpenExternalURL(pkg.donateUrl!)}>
      <Heart className="h-4 w-4 mr-2" /> Donate
    </Button>
  ) : null

  const showActions = browseOnly || !viewOnly || (showIncompatible && isInstalled)
  const showPrimary = !browseOnly && !viewOnly
  const showSecondaryRemove = !browseOnly && !queuedAction && (updateAvailable || (isInstalled && showIncompatible && viewOnly))
  const showDesktopActions = showActions || !!onToggleStar || !!pkg.donateUrl
  const narrowRowButtonCount = (pkg.donateUrl ? 1 : 0) + (showPrimary && showSecondaryRemove ? 1 : 0) + (showPrimary || showSecondaryRemove ? 1 : 0)

  const starButton = (className: string) => onToggleStar ? (
    <Button
      variant="outline"
      size="sm"
      className={cn('shrink-0 p-0 hit-target', className)}
      aria-pressed={!!isStarred}
      aria-label={isStarred ? 'Starred' : 'Star'}
      onClick={onToggleStar}
    >
      <Star className="h-4 w-4" weight={isStarred ? 'fill' : 'regular'} />
    </Button>
  ) : null

  const labeledStarButton = (className: string) => onToggleStar ? (
    <Button variant="outline" className={className} aria-pressed={!!isStarred} onClick={onToggleStar}>
      <Star className="h-4 w-4 mr-2" weight={isStarred ? 'fill' : 'regular'} />
      {isStarred ? 'Starred' : 'Star'}
    </Button>
  ) : null

  const incompatibleWarning = (className: string) => showWarning && (
    <div className={cn('flex items-center gap-2 rounded-lg border px-3 py-2', className)} style={{ borderColor: 'var(--warning)' }}>
      <Warning className="h-4 w-4 shrink-0" style={{ color: 'var(--warning)' }} />
      <p className="text-xs text-muted-foreground">
        <span className="text-foreground font-medium">Installed, but no longer compatible.</span>{' '}
        {osRange ? `This release supports OS ${osRange}.` : 'This release does not support your reMarkable.'}
      </p>
    </div>
  )

  const tabSwitcher = (
    <div className="mx-auto flex w-full max-w-[320px] gap-[3px] rounded-md bg-muted p-[3px] md:mx-0">
      {(['overview', 'changelog'] as BodyTab[]).map(value => (
        <button
          key={value}
          aria-selected={tab === value}
          onClick={() => selectTab(value)}
          className={cn(
            'flex-1 rounded-[calc(var(--radius)-4px)] px-3 py-1.5 touch:py-2.5 text-sm font-medium capitalize',
            tab === value ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground',
          )}
        >
          {value}
        </button>
      ))}
    </div>
  )

  return (
    <div ref={attachPanelRoot} {...swipeHandlers} className="relative flex flex-col min-h-0 flex-1">
      <div className="shrink-0 -mx-6 px-6 pb-3 border-b max-sm:-mr-[calc(max(1.5rem,var(--safe-area-right))-var(--safe-area-right))] max-sm:pr-[calc(max(1.5rem,var(--safe-area-right))-var(--safe-area-right))]">
        {onBack && (
          <div className="mb-1">
            <Button variant="ghost" size="sm" onClick={onBack}>
              <ArrowLeft className="h-3.5 w-3.5 mr-1" /> Back
            </Button>
          </div>
        )}

        <div className="pr-10">
          <DialogTitle className="text-xl font-bold sm:inline">{pkg.name}</DialogTitle>
          {pkg.upstreamAuthor && (
            <span className="mt-0.5 block whitespace-nowrap text-sm text-muted-foreground sm:mt-0 sm:ml-2 sm:inline">
              by{' '}
              {onSelectDeveloper ? (
                <button className="underline underline-offset-2 hover:text-foreground" onClick={() => onSelectDeveloper(pkg.upstreamAuthor)}>
                  {pkg.upstreamAuthor}
                </button>
              ) : pkg.upstreamAuthor}
            </span>
          )}
        </div>

        <div className="mt-2 flex items-start justify-between gap-3">
          <DialogDescription>{pkg.description}</DialogDescription>
          {!showActions && <div className="shrink-0 md:hidden">{starButton('h-9 w-9 sm:h-8 sm:w-8')}</div>}
        </div>

        {showActions && (
          <>
            {showPrimary && (
              <div className="mt-3 flex flex-col gap-2 sm:hidden">
                <div className="flex gap-2">
                  {primaryButton('flex-1')}
                  {starButton('h-10 w-10 shrink-0')}
                </div>
                {(showSecondaryRemove || pkg.donateUrl) && (
                  <div className="flex gap-2">
                    {showSecondaryRemove && removeButton('flex-1')}
                    {donateButton('flex-1')}
                  </div>
                )}
              </div>
            )}
            <div className={cn('mt-3 gap-2 md:hidden', showPrimary ? 'hidden sm:flex' : 'flex')}>
              {donateButton('flex-1 min-w-0')}
              {showPrimary && showSecondaryRemove && removeButton('flex-1 min-w-0')}
              {showPrimary ? primaryButton('flex-1 min-w-0') : showSecondaryRemove && removeButton('flex-1 min-w-0')}
              {narrowRowButtonCount <= 1 ? labeledStarButton('flex-1 min-w-0') : starButton('h-10 w-10 shrink-0')}
            </div>
          </>
        )}

        {incompatibleWarning('mt-3 md:hidden')}
      </div>

      <div className="shrink-0 pt-3 md:hidden">{tabSwitcher}</div>

      <div
        ref={bodyRef}
        onScroll={syncScrolledEdges}
        style={bodyScrolled ? scrolledEdgeMask : undefined}
        className="flex-1 min-h-0 overflow-y-auto md:overflow-hidden md:flex md:flex-col -mx-6 px-6 py-4 md:pt-3"
      >
        <div className="flex flex-col gap-6 md:grid md:grid-cols-[minmax(0,1fr)_200px] md:grid-rows-[auto_minmax(0,1fr)] md:gap-x-6 md:gap-y-0 md:flex-1 md:min-h-0">
          {showDesktopActions && (
            <div className="hidden md:flex md:flex-col md:gap-2 md:col-start-2 md:row-start-1 md:border-b md:pb-3">
              {pkg.donateUrl ? (
                <>
                  {showPrimary && primaryButton('w-full')}
                  {showSecondaryRemove && removeButton('w-full')}
                  <div className="flex gap-2">
                    {donateButton('flex-1')}
                    {starButton('h-10 w-10 shrink-0')}
                  </div>
                </>
              ) : showPrimary || showSecondaryRemove ? (
                <>
                  <div className="flex gap-2">
                    {showPrimary ? primaryButton('flex-1') : removeButton('flex-1')}
                    {starButton('h-10 w-10 shrink-0')}
                  </div>
                  {showPrimary && showSecondaryRemove && removeButton('w-full')}
                </>
              ) : labeledStarButton('w-full')}
              {incompatibleWarning('')}
            </div>
          )}

          <div className="min-w-0 md:col-start-1 md:row-span-2 md:row-start-1 md:flex md:flex-col md:min-h-0">
            <div className="hidden md:block shrink-0 pb-4">{tabSwitcher}</div>
            <div
              ref={mainRef}
              onScroll={syncScrolledEdges}
              style={mainScrolled ? scrolledEdgeMask : undefined}
              className="min-w-0 flex-1 md:min-h-0 md:overflow-y-auto md:overscroll-contain"
            >
              {visited.has('overview') && (
                <div className={cn(tab !== 'overview' && 'hidden')}>
                  {pkg.readmeUrl ? (
                    <PackageReadme
                      imageOverlayContainer={panelRoot}
                      url={pkg.readmeUrl}
                    />
                  ) : (
                    <p className="text-sm text-muted-foreground">This mod has no README.</p>
                  )}
                </div>
              )}
              {visited.has('changelog') && (
                <div className={cn(tab !== 'changelog' && 'hidden')}>
                  <PackageChangelog
                    packageName={pkg.name}
                    installedVersion={installedVersion}
                    latestVersion={pkg.version}
                  />
                </div>
              )}
            </div>
          </div>

          <aside className={cn('md:col-start-2 md:row-start-2 md:min-h-0 md:overflow-y-auto md:overscroll-contain border-t pt-4 md:border-t-0', showDesktopActions ? 'md:pt-3' : 'md:pt-0')}>
            {detailsColumn}
          </aside>
        </div>

        {detailsUnavailable && (
          <p className="text-sm text-muted-foreground mt-4 border-t pt-3">
            Connect to the internet to see full details.
          </p>
        )}
      </div>
    </div>
  )
}
