import { ArrowsClockwise, CheckCircle, CircleNotch, MagnifyingGlass, StopCircle, WarningCircle, X } from '@phosphor-icons/react'
import { useState, useEffect, useLayoutEffect, useMemo, useRef, useCallback, type Dispatch, type SetStateAction } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { PackageDetailModal } from '@/components/PackageDetailModal'
import { Badge } from '@/components/ui/badge'
import { PackageDetailPanel } from '@/components/PackageDetailPanel'
import { VellumInstallPrompt } from '@/components/VellumInstallPrompt'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { DiscoverView } from '@/components/mods/DiscoverView'
import { InstalledView } from '@/components/mods/InstalledView'
import { UpdatesView } from '@/components/mods/UpdatesView'
import { SearchResultsView } from '@/components/mods/SearchResultsView'
import { PlanView, changes, failedTitle, type OsChangePlan, type PlanPhase, type PlanProgress, type PlanQuestions, type PlanResult, type PlanSnapshot } from '@/components/mods/PlanView'
import { PlanSheet } from '@/components/mods/PlanSheet'
import { PlanCount, noPlanWash, type PlanBarModel, type PlanLevel, type PlanWash } from '@/components/mods/PlanBar'
import { InstallQuestionDialog, companionGate, gateCompanion, type Gate, type InstallAnswer, type InstallAnswers, type InstallQuestion } from '@/components/mods/InstallQuestionDialog'
import { useScrollFades } from '@/hooks/useScrollFades'
import { usePackageFavorites } from '@/hooks/usePackageFavorites'
import { useFollowedDevs } from '@/hooks/useFollowedDevs'
import { useAppContext } from '@/contexts/AppContext'
import { noPlanSolveFacts, type ModsStatus, type PlanSolveFacts } from '@/lib/modsStatus'
import { installedVersionOf, upgradeCandidateMap } from '@/lib/updates'
import { InstallProgress, InstallStatus, OperationResult, PackageCompatibilityStatus, PackageInfo, PackageConflict, QueueConflictEntry, UpgradeCandidate } from '@/lib/types'
import { newOperationId, runOperation } from '@/lib/operations'
import { cn } from '@/lib/utils'
import { DeviceService, OperationsService, PackagesService } from '@bindings/reManager'

export type SelectPackageForOSFn = (name: string, targetOS: string, isCompatible?: boolean) => Promise<void>

export interface OsSyncStatus {
  state: 'idle' | 'running' | 'stopped' | 'failed'
  incompatible: number
  removals: number
  // Null until Vellum can say, which it cannot while an incompatible mod is installed.
  changing: number | null
  settled: number
  total: number
  applied: number
  requested: number
}

export interface PlanActions {
  open: () => void
  applyOsSync: () => void
}

// A numeral's glyph sits high in its line box from reserved descender space, so SubTabCount uses text-box to trim to cap height; that trim is font-independent and holds for SF Pro and Roboto alike.
const subTabBadge = 'ml-1.5 inline-flex h-5 min-w-5 items-center justify-center rounded-full border px-[5px] py-0 text-[11px] font-medium leading-none tabular-nums'

function SubTabCount({ children }: { children: React.ReactNode }) {
  return <span className="block [text-box:trim-both_cap_alphabetic]">{children}</span>
}

const subTabTrigger = 'rounded-none border-b-2 border-transparent px-4 py-2 touch:hit-target text-sm font-medium text-muted-foreground shadow-none shrink-0 whitespace-nowrap focus-visible:inset-ring-2 focus-visible:inset-ring-[var(--ring)] data-[state=active]:border-foreground data-[state=active]:text-foreground data-[state=active]:bg-transparent data-[state=active]:shadow-none'

interface ModsTabProps {
  activeTab: string
  osMismatchDetected: boolean
  storedOsVersion: string
  currentOsVersion: string
  compatibilityStatus: PackageCompatibilityStatus | null
  onOsSynced: () => void
  onOpenOSManager: () => void
  bootstrapping: boolean
  bootstrapOutput: string
  bootstrapError: string | null
  onInstallVellum: () => void
  vellumBrokenInstall: string[] | null
  vellumCleaning: boolean
  onCleanupVellum: () => void

  installQueue: Set<string>
  setInstallQueue: Dispatch<SetStateAction<Set<string>>>
  uninstallQueue: Set<string>
  setUninstallQueue: Dispatch<SetStateAction<Set<string>>>
  upgradeQueue: Set<string>
  setUpgradeQueue: Dispatch<SetStateAction<Set<string>>>
  upgradeCandidates: Map<string, UpgradeCandidate> | null
  queueError: string | null
  setQueueError: (v: string | null) => void
  refreshingPackages: boolean
  setRefreshingPackages: (v: boolean) => void

  modsStatus: ModsStatus
  setPlanSolve?: (facts: PlanSolveFacts) => void
  setPlanOpen?: (v: boolean) => void
  setPlanApplying?: (v: boolean) => void
  setPlanBar?: (model: PlanBarModel | null) => void
  setModsLocked?: (locked: boolean) => void
  setOsSyncStatus?: (status: OsSyncStatus | null) => void
  planActionsRef?: React.MutableRefObject<PlanActions | null>

  selectPackageForOSRef?: React.MutableRefObject<SelectPackageForOSFn | null>
  openPackageByNameRef?: React.MutableRefObject<((name: string) => void) | null>
}

const noInstallAnswers: InstallAnswers = { providers: {}, companions: [], settledGates: [] }

const gates: Gate[] = ['oxide', 'xovi-extensions']

// Monaco's input surface is not always a textarea, so its container is checked too.
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable
    || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
    || target.closest('.monaco-editor') !== null
}

function installRequest(queue: Iterable<string>, answers: InstallAnswers): string[] {
  return [...new Set([...Object.values(answers.providers), ...answers.companions, ...queue])]
}

function conflictNotesFor(entries: QueueConflictEntry[], installed: Map<string, string>): Map<string, string> {
  const notes = new Map<string, string>()
  const ownerOf = new Map(entries.map(entry => [entry.package, entry.queueEntry]))
  for (const { queueEntry, package: mine, blocks } of entries) {
    const blocksOwner = ownerOf.get(blocks)
    const other = blocksOwner && blocksOwner !== blocks ? `${blocks}, from ${blocksOwner}`
      : !blocksOwner && installed.has(blocks) ? `${blocks}, which is installed`
        : blocks
    notes.set(queueEntry, queueEntry === mine
      ? `Conflicts with ${other}`
      : `Brings in ${mine}, which conflicts with ${other}`)
  }
  return notes
}

export function ModsTab({
  activeTab,
  osMismatchDetected, storedOsVersion, currentOsVersion,
  compatibilityStatus, onOsSynced, onOpenOSManager,
  bootstrapping, bootstrapOutput,
  bootstrapError, onInstallVellum, vellumBrokenInstall, vellumCleaning, onCleanupVellum,
  installQueue, setInstallQueue, uninstallQueue, setUninstallQueue,
  upgradeQueue, setUpgradeQueue, upgradeCandidates,
  queueError, setQueueError, refreshingPackages, setRefreshingPackages,
  modsStatus, setPlanSolve, setPlanOpen, setPlanApplying, setPlanBar, setModsLocked, setOsSyncStatus, planActionsRef,
  selectPackageForOSRef, openPackageByNameRef,
}: ModsTabProps) {
  const ctx = useAppContext()
  const { device, deviceInfo, connectionStatus, installedPackages, installTimes, packages, setPackages, vellumInstalled, commandRunning, rescanAllPackages, refreshAfterPackageChange, refreshAfterUpgrade } = ctx
  const resolvedTerminalTheme = ctx.resolvedTerminalTheme as 'dark' | 'light'

  const [search, setSearch] = useState('')
  const [subTab, setSubTab] = useState('discover')
  const searching = search.trim().length > 0
  const NO_SUB_TAB = '__search__'
  const { scrollRef: subTabsRef, fadeLeft: subTabsFadeLeft, fadeRight: subTabsFadeRight } = useScrollFades<HTMLDivElement>()
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() => localStorage.getItem('rm-viewMode') === 'list' ? 'list' : 'grid')
  const [selectedPackage, setSelectedPackage] = useState<PackageInfo | null>(null)
  const [sidebarViewOnly, setSidebarViewOnly] = useState(false)
  const [sidebarIncompatible, setSidebarIncompatible] = useState(false)
  const [sidebarHistory, setSidebarHistory] = useState<PackageInfo[]>([])
  const [detailTab, setDetailTab] = useState<'overview' | 'changelog'>('overview')
  const [applyingPlan, setApplyingPlan] = useState(false)
  const applyOperationRef = useRef<string | null>(null)
  const stopRequestedRef = useRef(false)
  const [stopRequested, setStopRequested] = useState(false)
  const [installAnswers, setInstallAnswers] = useState<InstallAnswers>(noInstallAnswers)
  const [openQuestions, setOpenQuestions] = useState<InstallQuestion[]>([])
  const [waiting, setWaiting] = useState<Set<string>>(new Set())
  const [askedQuestion, setAskedQuestion] = useState<{ question: InstallQuestion; current: string | boolean | null } | null>(null)
  const [applySnapshot, setApplySnapshot] = useState<PlanSnapshot | null>(null)
  const [applyResult, setApplyResult] = useState<PlanResult | null>(null)
  const [applyOutput, setApplyOutput] = useState<string[]>([])
  const [applyProgress, setApplyProgress] = useState<PlanProgress>(
    { running: new Set(), done: new Set(), failed: new Set() })
  const [planSheetOpen, setPlanSheetOpen] = useState(false)
  const planSheetOpenRef = useRef(planSheetOpen)
  planSheetOpenRef.current = planSheetOpen
  const { follows, followedDevs, toggleFollow, setPackageSeen } = useFollowedDevs()
  const { starred, toggleStar } = usePackageFavorites()
  const searchRef = useRef<HTMLInputElement>(null)
  const showDeveloperRef = useRef<((dev: string) => void) | null>(null)
  const resetDiscoverRef = useRef<(() => void) | null>(null)

  const showBroken = vellumBrokenInstall !== null
  const showInstallPrompt = !showBroken && vellumInstalled === false
  const showOsMismatch = !showBroken && !showInstallPrompt && osMismatchDetected
  const showMain = !showBroken && !showInstallPrompt && !showOsMismatch

  const browsablePackages = useMemo(() => packages.filter(p => !p.hidden), [packages])

  const followFeedCount = modsStatus.discover.count

  const currentVersion = (name: string) =>
    installedVersionOf(name, installedPackages, upgradeCandidates)

  const upgradableCount = modsStatus.upgrades.count
  const upgradesUnstaged = modsStatus.upgrades.unstaged > 0

  useEffect(() => { if (selectedPackage) { const u = packages.find(p => p.name === selectedPackage.name); if (u && u !== selectedPackage) setSelectedPackage(u) } }, [packages])
  useEffect(() => {
    if (activeTab !== 'mods') return
    const handler = (e: KeyboardEvent) => { if (e.key === '/' && !e.ctrlKey && !e.metaKey && !isEditableTarget(e.target)) { e.preventDefault(); searchRef.current?.focus() } }
    document.addEventListener('keydown', handler); return () => document.removeEventListener('keydown', handler)
  }, [activeTab])

  const installQueueRef = useRef(installQueue)
  const uninstallQueueRef = useRef(uninstallQueue)
  const upgradeQueueRef = useRef(upgradeQueue)
  useEffect(() => { installQueueRef.current = installQueue }, [installQueue])
  useEffect(() => { uninstallQueueRef.current = uninstallQueue }, [uninstallQueue])
  useEffect(() => { upgradeQueueRef.current = upgradeQueue }, [upgradeQueue])

  const installedKey = useMemo(
    () => [...installedPackages.entries()].map(([name, v]) => `${name}=${v}`).sort().join(','),
    [installedPackages]
  )

  const [packageConflicts, setPackageConflicts] = useState<Record<string, PackageConflict | undefined>>({})

  const uninstallKey = useMemo(() => [...uninstallQueue].sort().join(','), [uninstallQueue])

  useEffect(() => {
    if (connectionStatus !== 'connected') {
      setPackageConflicts({})
      return
    }
    if (applyingPlan) return
    let stale = false
    const installedAfterRemovals = Object.fromEntries(
      [...installedPackages].filter(([name]) => !uninstallQueue.has(name)))
    PackagesService.GetPackageConflicts(installedAfterRemovals, installRequest(installQueue, installAnswers), device, deviceInfo.firmware || '')
      .then(result => { if (!stale) setPackageConflicts(result || {}) })
      .catch(err => console.error('GetPackageConflicts failed:', err))
    return () => { stale = true }
  }, [installQueue, installAnswers, uninstallKey, installedKey, device, deviceInfo.firmware, connectionStatus, applyingPlan])

  const [queueConflict, setQueueConflict] = useState<{ entries: QueueConflictEntry[]; detail: string } | null>(null)
  const queueConflicts = queueConflict?.entries ?? []
  const [impliedPackages, setImpliedPackages] = useState<Set<string>>(new Set())
  const [solvedUpgrades, setSolvedUpgrades] = useState<Map<string, UpgradeCandidate>>(new Map())
  // A staged upgrade runs before the install phase, so by then apk has nothing left to move.
  const impliedUpgrades = useMemo(
    () => new Map([...solvedUpgrades].filter(([name]) => !upgradeQueue.has(name))),
    [solvedUpgrades, upgradeQueue],
  )

  const keptProviders = async (queue: string[], providers: Record<string, string>): Promise<Record<string, string>> => {
    const kept: Record<string, string> = {}
    if (!Object.keys(providers).length) return kept
    for (;;) {
      const choices = await PackagesService.GetInstallChoices([...Object.values(kept), ...queue], device) ?? []
      const newlyNeeded = choices.map(c => c.virtual).filter(v => providers[v] && !kept[v])
      if (!newlyNeeded.length) return kept
      newlyNeeded.forEach(v => { kept[v] = providers[v] })
    }
  }

  const [installSolving, setInstallSolving] = useState(false)
  const [removalSolving, setRemovalSolving] = useState(false)

  useEffect(() => {
    if (connectionStatus !== 'connected' || installQueue.size === 0) {
      setQueueConflict(null)
      setImpliedPackages(new Set())
      setSolvedUpgrades(new Map())
      setOpenQuestions([])
      setWaiting(new Set())
      setInstallSolving(false)
      if (installQueue.size === 0) setInstallAnswers(noInstallAnswers)
      return
    }
    if (applyingPlan) return
    let stale = false
    setInstallSolving(true)
    const timer = window.setTimeout(async () => {
      try {
        const queue = [...installQueue]
        const providers = await keptProviders(queue, installAnswers.providers)
        const request = installRequest(queue, { ...installAnswers, providers })
        const choices = await PackagesService.GetInstallChoices(request, device) ?? []
        const held = new Set(choices.flatMap(c => c.requiredBy ?? []))
        const solvable = request.filter(name => !held.has(name))
        const sim = solvable.length ? await PackagesService.SimulateInstall(solvable, [...uninstallQueue], device) : null
        const tree = sim && !sim.error ? sim.packages || [] : []

        let gateTree: string[] | null = sim && !sim.error ? tree : null
        if (installAnswers.settledGates.length && installAnswers.companions.length) {
          const bare = solvable.filter(name => !installAnswers.companions.includes(name))
          const bareSim = bare.length ? await PackagesService.SimulateInstall(bare, [...uninstallQueue], device) : null
          gateTree = bareSim && !bareSim.error ? bareSim.packages || [] : null
        }
        if (stale) return
        const settledGates = gateTree
          ? installAnswers.settledGates.filter(gate => gateTree.includes(gate))
          : installAnswers.settledGates
        const companions = installAnswers.companions.filter(name => settledGates.includes(companionGate[name]))
        const pruned = Object.keys(providers).length !== Object.keys(installAnswers.providers).length
          || settledGates.length !== installAnswers.settledGates.length
          || companions.length !== installAnswers.companions.length
        if (pruned) {
          setInstallAnswers({ providers, companions, settledGates })
          return
        }

        const openGates = gates.filter(gate =>
          tree.includes(gate) && !installedPackages.has(gate) && !installAnswers.settledGates.includes(gate))
        setWaiting(held)
        setOpenQuestions([
          ...choices.map(choice => ({ kind: 'provider' as const, choice })),
          ...openGates.map(gate => ({ kind: 'gate' as const, gate })),
        ])
        setQueueConflict(sim?.error ? { entries: sim.conflicts || [], detail: sim.error } : null)
        const answered = [...Object.values(installAnswers.providers), ...installAnswers.companions]
        setImpliedPackages(new Set(
          (sim && !sim.error ? tree : answered).filter(name => !installQueue.has(name) && !installedPackages.has(name))
        ))
        setSolvedUpgrades(new Map(
          [...upgradeCandidateMap(sim && !sim.error ? sim.upgrades : null)].filter(([name]) => !installQueue.has(name))
        ))
        if (!sim?.error) setQueueError(null)
        setInstallSolving(false)
      } catch (err) {
        console.error('plan check failed:', err)
        if (!stale) setInstallSolving(false)
      }
    }, 400)
    return () => { stale = true; window.clearTimeout(timer) }
  }, [installQueue, installAnswers, uninstallKey, installedKey, device, connectionStatus, applyingPlan])

  const solvableInstalls = useMemo(
    () => installRequest(installQueue, installAnswers).filter(name => !waiting.has(name)),
    [installQueue, installAnswers, waiting],
  )

  const [impliedRemovals, setImpliedRemovals] = useState<Set<string>>(new Set())
  const [removalInstallFirst, setRemovalInstallFirst] = useState(false)

  useEffect(() => {
    if (connectionStatus !== 'connected' || uninstallQueue.size === 0) {
      setImpliedRemovals(new Set())
      setRemovalInstallFirst(false)
      setRemovalSolving(false)
      return
    }
    if (applyingPlan) return
    let stale = false
    setRemovalSolving(true)
    const timer = window.setTimeout(() => {
      resolveRemovalTree([...uninstallQueue], showOsMismatch ? [] : solvableInstalls).then(({ removed, installFirst }) => {
        if (stale) return
        setImpliedRemovals(new Set(removed.filter(name => !uninstallQueue.has(name))))
        setRemovalInstallFirst(installFirst)
        setRemovalSolving(false)
      })
    }, 400)
    return () => { stale = true; window.clearTimeout(timer) }
  }, [uninstallKey, solvableInstalls, showOsMismatch, installedKey, connectionStatus, applyingPlan])

  // The sheet is modal={false} so the README panel beside it keeps pointer events (Radix's modal would trap them), which is also why this scroll lock is applied by hand.
  useEffect(() => {
    if (!selectedPackage) return
    const root = document.documentElement
    const previous = { html: root.style.overflow, body: document.body.style.overflow }
    root.style.overflow = 'hidden'
    document.body.style.overflow = 'hidden'
    return () => {
      root.style.overflow = previous.html
      document.body.style.overflow = previous.body
    }
  }, [selectedPackage])

  const planCount = modsStatus.plan.count


  useEffect(() => { setPlanApplying?.(applyingPlan) }, [applyingPlan, setPlanApplying])

  useEffect(() => {
    if (searching) return
    const strip = subTabsRef.current
    const active = strip?.querySelector<HTMLElement>('[data-state="active"]')
    if (!strip || !active) return
    // scrollIntoView with block:'nearest' also scrolls ancestors (the page, under the sticky header and safe-area scrim), so set the strip's own scrollLeft instead.
    const left = active.offsetLeft
    const right = left + active.offsetWidth
    if (left < strip.scrollLeft) {
      strip.scrollLeft = left
    } else if (right > strip.scrollLeft + strip.clientWidth) {
      strip.scrollLeft = right - strip.clientWidth
    }
  }, [subTab, searching])

  useEffect(() => {
    setPlanSolve?.({ implied: impliedPackages.size + impliedUpgrades.size, impliedRemovals: impliedRemovals.size, conflict: queueConflict !== null, openQuestions: openQuestions.length })
  }, [impliedPackages.size, impliedUpgrades.size, impliedRemovals.size, queueConflict, openQuestions.length, setPlanSolve])
  useEffect(() => () => setPlanSolve?.(noPlanSolveFacts), [setPlanSolve])

  useEffect(() => { setPlanOpen?.(planSheetOpen) }, [planSheetOpen, setPlanOpen])

  const conflictNotes = useMemo(
    () => conflictNotesFor(queueConflicts, installedPackages),
    [queueConflicts, installedPackages],
  )

  const getConflict = useCallback((pkg: PackageInfo): string | null => {
    const conflict = packageConflicts[pkg.name]
    if (!conflict) return null
    return conflict.queued
      ? `Conflicts with ${conflict.conflictsWith}, which is queued`
      : `Conflicts with ${conflict.conflictsWith}, which is installed`
  }, [packageConflicts])

  const openPackage = useCallback((pkg: PackageInfo) => {
    setSidebarViewOnly(false)
    setSidebarIncompatible(!pkg.compatible)
    setDetailTab('overview')
    setSelectedPackage(pkg)
  }, [])
  const openUpgradePackage = useCallback((pkg: PackageInfo) => {
    setSidebarViewOnly(false)
    setSidebarIncompatible(false)
    setDetailTab('changelog')
    setSelectedPackage(pkg)
  }, [])

  const openPackageByName = useCallback((name: string) => {
    const pkg = packages.find(p => p.name === name)
    if (pkg) {
      openPackage(pkg)
      return
    }
    setSelectedPackage(null)
    setSearch(name)
  }, [packages, openPackage])

  useEffect(() => {
    if (!openPackageByNameRef) return
    openPackageByNameRef.current = openPackageByName
    return () => { openPackageByNameRef.current = null }
  }, [openPackageByNameRef, openPackageByName])

  const queueErrorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const flashQueueError = useCallback((message: string) => {
    if (queueErrorTimerRef.current) clearTimeout(queueErrorTimerRef.current)
    setQueueError(message)
    queueErrorTimerRef.current = setTimeout(() => setQueueError(null), 4000)
  }, [setQueueError])
  useEffect(() => () => {
    if (queueErrorTimerRef.current) clearTimeout(queueErrorTimerRef.current)
    setQueueError(null)
  }, [setQueueError])

  const queueInstalls = useCallback((names: string[]) => {
    if (showOsMismatch) { flashQueueError(`Mods can be installed once they're updated for OS ${currentOsVersion}`); return }
    const blocked = names.find(name => uninstallQueueRef.current.has(name))
    if (blocked) flashQueueError(`Cannot add ${blocked} while it's queued for removal`)
    const added = names.filter(name => !uninstallQueueRef.current.has(name) && !installQueueRef.current.has(name))
    if (!added.length) return
    setInstallQueue(prev => new Set([...prev, ...added]))
  }, [flashQueueError, setInstallQueue, showOsMismatch, currentOsVersion])
  const addToQueue = useCallback((name: string) => queueInstalls([name]), [queueInstalls])
  const removeFromQueue = useCallback((name: string) => {
    setInstallQueue(prev => { const q = new Set(prev); q.delete(name); return q })
  }, [setInstallQueue])
  const toggleInstallQueue = useCallback((name: string) => {
    installQueueRef.current.has(name) ? removeFromQueue(name) : addToQueue(name)
  }, [addToQueue, removeFromQueue])
  const clearQueue = useCallback(() => setInstallQueue(new Set()), [setInstallQueue])
  const addToUninstallQueue = useCallback((name: string) => {
    if (installQueueRef.current.has(name)) { flashQueueError(`Cannot remove ${name} while it's queued for installation`); return }
    setUninstallQueue(prev => new Set([...prev, name]))
  }, [flashQueueError, setUninstallQueue])
  const removeFromUninstallQueue = useCallback((id: string) => {
    setUninstallQueue(prev => { const q = new Set(prev); q.delete(id); return q })
  }, [setUninstallQueue])
  const toggleUninstallQueue = useCallback((name: string) => {
    if (uninstallQueueRef.current.has(name)) { removeFromUninstallQueue(name); return }
    setUpgradeQueue(prev => { if (!prev.has(name)) return prev; const u = new Set(prev); u.delete(name); return u })
    addToUninstallQueue(name)
  }, [addToUninstallQueue, removeFromUninstallQueue, setUpgradeQueue])
  const toggleUpgradeQueue = useCallback((name: string) => {
    if (!upgradeQueueRef.current.has(name) && uninstallQueueRef.current.has(name)) {
      flashQueueError(`Cannot upgrade ${name} while it's queued for removal`)
      return
    }
    setUpgradeQueue(prev => { const q = new Set(prev); q.has(name) ? q.delete(name) : q.add(name); return q })
  }, [flashQueueError, setUpgradeQueue])
  const removeFromUpgradeQueue = useCallback((id: string) => {
    setUpgradeQueue(prev => { const q = new Set(prev); q.delete(id); return q })
  }, [setUpgradeQueue])
  const clearUpgradeQueue = useCallback(() => setUpgradeQueue(new Set()), [setUpgradeQueue])
  const clearUninstallQueue = useCallback(() => setUninstallQueue(new Set()), [setUninstallQueue])

  // Vellum refuses to simulate the sync while any installed mod is incompatible, so until then the sync rows have no targets.
  const osChangePlan = useMemo((): OsChangePlan | undefined => {
    if (!showOsMismatch) return undefined
    const incompatibleInstalled = compatibilityStatus?.incompatiblePackages ?? []
    const syncTargets = incompatibleInstalled.length === 0 ? upgradeCandidates : null
    const syncing = syncTargets ? [...syncTargets.keys()] : compatibilityStatus?.compatiblePackages ?? []
    return {
      from: storedOsVersion,
      to: currentOsVersion,
      incompatible: incompatibleInstalled.filter(name => !uninstallQueue.has(name)),
      sync: syncing.filter(name => !uninstallQueue.has(name)),
      syncTargets,
      targetsAwaitRemovals: incompatibleInstalled.length > 0,
      onStageRemoval: addToUninstallQueue,
      onStageAllRemovals: () => incompatibleInstalled.forEach(addToUninstallQueue),
      onOpenOSManager,
    }
  }, [showOsMismatch, compatibilityStatus, upgradeCandidates, uninstallQueue, storedOsVersion, currentOsVersion, addToUninstallQueue, onOpenOSManager])

  const unqueue = (names: string[]) => {
    if (!names.length) return
    const applied = new Set(names)
    const withoutApplied = (prev: Set<string>) => {
      const next = new Set([...prev].filter(name => !applied.has(name)))
      return next.size === prev.size ? prev : next
    }
    setUninstallQueue(withoutApplied)
    setUpgradeQueue(withoutApplied)
    setInstallQueue(withoutApplied)
  }

  const stagedAtResultRef = useRef<Set<string> | null>(null)
  const dismissApplyResult = useCallback(() => {
    setApplySnapshot(null)
    setApplyResult(null)
    stagedAtResultRef.current = null
  }, [])

  useEffect(() => {
    const staged = new Set([
      ...[...installQueue].map(n => `install:${n}`),
      ...[...upgradeQueue].map(n => `upgrade:${n}`),
      ...[...uninstallQueue].map(n => `remove:${n}`),
    ])
    if (!applyResult) return
    if (!stagedAtResultRef.current) {
      stagedAtResultRef.current = staged
      return
    }
    const before = stagedAtResultRef.current
    if ([...staged].some(key => !before.has(key))) dismissApplyResult()
  }, [applyResult, installQueue, upgradeQueue, uninstallQueue, dismissApplyResult])


  const handleRefreshPackages = async () => {
    setRefreshingPackages(true)
    try { await PackagesService.RefreshMetadata(); if (device) { const info = await DeviceService.GetDeviceInfo(); const arch = await DeviceService.GetDeviceArchitecture(device); setPackages(await PackagesService.GetPackages(device, info?.firmware || '', arch) || []) }; await rescanAllPackages() }
    catch (err) { console.error('Failed to refresh packages:', err) }
    setRefreshingPackages(false)
  }
  const handleSelectPackageForOSInternal = async (name: string, targetOS: string, isCompatible: boolean = true) => {
    try { const arch = await DeviceService.GetDeviceArchitecture(device); let pkg = await PackagesService.GetPackageForOS(name, targetOS, device, arch); if (!pkg) pkg = packages.find(p => p.name === name) || null; if (pkg) { setSidebarViewOnly(true); setSidebarIncompatible(!isCompatible); setSelectedPackage(pkg) } }
    catch (err) { console.error('Failed to get package for OS:', err) }
  }
  useEffect(() => { if (selectPackageForOSRef) selectPackageForOSRef.current = handleSelectPackageForOSInternal; return () => { if (selectPackageForOSRef) selectPackageForOSRef.current = null } })
  const openDeveloper = useCallback((dev: string) => {
    setSelectedPackage(null)
    setSidebarHistory([])
    setSidebarViewOnly(false)
    setSidebarIncompatible(false)
    setSearch('')
    setSubTab('discover')
    showDeveloperRef.current?.(dev)
  }, [])

  const installStarred = useCallback(async (names: string[]) => {
    const installedAfterRemovals = Object.fromEntries(
      [...installedPackages].filter(([name]) => !uninstallQueueRef.current.has(name)))
    const accepted: string[] = []
    const skipped: string[] = []
    for (const name of names) {
      try {
        const conflicts = await PackagesService.GetPackageConflicts(
          installedAfterRemovals, [...installRequest(installQueueRef.current, installAnswers), ...accepted], device, deviceInfo.firmware || '')
        if (conflicts?.[name]) { skipped.push(name); continue }
      } catch (err) {
        console.error('GetPackageConflicts failed:', err)
      }
      accepted.push(name)
    }
    queueInstalls(accepted)
    if (skipped.length) flashQueueError(`Skipped ${skipped.join(', ')}: conflicts with your changes`)
  }, [queueInstalls, installedPackages, installAnswers, device, deviceInfo.firmware, flashQueueError])
  const cancelStarred = useCallback((names: string[]) => {
    names.forEach(name => removeFromQueue(name))
  }, [removeFromQueue])
  const handleViewModeChange = (mode: 'grid' | 'list') => { setViewMode(mode); localStorage.setItem('rm-viewMode', mode) }

  const handleUpdateAll = () => {
    const toUpdate: string[] = upgradeCandidates
      ? [...upgradeCandidates.keys()]
      : []
    if (!upgradeCandidates) {
      for (const [name, iv] of installedPackages) { const pkg = packages.find(p => p.name === name); if (pkg && pkg.version !== iv && pkg.compatible) toUpdate.push(name) }
    }
    const eligible = toUpdate.filter(n => !uninstallQueue.has(n))
    if (eligible.length) { const q = new Set(upgradeQueue); for (const n of eligible) q.add(n); setUpgradeQueue(q) }
  }

  const findCompanionConflict = async (companion: string): Promise<string | null> => {
    try {
      const installs = solvableInstalls.filter(name => name !== companion)
      const sim = await PackagesService.SimulateInstall([companion, ...installs], [...uninstallQueue], device)
      if (!sim?.error) return null
      return sim.conflicts?.find(entry => entry.queueEntry === companion)?.blocks ?? null
    } catch (err) {
      console.error('companion conflict check failed:', err)
      return null
    }
  }

  async function resolveRemovalTree(removals: string[], installs: string[]): Promise<{ removed: string[]; installFirst: boolean }> {
    try {
      const sim = await PackagesService.SimulateUninstall(removals, installs, device)
      return { removed: sim?.packages?.length ? sim.packages : removals, installFirst: !!sim?.installFirst }
    } catch (err) {
      console.error('SimulateUninstall failed:', err)
      return { removed: removals, installFirst: false }
    }
  }

  const handleApplyPlan = () => {
    stopRequestedRef.current = false
    setStopRequested(false)
    const installs = showOsMismatch || installQueue.size === 0 ? [] : installRequest(installQueue, installAnswers)
    runPlan(installs)
  }

  const runPlan = async (installs: string[]) => {
    const removals = [...uninstallQueue]
    const syncTo = osChangePlan ? currentOsVersion : null
    let syncTargets = osChangePlan?.syncTargets ?? null
    let upgrades = osChangePlan ? osChangePlan.sync : [...upgradeQueue]

    const implied = [...impliedPackages]
    const planned = [...installs.filter(name => installQueue.has(name)), ...implied]
    const impliedUpgrade = installs.length ? new Map(impliedUpgrades) : new Map<string, UpgradeCandidate>()
    const installFirst = removals.length > 0 && removalInstallFirst
    const impliedRemove = removals.length ? [...impliedRemovals] : []
    const plannedRemovals = [...removals, ...impliedRemove]

    stagedAtResultRef.current = null
    setApplySnapshot({
      remove: plannedRemovals, upgrade: upgrades, install: planned, implied, impliedRemove, impliedUpgrade,
      sync: syncTo ? { to: syncTo, targets: syncTargets } : undefined,
    })
    setApplyResult(null)
    setApplyOutput([])
    setApplyingPlan(true)

    const progress: PlanProgress = { running: new Set(), done: new Set(), failed: new Set() }
    const publish = () => setApplyProgress({
      running: new Set(progress.running), done: new Set(progress.done), failed: new Set(progress.failed),
    })
    publish()

    const append = (line?: string) => {
      if (!line) return
      setApplyOutput(prev => (prev.length > 500 ? [...prev.slice(-500), line] : [...prev, line]))
    }
    const handlers = {
      onProgress: (p: InstallProgress) => {
        append(p.message)
        if (!p.component) return
        progress.running.clear()
        if (p.status === InstallStatus.InstallCompleted) progress.done.add(p.component)
        else if (p.status === InstallStatus.InstallError) progress.failed.add(p.component)
        else progress.running.add(p.component)
        publish()
      },
      onOutput: (text: string, hook: boolean) => {
        if (!hook) append(text.replace(/\n$/, ''))
      },
    }

    const runPhase = async (names: string[], start: (id: string) => Promise<unknown>, namesEachPackage: boolean) => {
      if (!namesEachPackage) {
        names.forEach(n => progress.running.add(n))
        publish()
      }
      const id = newOperationId()
      applyOperationRef.current = id
      const result = await runOperation(start, handlers, id)
      applyOperationRef.current = null
      progress.running.clear()
      if (result.ok) {
        names.forEach(n => { if (!progress.failed.has(n)) progress.done.add(n) })
      } else if (!result.canceled && !names.some(n => progress.failed.has(n))) {
        names.forEach(n => { if (!progress.done.has(n)) progress.failed.add(n) })
      }
      publish()
      unqueue(names.filter(n => progress.done.has(n)))
      return result
    }

    let outcome: PlanResult = { outcome: 'succeeded' }
    const settle = (result: OperationResult, phase: PlanPhase) => {
      if (result.ok) return true
      outcome = result.canceled
        ? { outcome: 'stopped' }
        : { outcome: 'failed', phase, error: result.error || 'The operation did not complete.' }
      return false
    }

    const runSync = async (to: string) => {
      if (!syncTargets) {
        try {
          const targets = upgradeCandidateMap(await PackagesService.GetUpgradeCandidates())
          syncTargets = targets
          upgrades = [...targets.keys()]
          setApplySnapshot(prev => prev && { ...prev, upgrade: upgrades, sync: { to, targets } })
        } catch (err) {
          console.error('GetUpgradeCandidates failed:', err)
        }
      }
      const result = await runPhase(upgrades, id => PackagesService.RunUpgrade(id), true)
      if (result.ok) onOsSynced()
      await refreshAfterUpgrade(result.ok, !!result.upgrade?.dnsError)
      return settle(result, 'upgrade')
    }

    type Phase = { names: string[]; always?: boolean; run: () => Promise<boolean> }
    const removePhase: Phase = { names: removals, run: async () => {
      const result = await runPhase(plannedRemovals, id => PackagesService.UninstallPackages(id, removals, device), true)
      await refreshAfterPackageChange(!!result.install?.dnsError)
      return settle(result, 'remove')
    } }
    const upgradePhase: Phase = { names: upgrades, always: syncTo !== null, run: async () => {
      if (syncTo) return runSync(syncTo)
      const result = await runPhase(upgrades, id => PackagesService.RunPackageUpgrade(id, upgrades), false)
      await refreshAfterUpgrade(result.ok, !!result.upgrade?.dnsError)
      return settle(result, 'upgrade')
    } }
    const installPhase: Phase = { names: installs, run: async () => {
      const result = await runPhase([...planned, ...impliedUpgrade.keys()], id => PackagesService.InstallPackages(id, installs, installs, device), true)
      await refreshAfterPackageChange(!!result.install?.dnsError)
      return settle(result, 'install')
    } }
    const phases = installFirst
      ? [installPhase, removePhase, upgradePhase]
      : [removePhase, upgradePhase, installPhase]

    try {
      for (const { names, always, run } of phases) {
        if (!names.length && !always) continue
        if (stopRequestedRef.current) {
          outcome = { outcome: 'stopped' }
          break
        }
        if (!await run()) break
      }
    } catch (err) {
      outcome = { outcome: 'failed', error: err instanceof Error ? err.message : String(err) }
    } finally {
      setApplyResult(outcome)
      setApplyingPlan(false)
      if (syncTo && outcome.outcome === 'succeeded' && !planSheetOpenRef.current) dismissApplyResult()
    }
  }

  const stopApply = () => {
    stopRequestedRef.current = true
    setStopRequested(true)
    if (applyOperationRef.current) void OperationsService.CancelOperation(applyOperationRef.current)
  }

  const finishApply = useCallback(() => {
    dismissApplyResult()
    setPlanSheetOpen(false)
  }, [dismissApplyResult])
  const openPlan = useCallback(() => setPlanSheetOpen(true), [])

  const onPlanSheetOpenChange = (open: boolean) => {
    if (!open && applySnapshot?.sync && applyResult?.outcome === 'succeeded') dismissApplyResult()
    setPlanSheetOpen(open)
  }

  const answerQuestion = (answer: InstallAnswer) => {
    setAskedQuestion(null)
    setOpenQuestions(prev => prev.filter(question => answer.kind === 'provider'
      ? !(question.kind === 'provider' && question.choice.virtual === answer.virtual)
      : !(question.kind === 'gate' && question.gate === answer.gate)))
    if (answer.kind === 'provider') {
      setInstallAnswers(prev => ({ ...prev, providers: { ...prev.providers, [answer.virtual]: answer.provider } }))
      return
    }
    const companion = gateCompanion[answer.gate]
    setInstallAnswers(prev => ({
      ...prev,
      settledGates: prev.settledGates.includes(answer.gate) ? prev.settledGates : [...prev.settledGates, answer.gate],
      companions: [...prev.companions.filter(name => name !== companion), ...(answer.companion ? [companion] : [])],
    }))
  }

  const changeProvider = async (virtual: string) => {
    const { [virtual]: current, ...otherProviders } = installAnswers.providers
    try {
      const request = installRequest(installQueue, { ...installAnswers, providers: otherProviders })
      const choice = (await PackagesService.GetInstallChoices(request, device) ?? []).find(c => c.virtual === virtual)
      if (choice) setAskedQuestion({ question: { kind: 'provider', choice }, current })
    } catch (err) {
      console.error('GetInstallChoices failed:', err)
    }
  }

  const changeGate = (gate: Gate) =>
    setAskedQuestion({ question: { kind: 'gate', gate }, current: installAnswers.companions.includes(gateCompanion[gate]) })

  const dropCompanion = (name: string) =>
    setInstallAnswers(prev => ({ ...prev, companions: prev.companions.filter(c => c !== name) }))

  const planQuestions: PlanQuestions = {
    open: openQuestions,
    answers: installAnswers,
    waiting,
    conflictNotes,
    onOpen: question => setAskedQuestion({ question, current: null }),
    onChangeProvider: virtual => { void changeProvider(virtual) },
    onChangeGate: changeGate,
    onDropCompanion: dropCompanion,
  }

  const applyBusy = applyingPlan || commandRunning
  const clearAllQueues = () => { clearQueue(); clearUninstallQueue(); clearUpgradeQueue(); setPlanSheetOpen(false) }

  const operationDisabled = commandRunning || connectionStatus !== 'connected'

  const applyNumbers = useMemo(() => {
    if (!applySnapshot) return null
    const names = [...applySnapshot.remove, ...applySnapshot.upgrade, ...applySnapshot.impliedUpgrade.keys(), ...applySnapshot.install]
    const implied = new Set([...applySnapshot.implied, ...applySnapshot.impliedRemove, ...applySnapshot.impliedUpgrade.keys()])
    const requested = names.filter(name => !implied.has(name))
    const settled = (name: string) => applyProgress.done.has(name) || applyProgress.failed.has(name)
    return {
      total: names.length,
      settled: names.filter(settled).length,
      requested: requested.length,
      applied: requested.filter(name => applyProgress.done.has(name)).length,
      removed: applySnapshot.remove.filter(name => !implied.has(name) && applyProgress.done.has(name)).length,
      upgraded: applySnapshot.upgrade.filter(name => applyProgress.done.has(name)).length,
      installed: applySnapshot.install.filter(name => !implied.has(name) && applyProgress.done.has(name)).length,
    }
  }, [applySnapshot, applyProgress])

  const syncing = !!applySnapshot?.sync
  const planLevel: PlanLevel = modsStatus.plan.level ?? 'pending'

  const [planBump, setPlanBump] = useState(0)
  const [planWash, setPlanWash] = useState<PlanWash>(noPlanWash)
  const stagedKeysRef = useRef<Set<string> | null>(null)
  useEffect(() => {
    const staged = new Set([
      ...[...installQueue].map(n => `install:${n}`),
      ...[...upgradeQueue].map(n => `upgrade:${n}`),
      ...[...uninstallQueue].map(n => `remove:${n}`),
    ])
    const before = stagedKeysRef.current
    stagedKeysRef.current = staged
    if (before === null || ![...staged].some(key => !before.has(key))) return
    const arrival = before.size === 0
    if (!arrival) setPlanBump(b => b + 1)
    setPlanWash(w => ({ key: w.key + 1, arrival }))
  }, [installQueue, upgradeQueue, uninstallQueue])

  const planBarVisible = showMain && !syncing && (planCount > 0 || applySnapshot !== null)

  const planBarModel = useMemo((): PlanBarModel | null => {
    if (!planBarVisible) return null
    const base = { level: planLevel, bump: 0, wash: noPlanWash, everyTab: applySnapshot !== null, onOpen: openPlan, onDone: finishApply, done: false }
    if (applySnapshot && applyNumbers) {
      const { total, settled, requested, applied, removed, upgraded, installed } = applyNumbers
      if (!applyResult) {
        return {
          ...base,
          icon: <CircleNotch className="h-4 w-4 animate-spin text-muted-foreground" />,
          title: `Applying ${changes(total)}`,
          detail: `${settled} of ${total}`,
          progress: total ? Math.round((settled / total) * 100) : 0,
        }
      }
      const appliedOf = `${applied} of ${changes(requested)} applied`
      if (applyResult.outcome === 'succeeded') {
        const parts = [removed && `${removed} removed`, upgraded && `${upgraded} upgraded`, installed && `${installed} installed`].filter(Boolean)
        return {
          ...base,
          icon: <CheckCircle className="h-4 w-4 text-[var(--success-text)]" />,
          title: `${changes(requested)} applied`,
          detail: parts.join(' · '),
          done: true,
        }
      }
      if (applyResult.outcome === 'stopped') {
        return { ...base, icon: <StopCircle className="h-4 w-4 text-muted-foreground" />, title: 'Stopped', detail: appliedOf }
      }
      return {
        ...base,
        icon: <WarningCircle className="h-4 w-4 text-destructive" />,
        title: applyResult.phase ? failedTitle(applyResult.phase, false) : 'Apply failed',
        titleClassName: 'text-destructive',
        detail: appliedOf,
      }
    }
    const removals = uninstallQueue.size + impliedRemovals.size
    const installs = installQueue.size + impliedPackages.size
    const upgrades = upgradeQueue.size + impliedUpgrades.size
    const counted = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`
    const summary = [
      removals && counted(removals, 'removal', 'removals'),
      upgrades && counted(upgrades, 'upgrade', 'upgrades'),
      installs && counted(installs, 'install', 'installs'),
    ].filter(Boolean).join(' · ')
    const detail = queueConflict !== null ? <span className="text-destructive">Remove one of the conflicting mods to apply.</span>
      : openQuestions.length > 0 ? <span className="text-[var(--warning-text)]">{openQuestions.length === 1 ? 'A question' : `${openQuestions.length} questions`} to answer</span>
      : summary
    return { ...base, bump: planBump, wash: planWash, title: 'Changes', count: planCount, detail }
  }, [planBarVisible, planLevel, openPlan, finishApply, applySnapshot, applyNumbers, applyResult, uninstallQueue.size, impliedRemovals.size, installQueue.size, impliedPackages.size, upgradeQueue.size, impliedUpgrades.size, queueConflict, openQuestions.length, planCount, planBump, planWash])

  // Layout effect so the bar lands in the same paint as the queue change that App already turned into the mods dot.
  useLayoutEffect(() => { setPlanBar?.(planBarModel) }, [planBarModel, setPlanBar])
  useEffect(() => () => setPlanBar?.(null), [setPlanBar])

  // Vellum refuses every other change until the sync records the new OS, so Mods stays locked while the OS change waits and while its sync runs.
  const modsLocked = showOsMismatch || (syncing && !applyResult)
  useEffect(() => { setModsLocked?.(modsLocked) }, [modsLocked, setModsLocked])
  useEffect(() => () => setModsLocked?.(false), [setModsLocked])

  const osSyncStatus = useMemo((): OsSyncStatus | null => {
    if (!showOsMismatch && !syncing) return null
    const state: OsSyncStatus['state'] = !syncing ? 'idle'
      : !applyResult ? 'running'
      : applyResult.outcome === 'stopped' ? 'stopped'
      : applyResult.outcome === 'failed' ? 'failed'
      : 'idle'
    return {
      state,
      incompatible: osChangePlan?.incompatible.length ?? 0,
      removals: showOsMismatch ? uninstallQueue.size : 0,
      changing: osChangePlan?.syncTargets ? osChangePlan.sync.length : null,
      settled: applyNumbers?.settled ?? 0,
      total: applyNumbers?.total ?? 0,
      applied: applyNumbers?.applied ?? 0,
      requested: applyNumbers?.requested ?? 0,
    }
  }, [showOsMismatch, syncing, applyResult, osChangePlan, uninstallQueue.size, applyNumbers])
  useEffect(() => { setOsSyncStatus?.(osSyncStatus) }, [osSyncStatus, setOsSyncStatus])
  useEffect(() => () => setOsSyncStatus?.(null), [setOsSyncStatus])

  useEffect(() => {
    if (!planActionsRef) return
    planActionsRef.current = {
      open: openPlan,
      applyOsSync: () => { if (showOsMismatch && !applyBusy) handleApplyPlan() },
    }
  })

  const planView = (
    <PlanView
      packages={packages}
      installedPackages={installedPackages}
      installQueue={installQueue}
      upgradeQueue={upgradeQueue}
      uninstallQueue={uninstallQueue}
      impliedPackages={impliedPackages}
      impliedRemovals={impliedRemovals}
      impliedUpgrades={impliedUpgrades}
      upgradeCandidates={upgradeCandidates}
      questions={showOsMismatch ? undefined : planQuestions}
      resolutionFailure={!showOsMismatch && queueConflict && conflictNotes.size === 0 ? queueConflict.detail : null}
      onSelectPackage={openPackage}
      onRemoveFromInstall={removeFromQueue}
      onRemoveFromUpgrade={removeFromUpgradeQueue}
      onRemoveFromUninstall={removeFromUninstallQueue}
      onClearAll={clearAllQueues}
      onApply={handleApplyPlan}
      applyLabel="Apply"
      applyBusy={applyingPlan}
      solving={showOsMismatch ? removalSolving : installSolving || removalSolving}
      applyBlocked={showOsMismatch
        ? (osChangePlan?.incompatible.length ?? 0) > 0 || removalSolving
        : queueConflict !== null || openQuestions.length > 0 || installSolving || removalSolving}
      snapshot={applySnapshot}
      progress={applyProgress}
      result={applyResult}
      onFinishApply={finishApply}
      onCloseApply={dismissApplyResult}
      onRetryApply={() => { dismissApplyResult(); void handleApplyPlan() }}
      onStopApply={stopApply}
      stopRequested={stopRequested}
      output={applyOutput}
      terminalTheme={resolvedTerminalTheme}
      disabled={operationDisabled}
      osChange={osChangePlan}
    />
  )

  const stepBackInDetail = sidebarHistory.length > 0
    ? () => {
        const prev = sidebarHistory[sidebarHistory.length - 1]
        setSidebarHistory(h => h.slice(0, -1))
        setSelectedPackage(prev)
        setSidebarViewOnly(false)
        setSidebarIncompatible(false)
      }
    : undefined

  return (
    <>
      <TabsContent value="mods" forceMount className={activeTab === 'mods' ? '' : 'hidden'}>
        {showBroken ? (
          <Card><CardHeader><div className="flex items-center gap-2"><WarningCircle className="h-5 w-5 text-destructive" /><CardTitle>Vellum installation incomplete</CardTitle></div><CardDescription>Your Vellum installation is missing core packages and needs to be reinstalled.</CardDescription></CardHeader><CardContent className="space-y-4"><p className="text-sm text-muted-foreground">Missing packages: {vellumBrokenInstall!.join(', ')}</p><div className="flex justify-end pt-2"><Button onClick={onCleanupVellum} disabled={vellumCleaning}>{vellumCleaning ? (<><CircleNotch className="mr-2 h-4 w-4 animate-spin" />Cleaning up…</>) : 'Clean up and reinstall'}</Button></div></CardContent></Card>
        ) : showInstallPrompt ? (
          <VellumInstallPrompt bootstrapping={bootstrapping} bootstrapOutput={bootstrapOutput} bootstrapError={bootstrapError} onInstall={onInstallVellum} terminalTheme={resolvedTerminalTheme} />
        ) : showOsMismatch ? (
          null
        ) : showMain ? (
          <div className="space-y-4">
            <div className="flex gap-2">
              <div className="relative flex-1">
                <MagnifyingGlass className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input ref={searchRef} placeholder="Search mods…" value={search} onChange={(e) => setSearch(e.target.value)} className={`pl-9 ${search ? 'pr-8' : ''}`} />
                {search && <button onClick={() => setSearch('')} aria-label="Clear search" className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"><X className="h-4 w-4" /></button>}
              </div>
              <Tooltip><TooltipTrigger asChild><Button variant="outline" size="icon" aria-label="Refresh package list" onClick={handleRefreshPackages} disabled={refreshingPackages || applyingPlan}>{refreshingPackages ? <CircleNotch className="h-4 w-4 animate-spin" /> : <ArrowsClockwise className="h-4 w-4" />}</Button></TooltipTrigger><TooltipContent>Refresh package list</TooltipContent></Tooltip>
            </div>

            <Tabs activationMode="manual" value={searching ? NO_SUB_TAB : subTab} onValueChange={(v) => { setSubTab(v); setSearch('') }}>
              <div
                aria-hidden
                className="pointer-events-none fixed inset-x-0 top-0 z-30 h-[calc(var(--safe-area-top)+8px)] bg-background hidden touch:block"
              />
              <div className="sticky top-0 touch:top-[calc(var(--safe-area-top)+8px)] z-30 bg-background border-b border-border mb-4 -ml-gutter-left -mr-gutter-right">
              <TabsList className="flex h-auto w-full gap-0 rounded-none bg-transparent p-0 justify-start">
                <div
                  ref={subTabsRef}
                  className={cn(
                    'flex min-w-0 flex-initial pl-gutter-left overflow-x-auto overscroll-x-none [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
                    '[--room:calc((var(--hit-target)_-_36px)/2)] py-[var(--room)] -my-[var(--room)]',
                    'pr-gutter-right',
                    subTabsFadeLeft && 'scroll-fade-left',
                    subTabsFadeRight && 'scroll-fade-right',
                  )}
                >
                  <TabsTrigger value="discover" className={subTabTrigger} onClick={() => { if (subTab === 'discover' && !searching) resetDiscoverRef.current?.(); else window.scrollTo({ top: 0 }) }}>Discover{followFeedCount > 0 && <Badge variant="unread" className={cn(subTabBadge, "border-[oklch(from_var(--unread)_l_c_h_/_25%)]")}><SubTabCount>{followFeedCount}</SubTabCount></Badge>}</TabsTrigger>
                  <TabsTrigger value="installed" className={subTabTrigger} onClick={() => window.scrollTo({ top: 0 })}>Installed{modsStatus.installed.count > 0 && <Badge variant="secondary" className={cn(subTabBadge, "border-border")}><SubTabCount>{modsStatus.installed.count}</SubTabCount></Badge>}</TabsTrigger>
                  <TabsTrigger value="updates" className={subTabTrigger} onClick={() => window.scrollTo({ top: 0 })}>Upgrades{upgradableCount > 0 && <Badge variant={upgradesUnstaged ? 'info' : 'secondary'} className={cn(subTabBadge, upgradesUnstaged ? "border-[oklch(from_var(--info)_l_c_h_/_25%)]" : "border-border")}>{upgradableCount}</Badge>}</TabsTrigger>
                </div>
              </TabsList>
              </div>
              <TabsContent value="discover" forceMount className={cn('mt-0', (searching || subTab !== 'discover') && 'hidden')}><DiscoverView packages={browsablePackages} installedPackages={installedPackages} installQueue={installQueue} impliedPackages={impliedPackages} uninstallQueue={uninstallQueue} onSelectPackage={openPackage} onToggleInstallQueue={toggleInstallQueue} getConflict={getConflict} disabled={operationDisabled} followedDevs={followedDevs} follows={follows} onToggleFollow={toggleFollow} onSetPackageSeen={setPackageSeen} starred={starred} onToggleStar={toggleStar} onInstallStarred={installStarred} onCancelStarred={cancelStarred} showDeveloperRef={showDeveloperRef} resetToOverviewRef={resetDiscoverRef} visible={activeTab === 'mods' && subTab === 'discover' && !searching} viewMode={viewMode} onViewModeChange={handleViewModeChange} /></TabsContent>
              <TabsContent value="installed" forceMount className={cn('mt-0', (searching || subTab !== 'installed') && 'hidden')}><InstalledView starred={starred} onToggleStar={toggleStar} packages={browsablePackages} installedPackages={installedPackages} installTimes={installTimes} uninstallQueue={uninstallQueue} onSelectPackage={openPackage} onToggleUninstallQueue={toggleUninstallQueue} disabled={operationDisabled} viewMode={viewMode} onViewModeChange={handleViewModeChange} /></TabsContent>
              <TabsContent value="updates" forceMount className={cn('mt-0', (searching || subTab !== 'updates') && 'hidden')}><UpdatesView starred={starred} onToggleStar={toggleStar} packages={packages} installedPackages={installedPackages} upgradeQueue={upgradeQueue} uninstallQueue={uninstallQueue} upgradeCandidates={upgradeCandidates} onSelectPackage={openUpgradePackage} onToggleUpgradeQueue={toggleUpgradeQueue} onToggleUninstallQueue={toggleUninstallQueue} onUpdateAll={handleUpdateAll} onClearUpgradeQueue={clearUpgradeQueue} disabled={operationDisabled} viewMode={viewMode} onViewModeChange={handleViewModeChange} /></TabsContent>
              {searching && (
                <SearchResultsView
                  starred={starred}
                  onToggleStar={toggleStar}
                  packages={browsablePackages}
                  installedPackages={installedPackages}
                  installTimes={installTimes}
                  installQueue={installQueue}
                  uninstallQueue={uninstallQueue}
                  impliedPackages={impliedPackages}
                  search={search}
                  onSelectPackage={openPackage}
                  onToggleInstallQueue={toggleInstallQueue}
                  onToggleUninstallQueue={toggleUninstallQueue}
                  getConflict={getConflict}
                  disabled={operationDisabled}
                  viewMode={viewMode}
                  onViewModeChange={handleViewModeChange}
                />
              )}
            </Tabs>
            {queueError && <div className="bg-destructive/10 border border-destructive/20 text-destructive px-4 py-3 rounded-lg text-sm">{queueError}</div>}
          </div>
        ) : null}
      </TabsContent>

      <PlanSheet
        open={planSheetOpen}
        onOpenChange={onPlanSheetOpenChange}
        title={<>Changes{!applySnapshot && planCount > 0 && <PlanCount count={planCount} level={planLevel} />}</>}
      >
        {planView}
      </PlanSheet>

      {(activeTab === 'mods' || planSheetOpen) && (
      <>
      <InstallQuestionDialog
        question={askedQuestion?.question ?? null}
        current={askedQuestion?.current ?? null}
        installedPackages={installedPackages}
        installQueue={installQueue}
        firmware={deviceInfo.firmware || currentOsVersion}
        isConnected={connectionStatus === 'connected'}
        onFindCompanionConflict={findCompanionConflict}
        onAnswer={answerQuestion}
        onCancel={() => setAskedQuestion(null)}
        onPackageInfo={handleSelectPackageForOSInternal}
      />

      <PackageDetailModal open={selectedPackage !== null} onBack={stepBackInDetail} onClose={() => { setSelectedPackage(null); setSidebarViewOnly(false); setSidebarIncompatible(false); setSidebarHistory([]) }}>
          {selectedPackage && (
            <PackageDetailPanel
              pkg={selectedPackage}
              isInstalled={currentVersion(selectedPackage.name) !== undefined}
              detailsUnavailable={!packages.some(p => p.name === selectedPackage.name)}
              installedPackages={installedPackages}
              installedVersion={currentVersion(selectedPackage.name)}
              onInstall={() => toggleInstallQueue(selectedPackage.name)}
              onUninstall={() => toggleUninstallQueue(selectedPackage.name)}
              onUpdate={() => toggleUpgradeQueue(selectedPackage.name)}
              queuedAction={installQueue.has(selectedPackage.name) ? 'install' : upgradeQueue.has(selectedPackage.name) ? 'upgrade' : uninstallQueue.has(selectedPackage.name) ? 'remove' : undefined}
              disabled={operationDisabled}
              onSelectPackage={(name) => { const pkg = packages.find(p => p.name === name); if (pkg) { if (selectedPackage) setSidebarHistory(h => [...h, selectedPackage]); setSidebarViewOnly(false); setSidebarIncompatible(false); setSelectedPackage(pkg) } }}
              allPackages={packages}
              conflict={getConflict(selectedPackage)}
              isOsCompatible={selectedPackage.compatible}
              viewOnly={sidebarViewOnly || selectedPackage.hidden}
              showIncompatible={sidebarIncompatible}
              initialTab={detailTab}
              onSelectDeveloper={openDeveloper}
              isStarred={starred.has(selectedPackage.name)}
              onToggleStar={() => toggleStar(selectedPackage.name)}
              onBack={stepBackInDetail}
            />
          )}
      </PackageDetailModal>
      </>
      )}
    </>
  )
}
