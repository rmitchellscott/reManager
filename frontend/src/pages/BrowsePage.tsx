import { ArrowsClockwise, CircleNotch, MagnifyingGlass, X } from '@phosphor-icons/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { PageHeader } from '@/components/PageHeader'
import { PackageDetailModal } from '@/components/PackageDetailModal'
import { DiscoverView } from '@/components/mods/DiscoverView'
import { SearchResultsView } from '@/components/mods/SearchResultsView'
import { PackageDetailPanel } from '@/components/PackageDetailPanel'
import { usePackageFavorites } from '@/hooks/usePackageFavorites'
import { useFollowedDevs } from '@/hooks/useFollowedDevs'
import { PackageInfo } from '@/lib/types'
import { useLoaderVisible } from '@/hooks/useLoaderVisible'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { CloudArrowDown } from '@phosphor-icons/react'

interface BrowsePageProps {
  packages: PackageInfo[]
  catalogStatus: 'loading' | 'ready' | 'error'
  onBack: () => void
  onSettings: () => void
  onRefreshCatalog: () => Promise<void>
  openPackageByNameRef?: React.MutableRefObject<((name: string) => void) | null>
}

export function BrowsePage({ packages, catalogStatus, onBack, onSettings, onRefreshCatalog, openPackageByNameRef }: BrowsePageProps) {
  const [search, setSearch] = useState('')
  const [refreshingCatalog, setRefreshingCatalog] = useState(false)
  const [selectedPackage, setSelectedPackage] = useState<PackageInfo | null>(null)
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() => localStorage.getItem('rm-viewMode') === 'list' ? 'list' : 'grid')
  const { starred, toggleStar } = usePackageFavorites()
  const { follows, followedDevs, toggleFollow, setPackageSeen } = useFollowedDevs()
  const searchRef = useRef<HTMLInputElement>(null)
  const showDeveloperRef = useRef<((dev: string) => void) | null>(null)

  const browsable = useMemo(() => packages.filter(p => !p.hidden), [packages])
  const searching = search.trim().length > 0

  const noPackages = useMemo(() => new Map<string, string>(), [])
  const noNames = useMemo(() => new Set<string>(), [])
  const noTimes = useMemo(() => new Map<string, number>(), [])
  const noop = useCallback(() => {}, [])
  const handleViewModeChange = (mode: 'grid' | 'list') => { setViewMode(mode); localStorage.setItem('rm-viewMode', mode) }

  const openDeveloper = useCallback((dev: string) => {
    setSelectedPackage(null)
    setSearch('')
    showDeveloperRef.current?.(dev)
  }, [])

  const openPackageByName = useCallback((name: string) => {
    const pkg = browsable.find(p => p.name === name)
    if (pkg) {
      setSearch('')
      setSelectedPackage(pkg)
      return
    }
    setSelectedPackage(null)
    setSearch(name)
  }, [browsable])

  useEffect(() => {
    if (!openPackageByNameRef) return
    openPackageByNameRef.current = openPackageByName
    return () => { openPackageByNameRef.current = null }
  }, [openPackageByNameRef, openPackageByName])

  const catalogLoading = useLoaderVisible(catalogStatus === 'loading')

  const handleRefreshCatalog = async () => {
    setRefreshingCatalog(true)
    try { await onRefreshCatalog() }
    catch (err) { console.error('Failed to refresh catalog:', err) }
    setRefreshingCatalog(false)
  }

  return (
    <>
    <PageHeader
      device=""
      narrowTitle="Browse mods"
      subtitle="Browsing without a reMarkable"
      exitLabel="Back to Connect"
      onSettings={onSettings}
      onDisconnect={onBack}
    />
    <div className="space-y-4">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <MagnifyingGlass className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
            placeholder="Search mods…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className={`pl-9 ${search ? 'pr-8' : ''}`}
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              aria-label="Clear search"
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
        <Tooltip><TooltipTrigger asChild><Button variant="outline" size="icon" aria-label="Refresh package list" onClick={handleRefreshCatalog} disabled={refreshingCatalog}>{refreshingCatalog ? <CircleNotch className="h-4 w-4 animate-spin" /> : <ArrowsClockwise className="h-4 w-4" />}</Button></TooltipTrigger><TooltipContent>Refresh package list</TooltipContent></Tooltip>
      </div>

      {searching ? (
        <SearchResultsView
          starred={starred}
          onToggleStar={toggleStar}
          packages={browsable}
          installedPackages={noPackages}
          installTimes={noTimes}
          installQueue={noNames}
          uninstallQueue={noNames}
          impliedPackages={noNames}
          search={search}
          onSelectPackage={setSelectedPackage}
          onToggleInstallQueue={noop}
          onToggleUninstallQueue={noop}
          getConflict={() => null}
          disabled
          viewMode={viewMode}
          onViewModeChange={handleViewModeChange}
        />
      ) : (
        <DiscoverView
          browseOnly
          catalogStatus={catalogStatus}
          packages={browsable}
          installedPackages={noPackages}
          installQueue={noNames}
          impliedPackages={noNames}
          uninstallQueue={noNames}
          onSelectPackage={setSelectedPackage}
          onToggleInstallQueue={noop}
          getConflict={() => null}
          disabled
          followedDevs={followedDevs}
          follows={follows}
          onToggleFollow={toggleFollow}
          onSetPackageSeen={setPackageSeen}
          starred={starred}
          onToggleStar={toggleStar}
          onInstallStarred={noop}
          onCancelStarred={noop}
          showDeveloperRef={showDeveloperRef}
          viewMode={viewMode}
          onViewModeChange={handleViewModeChange}
        />
      )}

      <PackageDetailModal open={selectedPackage !== null} onClose={() => setSelectedPackage(null)}>
          {selectedPackage && (
            <PackageDetailPanel
              browseOnly
              pkg={selectedPackage}
              isInstalled={false}
              installedPackages={noPackages}
              onInstall={noop}
              onUninstall={noop}
              onUpdate={noop}
              disabled
              onSelectPackage={(name) => {
                const next = browsable.find(p => p.name === name)
                if (next) setSelectedPackage(next)
              }}
              allPackages={browsable}
              isStarred={starred.has(selectedPackage.name)}
              onToggleStar={() => toggleStar(selectedPackage.name)}
              onSelectDeveloper={openDeveloper}
            />
        )}
      </PackageDetailModal>

      <Dialog open={catalogLoading} onOpenChange={() => {}} closable={false}>
        <DialogContent>
          <DialogHeader severity="info" icon={CloudArrowDown}>
            <DialogTitle>Loading mods</DialogTitle>
            <DialogDescription>Fetching the mod catalog.</DialogDescription>
          </DialogHeader>
          <Progress
            indeterminate
            className="h-1 bg-muted"
            indicatorClassName="bg-[var(--info)]"
            aria-label="Loading mods"
          />
        </DialogContent>
      </Dialog>
    </div>
    </>
  )
}
