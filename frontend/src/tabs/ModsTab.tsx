import { useState, useEffect, useMemo, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { TabsContent } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Skeleton } from '@/components/ui/skeleton'
import { PackageDetailPanel } from '@/components/PackageDetailPanel'
import { UpgradeChecklist } from '@/components/UpgradeChecklist'
import { StatusBadge } from '@/components/StatusBadge'
import { ReadmeDialog } from '@/components/ReadmeDialog'
import { VellumInstallPrompt } from '@/components/VellumInstallPrompt'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Badge } from '@/components/ui/badge'
import { Loader2, Check, AlertTriangle, AlertCircle, Trash2, Plus, X, Search, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react'
import { useAppContext } from '@/contexts/AppContext'
import { PackageInfo, VirtualChoice, PackageConflict, QueueConflictEntry } from '@/lib/types'

export type SelectPackageForOSFn = (name: string, targetOS: string, isCompatible?: boolean) => Promise<void>

interface ModsTabProps {
  warningsChecked: boolean
  osMismatchDetected: boolean
  storedOsVersion: string
  currentOsVersion: string
  checklistLoading: boolean
  compatibilityStatus: {
    installedPackages: string[]
    compatiblePackages: string[]
    incompatiblePackages: string[]
    currentOsVersion: string
    storedOsVersion: string
    fetchFailed: boolean
    statusMap?: Record<string, string>
  } | null
  bootstrapping: boolean
  bootstrapOutput: string
  bootstrapError: string | null
  vellumBrokenInstall: string[] | null
  vellumCleaning: boolean

  installQueue: Set<string>
  setInstallQueue: (v: Set<string>) => void
  uninstallQueue: Set<string>
  setUninstallQueue: (v: Set<string>) => void
  queueError: string | null
  setQueueError: (v: string | null) => void
  refreshingPackages: boolean
  setRefreshingPackages: (v: boolean) => void

  setPendingProviderChoice: (v: { choices: VirtualChoice[]; index: number; selected: string[]; requested: string[] } | null) => void
  runInstallSimulation: (packages: string[]) => Promise<void>
  setPendingUninstallConfirm: (v: any) => void

  setActiveTab: (v: 'mods' | 'maintenance' | 'utilities') => void
  handleChecklistUpgrade: () => Promise<void>
  selectPackageForOSRef?: React.MutableRefObject<SelectPackageForOSFn | null>
}

function installedOnlyPackage(name: string, version: string): PackageInfo {
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
  }
}

function TruncatedName({ name, className }: { name: string; className?: string }) {
  const ref = useRef<HTMLSpanElement>(null)
  const [truncated, setTruncated] = useState(false)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setTruncated(el.scrollWidth > el.clientWidth)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [name])

  const label = <span ref={ref} className={`block truncate ${className ?? ''}`}>{name}</span>
  if (!truncated) return label

  return (
    <Tooltip>
      <TooltipTrigger asChild>{label}</TooltipTrigger>
      <TooltipContent>{name}</TooltipContent>
    </Tooltip>
  )
}

function ScrollableQueue({ className, children }: { className: string; children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  const [moreBelow, setMoreBelow] = useState(false)

  const measure = () => {
    const el = ref.current
    if (!el) return
    setMoreBelow(el.scrollHeight - el.scrollTop - el.clientHeight > 1)
  }

  useEffect(measure)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return (
    <div className="relative mb-3">
      <div ref={ref} onScroll={measure} className={className}>{children}</div>
      {moreBelow && (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 border-b" />
      )}
    </div>
  )
}

function queueGridColumns(count: number): string {
  if (count <= 1) return 'grid-cols-1'
  if (count === 2) return 'grid-cols-1 sm:grid-cols-2'
  if (count === 3) return 'grid-cols-1 sm:grid-cols-2 md:grid-cols-3'
  return 'grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4'
}

export function ModsTab({
  warningsChecked,
  osMismatchDetected,
  storedOsVersion,
  currentOsVersion,
  checklistLoading,
  compatibilityStatus,
  bootstrapping,
  bootstrapOutput,
  bootstrapError,
  vellumBrokenInstall,
  vellumCleaning,
  installQueue,
  setInstallQueue,
  uninstallQueue,
  setUninstallQueue,
  queueError,
  setQueueError,
  refreshingPackages,
  setRefreshingPackages,
  setPendingProviderChoice,
  runInstallSimulation,
  setPendingUninstallConfirm,
  setActiveTab,
  handleChecklistUpgrade,
  selectPackageForOSRef,
}: ModsTabProps) {
  const ctx = useAppContext()
  const { device, deviceInfo, connectionStatus, installedPackages, packages, setPackages, vellumInstalled, installing, uninstalling, rescanAllPackages } = ctx
  const resolvedTerminalTheme = ctx.resolvedTerminalTheme as 'dark' | 'light'

  const [search, setSearch] = useState('')
  const [categoryFilter, setCategoryFilter] = useState('all')
  const [developerFilter, setDeveloperFilter] = useState('all')
  const [viewMode, setViewMode] = useState<'full' | 'compact'>(() => {
    const saved = localStorage.getItem('packageViewMode')
    return saved === 'compact' ? 'compact' : 'full'
  })

  const [selectedPackage, setSelectedPackage] = useState<PackageInfo | null>(null)
  const [sidebarViewOnly, setSidebarViewOnly] = useState(false)
  const [sidebarIncompatible, setSidebarIncompatible] = useState(false)
  const [sidebarHistory, setSidebarHistory] = useState<PackageInfo[]>([])
  const [readmeDialogOpen, setReadmeDialogOpen] = useState(false)
  const [readmeUrl, setReadmeUrl] = useState<string | null>(null)
  const [readmePackageName, setReadmePackageName] = useState('')

  const [simulatingInstall, setSimulatingInstall] = useState(false)
  const [simulatingUninstall, setSimulatingUninstall] = useState(false)

  const [pendingUninstall, setPendingUninstall] = useState<{
    componentIds: string[]
    componentNames: string[]
    dependents: Array<{ id: string; name: string }>
  } | null>(null)
  const [pendingOrphanRemoval, setPendingOrphanRemoval] = useState<{
    itemToRemove: string
    orphans: Array<{ id: string; name: string }>
  } | null>(null)

  const categories = useMemo(() => {
    const cats = new Set(packages.flatMap(p => p.categories || []))
    return Array.from(cats).sort()
  }, [packages])

  const developers = useMemo(() => {
    const devs = new Set(packages.map(p => p.upstreamAuthor).filter(Boolean))
    return Array.from(devs).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
  }, [packages])

  const filteredPackages = useMemo(() => {
    return [...packages]
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
      .filter(pkg => {
        if (search && !pkg.name.toLowerCase().includes(search.toLowerCase()) &&
            !pkg.description.toLowerCase().includes(search.toLowerCase())) {
          return false
        }
        if (categoryFilter !== 'all' && !(pkg.categories || []).includes(categoryFilter)) {
          return false
        }
        if (developerFilter !== 'all' && pkg.upstreamAuthor !== developerFilter) {
          return false
        }
        return true
      })
  }, [packages, search, categoryFilter, developerFilter])

  const installedFiltered = useMemo(() => {
    const inCatalog = filteredPackages.filter(pkg => installedPackages.has(pkg.name))
    const catalogNames = new Set(packages.map(p => p.name))
    const synthetic = categoryFilter === 'all' && developerFilter === 'all'
      ? Array.from(installedPackages.entries())
          .filter(([name]) => !catalogNames.has(name))
          .filter(([name]) => !search || name.toLowerCase().includes(search.toLowerCase()))
          .map(([name, version]) => installedOnlyPackage(name, version))
      : []
    return [...inCatalog, ...synthetic].sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
  }, [filteredPackages, packages, installedPackages, search, categoryFilter, developerFilter])

  const availableFiltered = useMemo(() => {
    const available = filteredPackages.filter(pkg => !installedPackages.has(pkg.name))
    return available.sort((a, b) => {
      if (a.compatible !== b.compatible) return a.compatible ? -1 : 1
      return 0
    })
  }, [filteredPackages, installedPackages])

  useEffect(() => {
    if (selectedPackage) {
      const updated = packages.find(p => p.name === selectedPackage.name)
      if (updated && updated !== selectedPackage) {
        setSelectedPackage(updated)
      }
    }
  }, [packages])

  useEffect(() => {
    localStorage.setItem('packageViewMode', viewMode)
  }, [viewMode])

  const installedKey = useMemo(
    () => [...installedPackages.entries()].map(([name, v]) => `${name}=${v}`).sort().join(','),
    [installedPackages]
  )

  const [packageConflicts, setPackageConflicts] = useState<Record<string, PackageConflict>>({})

  useEffect(() => {
    if (connectionStatus !== 'connected') {
      setPackageConflicts({})
      return
    }
    let stale = false
    const installed = Object.fromEntries(installedPackages)
    window.go.main.App.GetPackageConflicts(installed, [...installQueue], device, deviceInfo.firmware || '')
      .then(result => { if (!stale) setPackageConflicts(result || {}) })
      .catch(err => console.error('GetPackageConflicts failed:', err))
    return () => { stale = true }
  }, [installQueue, installedKey, device, deviceInfo.firmware, connectionStatus])

  const [queueConflict, setQueueConflict] = useState<{ entries: QueueConflictEntry[]; detail: string } | null>(null)
  const [conflictDetailOpen, setConflictDetailOpen] = useState(false)
  const queueConflicts = queueConflict?.entries ?? []
  const [impliedPackages, setImpliedPackages] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (connectionStatus !== 'connected' || installQueue.size === 0) {
      setQueueConflict(null)
      setConflictDetailOpen(false)
      setImpliedPackages(new Set())
      return
    }
    let stale = false
    const timer = window.setTimeout(() => {
      window.go.main.App.SimulateInstall([...installQueue], device)
        .then(sim => {
          if (stale) return
          const awaitingProviderChoice = (sim.unresolvedVirtuals?.length ?? 0) > 0
          setQueueConflict(sim.error && !awaitingProviderChoice
            ? { entries: sim.conflicts || [], detail: sim.error }
            : null)
          setConflictDetailOpen(false)
          setImpliedPackages(sim.error ? new Set() : new Set(
            (sim.packages || []).filter(name => !installQueue.has(name) && !installedPackages.has(name))
          ))
          if (!sim.error) setQueueError(null)
        })
        .catch(err => console.error('queue conflict check failed:', err))
    }, 400)
    return () => { stale = true; window.clearTimeout(timer) }
  }, [installQueue, installedKey, device, connectionStatus])

  const availableMarks = useMemo(
    () => availableFiltered.map(pkg =>
      installQueue.has(pkg.name) ? 'queued' : impliedPackages.has(pkg.name) ? 'implied' : null
    ),
    [availableFiltered, installQueue, impliedPackages]
  )

  const queueBarRef = useRef<HTMLDivElement>(null)
  const queueVisible = installQueue.size > 0 || uninstallQueue.size > 0

  useEffect(() => {
    const root = document.documentElement
    const el = queueBarRef.current
    if (!el) {
      root.style.setProperty('--queue-bar-height', '0px')
      return
    }
    const apply = () => root.style.setProperty('--queue-bar-height', `${el.offsetHeight}px`)
    apply()
    const observer = new ResizeObserver(apply)
    observer.observe(el)
    return () => {
      observer.disconnect()
      root.style.setProperty('--queue-bar-height', '0px')
    }
  }, [queueVisible])

  const [queueOpen, setQueueOpen] = useState('')

  const conflictPairs = useMemo(() => {
    const pairs: { a: string; b: string }[] = []
    const seen = new Set<string>()
    for (const entry of queueConflicts) {
      const key = [entry.package, entry.blocks].sort().join('\u0000')
      if (seen.has(key)) continue
      seen.add(key)
      pairs.push({ a: entry.package, b: entry.blocks })
    }
    return pairs
  }, [queueConflicts])

  const conflictByQueueEntry = useMemo(() => {
    const map: Record<string, QueueConflictEntry> = {}
    for (const entry of queueConflicts) map[entry.queueEntry] = entry
    return map
  }, [queueConflicts])

  const sortedInstallQueue = useMemo(() => {
    return Array.from(installQueue).sort((a, b) => {
      const aConflicts = !!conflictByQueueEntry[a]
      const bConflicts = !!conflictByQueueEntry[b]
      if (aConflicts !== bConflicts) return aConflicts ? -1 : 1
      return a.localeCompare(b, undefined, { sensitivity: 'base' })
    })
  }, [installQueue, conflictByQueueEntry])

  const sortedUninstallQueue = useMemo(() => {
    return Array.from(uninstallQueue).sort((a, b) =>
      a.localeCompare(b, undefined, { sensitivity: 'base' }))
  }, [uninstallQueue])

  const getConflict = (pkg: PackageInfo): string | null => {
    const conflict = packageConflicts[pkg.name]
    if (!conflict) return null
    return conflict.queued
      ? `Conflicts with queued: ${conflict.conflictsWith}`
      : `Conflicts with installed: ${conflict.conflictsWith}`
  }

  const addToQueue = (name: string) => {
    if (uninstallQueue.has(name)) {
      setQueueError(`Cannot add — ${name} is queued for removal`)
      setTimeout(() => setQueueError(null), 4000)
      return
    }

    const newQueue = new Set(installQueue)
    newQueue.add(name)
    setInstallQueue(newQueue)
  }

  const removeFromQueue = (name: string) => {
    const newQueue = new Set(installQueue)
    newQueue.delete(name)
    setInstallQueue(newQueue)
  }

  const confirmOrphanRemoval = (removeOrphans: boolean) => {
    if (!pendingOrphanRemoval) return

    const newQueue = new Set(installQueue)
    newQueue.delete(pendingOrphanRemoval.itemToRemove)

    if (removeOrphans) {
      for (const orphan of pendingOrphanRemoval.orphans) {
        newQueue.delete(orphan.id)
      }
    }

    setInstallQueue(newQueue)
    setPendingOrphanRemoval(null)
  }

  const clearQueue = () => {
    setInstallQueue(new Set())
  }

  const addToUninstallQueue = (name: string) => {
    if (installQueue.has(name)) {
      setQueueError(`Cannot remove — ${name} is queued for installation`)
      setTimeout(() => setQueueError(null), 4000)
      return
    }

    const newQueue = new Set(uninstallQueue)
    newQueue.add(name)
    setUninstallQueue(newQueue)
  }

  const confirmUninstallWithDependents = (includeAll: boolean) => {
    if (!pendingUninstall) return

    const newQueue = new Set(uninstallQueue)
    for (const id of pendingUninstall.componentIds) {
      newQueue.add(id)
    }
    if (includeAll) {
      for (const dep of pendingUninstall.dependents) {
        newQueue.add(dep.id)
      }
    }
    setUninstallQueue(newQueue)
    setPendingUninstall(null)
  }

  const removeFromUninstallQueue = (id: string) => {
    const newQueue = new Set(uninstallQueue)
    newQueue.delete(id)
    setUninstallQueue(newQueue)
  }

  const clearUninstallQueue = () => {
    setUninstallQueue(new Set())
  }

  const handleViewReadme = (url: string) => {
    const isMarkdown = url.endsWith('.md') || new URL(url).hostname === 'raw.githubusercontent.com'
    if (isMarkdown) {
      setReadmeUrl(url)
      setReadmePackageName(selectedPackage?.name || '')
      setReadmeDialogOpen(true)
    } else {
      window.runtime.BrowserOpenURL(url)
    }
  }

  const handleRefreshPackages = async () => {
    setRefreshingPackages(true)
    try {
      await window.go.main.App.RefreshMetadata()
      if (device) {
        const info = await window.go.main.App.GetDeviceInfo()
        const arch = await window.go.main.App.GetDeviceArchitecture(device)
        const filteredPkgs = await window.go.main.App.GetPackages(device, info.firmware || '', arch)
        setPackages(filteredPkgs || [])
      }
      await rescanAllPackages()
    } catch (err) {
      console.error('Failed to refresh packages:', err)
    }
    setRefreshingPackages(false)
  }

  useEffect(() => {
    if (selectPackageForOSRef) {
      selectPackageForOSRef.current = handleSelectPackageForOSInternal
    }
    return () => { if (selectPackageForOSRef) selectPackageForOSRef.current = null }
  })

  const handleSelectPackageForOSInternal = async (name: string, targetOS: string, isCompatible: boolean = true) => {
    try {
      const arch = await window.go.main.App.GetDeviceArchitecture(device)
      let pkg = await window.go.main.App.GetPackageForOS(name, targetOS, device, arch)
      if (!pkg) {
        pkg = packages.find(p => p.name === name) || null
      }
      if (pkg) {
        setSidebarViewOnly(true)
        setSidebarIncompatible(!isCompatible)
        setSelectedPackage(pkg)
      }
    } catch (err) {
      console.error('Failed to get package for OS:', err)
    }
  }

  return (
    <>
      <TabsContent value="mods">
        {vellumInstalled === null || (vellumInstalled && !warningsChecked) ? (
          <div className="space-y-6">
            <div className="flex flex-wrap gap-2">
              <Skeleton className="h-9 flex-1 min-w-[200px]" />
              <Skeleton className="h-9 w-9" />
              <Skeleton className="h-9 w-32" />
              <Skeleton className="h-9 w-32" />
            </div>
            <Card>
              <div className="px-6 py-4">
                <Skeleton className="h-4 w-32" />
              </div>
              <div className="divide-y px-6 pb-4">
                {Array.from({ length: 8 }).map((_, i) => (
                  <div key={i} className="py-3 flex items-center gap-4">
                    <div className="flex-1 min-w-0">
                      {viewMode === 'compact' ? (
                        <div className="flex items-center gap-2">
                          <Skeleton className="h-4 w-[160px] shrink-0" />
                          <Skeleton className="h-4 flex-1" />
                        </div>
                      ) : (
                        <div className="space-y-2">
                          <div className="flex items-center gap-2">
                            <Skeleton className="h-4 w-32" />
                            <Skeleton className="h-5 w-16 rounded-full" />
                          </div>
                          <Skeleton className="h-4 w-3/4" />
                        </div>
                      )}
                    </div>
                    <Skeleton className="h-8 w-20 shrink-0" />
                  </div>
                ))}
              </div>
            </Card>
          </div>
        ) : vellumBrokenInstall !== null ? (
          <Card>
            <CardHeader>
              <div className="flex items-center gap-2">
                <AlertCircle className="h-5 w-5 text-destructive" />
                <CardTitle>Vellum Installation Incomplete</CardTitle>
              </div>
              <CardDescription>
                Your Vellum installation is missing core packages and needs to be reinstalled.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Missing packages: {vellumBrokenInstall.join(', ')}
              </p>
              <div className="flex justify-end pt-2">
                <Button
                  onClick={() => window.go.main.App.CleanupBrokenVellum()}
                  disabled={vellumCleaning}
                >
                  {vellumCleaning ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Cleaning up...
                    </>
                  ) : (
                    'Clean up and reinstall'
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : vellumInstalled === false ? (
          <VellumInstallPrompt
            bootstrapping={bootstrapping}
            bootstrapOutput={bootstrapOutput}
            bootstrapError={bootstrapError}
            onInstall={() => window.go.main.App.BootstrapVellum()}
            terminalTheme={resolvedTerminalTheme}
          />
        ) : osMismatchDetected ? (
          <div>
            {compatibilityStatus ? (
              <UpgradeChecklist
                storedOsVersion={compatibilityStatus.storedOsVersion || storedOsVersion}
                currentOsVersion={compatibilityStatus.currentOsVersion || currentOsVersion}
                compatiblePackages={compatibilityStatus.compatiblePackages || []}
                incompatiblePackages={compatibilityStatus.incompatiblePackages || []}
                loading={checklistLoading}
                fetchFailed={compatibilityStatus.fetchFailed || false}
                uninstallQueue={uninstallQueue}
                onAddToUninstallQueue={addToUninstallQueue}
                onRemoveFromUninstallQueue={removeFromUninstallQueue}
                onRunUpgrade={handleChecklistUpgrade}
                onGoToUtilities={() => setActiveTab('utilities')}
                onSelectPackage={handleSelectPackageForOSInternal}
                statusMap={compatibilityStatus.statusMap}
              />
            ) : (
              <Card>
                <CardHeader>
                  <div className="flex items-center gap-2">
                    <AlertTriangle className="h-5 w-5 text-yellow-500" />
                    <CardTitle>OS Change Detected</CardTitle>
                  </div>
                  <CardDescription>
                    Your reMarkable OS has changed from {storedOsVersion} to {currentOsVersion}.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Checking package compatibility...
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        ) : (
        <div className="space-y-6">
            {/* Filters */}
            <div className="flex flex-wrap gap-2">
              <div className="flex gap-2 w-full md:contents">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search mods..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className={`pl-9 ${search ? 'pr-8' : ''}`}
                  />
                  {search && (
                    <button
                      onClick={() => setSearch('')}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      <X className="h-4 w-4" />
                    </button>
                  )}
                </div>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      size="icon"
                      className="md:order-last"
                      onClick={handleRefreshPackages}
                      disabled={refreshingPackages}
                    >
                      {refreshingPackages ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <RefreshCw className="h-4 w-4" />
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Refresh package list</TooltipContent>
                </Tooltip>
              </div>
              <div className="flex gap-2 w-full md:contents">
                <Select value={categoryFilter} onValueChange={setCategoryFilter}>
                  <SelectTrigger className="flex-1 md:flex-none md:w-[160px]">
                    <SelectValue placeholder="Category" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Categories</SelectItem>
                    {categories.map((cat) => (
                      <SelectItem key={cat} value={cat}>{cat}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={developerFilter} onValueChange={setDeveloperFilter}>
                  <SelectTrigger className="flex-1 md:flex-none md:w-[160px]">
                    <SelectValue placeholder="Developer" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Developers</SelectItem>
                    {developers.map((dev) => (
                      <SelectItem key={dev} value={dev}>{dev}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={viewMode} onValueChange={(v) => setViewMode(v as 'full' | 'compact')}>
                  <SelectTrigger className="flex-1 md:flex-none md:w-[120px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="full">Full</SelectItem>
                    <SelectItem value="compact">Compact</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Installed Section */}
            {installedFiltered.length > 0 && (
              <Card>
                <Accordion type="single" collapsible defaultValue="installed">
                  <AccordionItem value="installed" className="border-none">
                    <AccordionTrigger className="px-6 py-4 text-sm font-semibold uppercase tracking-wide hover:no-underline">
                      Installed ({installedFiltered.length})
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="divide-y px-6 pb-4">
                        {installedFiltered.map((pkg, index) => {
                          const isQueued = uninstallQueue.has(pkg.name)
                          const prevQueued = index > 0 && uninstallQueue.has(installedFiltered[index - 1].name)
                          const nextQueued = index < installedFiltered.length - 1 && uninstallQueue.has(installedFiltered[index + 1].name)
                          const row = (
                            <div key={pkg.name} className={`py-3 flex items-center gap-4 ${isQueued ? `border-l-4 border-destructive pl-3 ${!prevQueued ? 'border-t' : ''} ${!nextQueued ? 'border-b' : ''}` : !pkg.compatible ? 'border-l-4 border-yellow-500 pl-3 border-t border-t-yellow-500' : index % 2 === 1 ? 'bg-muted/50 hover:bg-muted' : 'hover:bg-muted/70'}`}>
                            <div
                              className="flex-1 min-w-0 cursor-pointer"
                              onClick={() => { setSidebarViewOnly(false); setSidebarIncompatible(!pkg.compatible); setSelectedPackage(pkg) }}
                            >
                              {viewMode === 'compact' ? (
                                <div className="flex items-center gap-2">
                                  <div className="w-[120px] md:w-[160px] lg:w-[200px] xl:w-[240px] shrink-0">
                                    <span className="font-medium truncate block">{pkg.name}</span>
                                    <StatusBadge status={pkg.status} />
                                  </div>
                                  <span className="text-sm text-muted-foreground truncate">{pkg.description}</span>
                                </div>
                              ) : (
                                <>
                                  <div className="flex items-center gap-2">
                                    <span className="font-medium">{pkg.name}</span>
                                    {(pkg.categories || []).map(cat => <Badge key={cat} variant="outline">{cat}</Badge>)}
                                    <StatusBadge status={pkg.status} />
                                  </div>
                                  <p className="text-sm text-muted-foreground mt-1">{pkg.description}</p>
                                  {pkg.upstreamAuthor && (
                                    <span className="text-sm text-muted-foreground">
                                      by {pkg.upstreamAuthor}
                                    </span>
                                  )}
                                </>
                              )}
                            </div>
                            {isQueued ? (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => removeFromUninstallQueue(pkg.name)}
                              >
                                <Check className="h-4 w-4 mr-1" />
                                Queued
                              </Button>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => addToUninstallQueue(pkg.name)}
                                disabled={installing || uninstalling || connectionStatus !== 'connected'}
                              >
                                <Trash2 className="h-4 w-4 mr-1" />
                                Remove
                              </Button>
                            )}
                            </div>
                          )
                          return !pkg.compatible ? (
                            <Tooltip key={pkg.name}>
                              <TooltipTrigger asChild>{row}</TooltipTrigger>
                              <TooltipContent>{pkg.incompatibleReason === 'device' ? 'Does not support your reMarkable model' : 'Does not support your current OS'}</TooltipContent>
                            </Tooltip>
                          ) : row
                        })}
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                </Accordion>
              </Card>
            )}

            {/* Available Section */}
            {availableFiltered.length > 0 && (
              <Card>
                <Accordion type="single" collapsible defaultValue="available">
                  <AccordionItem value="available" className="border-none">
                    <AccordionTrigger className="px-6 py-4 text-sm font-semibold uppercase tracking-wide hover:no-underline">
                      Available ({availableFiltered.filter(p => p.compatible).length}{availableFiltered.some(p => !p.compatible) && ` · ${availableFiltered.filter(p => !p.compatible).length} incompatible`})
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="divide-y px-6 pb-4">
                        {availableFiltered.map((pkg, index) => {
                          const mark = availableMarks[index]
                          const isQueued = mark === 'queued'
                          const startsGroup = availableMarks[index - 1] !== mark
                          const endsGroup = availableMarks[index + 1] !== mark
                          const conflict = getConflict(pkg)
                          return (
                            <div key={pkg.name} className={`py-3 flex items-center gap-4 ${!pkg.compatible ? 'opacity-50' : ''} ${mark ? `border-l-4 ${isQueued ? 'border-primary' : 'border-primary/40'} pl-3 ${startsGroup ? 'border-t' : ''} ${endsGroup ? 'border-b' : ''}` : index % 2 === 1 ? 'bg-muted/50 hover:bg-muted' : 'hover:bg-muted/70'}`}>
                            <div
                              className="flex-1 min-w-0 cursor-pointer"
                              onClick={() => { setSidebarViewOnly(false); setSidebarIncompatible(!pkg.compatible); setSelectedPackage(pkg) }}
                            >
                              {viewMode === 'compact' ? (
                                <div className="flex items-center gap-2">
                                  <div className="w-[120px] md:w-[160px] lg:w-[200px] xl:w-[240px] shrink-0">
                                    <span className="font-medium truncate block">{pkg.name}</span>
                                    <StatusBadge status={pkg.status} />
                                  </div>
                                  <span className="text-sm text-muted-foreground truncate">{pkg.description}</span>
                                </div>
                              ) : (
                                <>
                                  <div className="flex items-center gap-2">
                                    <span className="font-medium">{pkg.name}</span>
                                    {(pkg.categories || []).map(cat => <Badge key={cat} variant="outline">{cat}</Badge>)}
                                    <StatusBadge status={pkg.status} />
                                  </div>
                                  <p className="text-sm text-muted-foreground mt-1">{pkg.description}</p>
                                  {pkg.upstreamAuthor && (
                                    <span className="text-sm text-muted-foreground">
                                      by {pkg.upstreamAuthor}
                                    </span>
                                  )}
                                </>
                              )}
                            </div>
                            {!pkg.compatible ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span>
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      disabled
                                    >
                                      Incompatible
                                    </Button>
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent>{pkg.incompatibleReason === 'device' ? 'Does not support your reMarkable model' : 'Does not support your current OS'}</TooltipContent>
                              </Tooltip>
                            ) : isQueued ? (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => removeFromQueue(pkg.name)}
                              >
                                <Check className="h-4 w-4 mr-1" />
                                Queued
                              </Button>
                            ) : conflict ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span>
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      disabled
                                    >
                                      <Plus className="h-4 w-4 mr-1" />
                                      Add
                                    </Button>
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent>{conflict}</TooltipContent>
                              </Tooltip>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => addToQueue(pkg.name)}
                                disabled={installing || uninstalling || connectionStatus !== 'connected'}
                              >
                                <Plus className="h-4 w-4 mr-1" />
                                Add
                              </Button>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </AccordionContent>
                  </AccordionItem>
                </Accordion>
              </Card>
            )}

            {filteredPackages.length === 0 && (
              <p className="text-center text-muted-foreground py-8">
                {packages.length === 0
                  ? 'Mod catalog unavailable. An internet connection is required to browse and install mods.'
                  : 'No mods match your filters'}
              </p>
            )}

          </div>
        )}
      </TabsContent>

      {/* Queue Section - Fixed at bottom, visible on all tabs */}
      {(installQueue.size > 0 || uninstallQueue.size > 0) && (
        <div ref={queueBarRef} className="fixed bottom-0 left-0 right-0 py-4 px-6 bg-background border-t shadow-[0_-4px_6px_-1px_rgba(0,0,0,0.1)] z-50 space-y-4 max-h-[85vh] overflow-y-auto overscroll-y-contain">
          {/* Install Queue */}
          {installQueue.size > 0 && (
            <div>
              <Accordion type="single" collapsible value={queueOpen} onValueChange={setQueueOpen}>
                <AccordionItem value="install-queue" className="border-none">
                  <AccordionTrigger expandsUpward className="pt-1 pb-4 text-sm font-medium hover:no-underline">
                    Install Queue ({installQueue.size})
                  </AccordionTrigger>
                  <AccordionContent>
                    <ScrollableQueue className={`max-h-[35vh] overflow-y-auto overscroll-y-contain grid ${queueGridColumns(installQueue.size)} gap-x-10 gap-y-1`}>
                      {sortedInstallQueue.map((name) => {
                        const conflict = conflictByQueueEntry[name]
                        return (
                          <div
                            key={name}
                            className="flex items-center justify-between text-sm py-1 px-2 rounded-md hover:bg-muted transition-colors"
                          >
                            <div className="min-w-0">
                              <TruncatedName
                                name={name}
                                className={conflict ? 'text-destructive font-medium' : ''}
                              />
                              {conflict && (
                                <p className="text-xs text-destructive">
                                  {conflict.package === name
                                    ? `Can't be installed with ${conflict.blocks}`
                                    : `Needs ${conflict.package}, which can't be installed with ${conflict.blocks}`}
                                </p>
                              )}
                            </div>
                            <button
                              onClick={() => removeFromQueue(name)}
                              className="text-muted-foreground hover:text-foreground shrink-0"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          </div>
                        )
                      })}
                    </ScrollableQueue>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
              {queueError && (
                <div className="mb-2 rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                  {queueError}
                </div>
              )}
              {queueConflict && (queueOpen !== 'install-queue' || conflictPairs.length === 0) && (
                <div className="mb-2 rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                  <button
                    type="button"
                    onClick={() => setConflictDetailOpen(open => !open)}
                    className="flex w-full items-start gap-2 text-left"
                    aria-expanded={conflictDetailOpen}
                  >
                    {conflictDetailOpen
                      ? <ChevronDown className="h-4 w-4 shrink-0 mt-0.5" />
                      : <ChevronRight className="h-4 w-4 shrink-0 mt-0.5" />}
                    <span>
                      {conflictPairs.length === 1 ? (
                        <><strong className="font-semibold">{conflictPairs[0].a}</strong> and{' '}
                        <strong className="font-semibold">{conflictPairs[0].b}</strong> can't be installed
                        together. Expand the queue to fix.</>
                      ) : conflictPairs.length > 1 ? (
                        <>Some queued packages can't be installed together. Expand the queue to fix.</>
                      ) : (
                        <>The install queue couldn't be resolved. Show details.</>
                      )}
                    </span>
                  </button>
                  {conflictDetailOpen && (
                    <pre className="mt-2 max-h-48 overflow-auto rounded bg-destructive/10 p-2 font-mono text-xs whitespace-pre-wrap">
                      {queueConflict.detail}
                    </pre>
                  )}
                </div>
              )}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={clearQueue}
                  disabled={installing || uninstalling}
                >
                  Clear Install Queue
                </Button>
                <Button
                  className="flex-1"
                  onClick={async () => {
                    setSimulatingInstall(true)
                    try {
                      const requested = [...installQueue]
                      let choices: VirtualChoice[] = []
                      try {
                        choices = await window.go.main.App.GetInstallChoices(requested, device)
                      } catch (err) {
                        console.error('GetInstallChoices failed:', err)
                      }
                      if (choices.length > 0) {
                        setPendingProviderChoice({ choices, index: 0, selected: [], requested })
                      } else {
                        await runInstallSimulation(requested)
                      }
                    } finally {
                      setSimulatingInstall(false)
                    }
                  }}
                  disabled={installing || uninstalling || simulatingInstall || !!queueConflict || connectionStatus !== 'connected'}
                >
                  {installing ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Installing...
                    </>
                  ) : simulatingInstall ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Checking...
                    </>
                  ) : (
                    'Install Selected'
                  )}
                </Button>
              </div>
            </div>
          )}

          {/* Uninstall Queue */}
          {uninstallQueue.size > 0 && (
            <div>
              <Accordion type="single" collapsible>
                <AccordionItem value="uninstall-queue" className="border-none">
                  <AccordionTrigger expandsUpward className="pt-1 pb-4 text-sm font-medium text-destructive hover:no-underline">
                    Uninstall Queue ({uninstallQueue.size})
                  </AccordionTrigger>
                  <AccordionContent>
                    <ScrollableQueue className={`max-h-[35vh] overflow-y-auto overscroll-y-contain grid ${queueGridColumns(uninstallQueue.size)} gap-x-10 gap-y-1`}>
                      {sortedUninstallQueue.map((name) => {
                        return (
                          <div
                            key={name}
                            className="flex items-center justify-between text-sm py-1 px-2 rounded-md hover:bg-muted transition-colors"
                          >
                            <TruncatedName name={name} />
                            <button
                              onClick={() => removeFromUninstallQueue(name)}
                              className="text-muted-foreground hover:text-foreground shrink-0"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          </div>
                        )
                      })}
                    </ScrollableQueue>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={clearUninstallQueue}
                  disabled={installing || uninstalling}
                >
                  Clear Uninstall Queue
                </Button>
                <Button
                  className="flex-1"
                  onClick={async () => {
                    setSimulatingUninstall(true)
                    try {
                      const selected = [...uninstallQueue]
                      const sim = await window.go.main.App.SimulateUninstall(selected)
                      if (sim.worldDeps && sim.worldDeps.length > 0) {
                        setPendingUninstallConfirm({
                          selected,
                          packages: sim.allAffected || selected,
                          blocked: null,
                          useRecursive: false,
                          worldDeps: sim.worldDeps
                        })
                      } else if (sim.blocked && Object.keys(sim.blocked).length > 0) {
                        setPendingUninstallConfirm({
                          selected,
                          packages: sim.recursivePackages || sim.packages || selected,
                          blocked: sim.blocked,
                          useRecursive: true
                        })
                      } else {
                        setPendingUninstallConfirm({
                          selected,
                          packages: (sim.packages && sim.packages.length > 0) ? sim.packages : selected,
                          blocked: null,
                          useRecursive: false
                        })
                      }
                    } catch (err) {
                      console.error('SimulateUninstall failed:', err)
                      const selected = [...uninstallQueue]
                      setPendingUninstallConfirm({
                        selected,
                        packages: selected,
                        blocked: null,
                        useRecursive: false
                      })
                    } finally {
                      setSimulatingUninstall(false)
                    }
                  }}
                  disabled={installing || uninstalling || simulatingUninstall || connectionStatus !== 'connected'}
                >
                  {uninstalling ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Removing...
                    </>
                  ) : simulatingUninstall ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Checking...
                    </>
                  ) : (
                    'Uninstall Selected'
                  )}
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Uninstall Dependents Confirmation Dialog */}
      <Dialog open={pendingUninstall !== null} onOpenChange={(open) => !open && setPendingUninstall(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-yellow-500" />
              Components Have Dependents
            </DialogTitle>
            <DialogDescription>
              <div className="space-y-4 pt-4">
                <p>
                  The following installed components depend on{' '}
                  <strong>{pendingUninstall?.componentNames.join(', ')}</strong>:
                </p>
                <ul className="list-disc list-inside space-y-1 text-sm">
                  {pendingUninstall?.dependents.map((dep) => (
                    <li key={dep.id}>{dep.name}</li>
                  ))}
                </ul>
                <p>These components may not work correctly after removal.</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setPendingUninstall(null)}>
              Cancel
            </Button>
            <Button variant="outline" onClick={() => confirmUninstallWithDependents(false)}>
              Remove Only {pendingUninstall?.componentNames[0]}
            </Button>
            <Button variant="destructive" onClick={() => confirmUninstallWithDependents(true)}>
              Remove All ({(pendingUninstall?.componentIds.length || 0) + (pendingUninstall?.dependents.length || 0)})
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Orphan Dependency Removal Dialog */}
      <Dialog open={pendingOrphanRemoval !== null} onOpenChange={(open) => !open && setPendingOrphanRemoval(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove Dependencies?</DialogTitle>
            <DialogDescription>
              <div className="space-y-4 pt-4">
                <p>
                  The following dependencies are no longer needed by any queued items:
                </p>
                <ul className="list-disc list-inside space-y-1 text-sm">
                  {pendingOrphanRemoval?.orphans.map((dep) => (
                    <li key={dep.id}>{dep.name}</li>
                  ))}
                </ul>
                <p>Would you like to remove them from the queue as well?</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => confirmOrphanRemoval(false)}>
              Keep in Queue
            </Button>
            <Button onClick={() => confirmOrphanRemoval(true)}>
              Remove All
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Package Detail Side Panel */}
      <Sheet open={selectedPackage !== null} onOpenChange={(open) => { if (!open) { setSelectedPackage(null); setSidebarViewOnly(false); setSidebarIncompatible(false); setSidebarHistory([]) } }}>
        <SheetContent side="right" className="w-[400px] sm:w-[450px] sm:max-w-none flex flex-col">
          {selectedPackage && (
            <PackageDetailPanel
              pkg={selectedPackage}
              isInstalled={installedPackages.has(selectedPackage.name)}
              detailsUnavailable={!packages.some(p => p.name === selectedPackage.name)}
              installedPackages={installedPackages}
              installedVersion={installedPackages.get(selectedPackage.name)}
              onInstall={() => {
                addToQueue(selectedPackage.name)
                setSelectedPackage(null)
              }}
              onUninstall={() => {
                addToUninstallQueue(selectedPackage.name)
                setSelectedPackage(null)
              }}
              isQueued={installQueue.has(selectedPackage.name) || uninstallQueue.has(selectedPackage.name)}
              queueType={installQueue.has(selectedPackage.name) ? 'install' : uninstallQueue.has(selectedPackage.name) ? 'uninstall' : null}
              disabled={installing || uninstalling || connectionStatus !== 'connected'}
              onSelectPackage={(name) => {
                const pkg = packages.find(p => p.name === name)
                if (pkg) {
                  if (selectedPackage) setSidebarHistory(h => [...h, selectedPackage])
                  setSidebarViewOnly(false)
                  setSidebarIncompatible(false)
                  setSelectedPackage(pkg)
                }
              }}
              allPackages={packages}
              firmware={deviceInfo.firmware || ''}
              conflict={getConflict(selectedPackage)}
              isOsCompatible={selectedPackage.compatible}
              viewOnly={sidebarViewOnly}
              showIncompatible={sidebarIncompatible}
              onViewReadme={handleViewReadme}
              onBack={sidebarHistory.length > 0 ? () => {
                const prev = sidebarHistory[sidebarHistory.length - 1]
                setSidebarHistory(h => h.slice(0, -1))
                setSelectedPackage(prev)
                setSidebarViewOnly(false)
                setSidebarIncompatible(false)
              } : undefined}
            />
          )}
        </SheetContent>
      </Sheet>

      <ReadmeDialog
        open={readmeDialogOpen}
        onOpenChange={setReadmeDialogOpen}
        url={readmeUrl}
        packageName={readmePackageName}
      />
    </>
  )
}
