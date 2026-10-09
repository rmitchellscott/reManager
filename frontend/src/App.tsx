import { ArrowsCounterClockwise, CircleNotch, Package, Warning, WarningCircle, WifiHigh, WifiSlash, Wrench } from '@phosphor-icons/react'
import { useState, useEffect, useLayoutEffect, useRef, useMemo, useCallback } from 'react'
import { Toaster, toast } from 'sonner'
import { applyThemeWithPortal } from './main'
import { BrowsePage } from '@/pages/BrowsePage'
import { RetryCountdown } from '@/components/RetryCountdown'
import { HookDialog } from '@/components/HookDialog'
import { cn, debugLog } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Tabs } from '@/components/ui/tabs'
import { TooltipProvider } from '@/components/ui/tooltip'
import { MainTabBar } from '@/components/MainTabBar'
import { useKeyboardVisible } from '@/hooks/useKeyboardVisible'
import { useCollapseOnScroll } from '@/hooks/useCollapseOnScroll'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { ProgressModal } from '@/components/ProgressModal'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { onSyncedChange } from '@/lib/syncEvents'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { VellumInstallSuccessDialog } from '@/components/VellumInstallSuccessDialog'
import { VellumUninstallSuccessDialog } from '@/components/VellumUninstallSuccessDialog'
import { SettingsDialog, type SettingsSection } from '@/components/SettingsDialog'
import { PageHeader } from '@/components/PageHeader'
import { SupportBundlePage } from '@/components/SupportBundlePage'
import type { SupportScreen } from '@/components/SupportHome'
import { DnsErrorModal } from '@/components/DnsErrorModal'
import { FilesystemRestoreErrorDialog } from '@/components/FilesystemRestoreErrorDialog'
import { ConnectPage } from '@/pages/ConnectPage'
import { AppProvider } from '@/contexts/AppContext'
import { useConnectionEvents } from '@/hooks/useConnectionEvents'
import { useBackHandler } from '@/hooks/useBackHandler'
import { useVellumEvents } from '@/hooks/useVellumEvents'
import { useOperation, type OperationKind } from '@/hooks/useOperation'
import { operationFailure } from '@/lib/operations'
import { useTransferEvents } from '@/hooks/useTransferEvents'
import { useWarnings } from '@/hooks/useWarnings'
import { SystemTab } from '@/tabs/SystemTab'
import { ModsTab, type OsSyncStatus, type PlanActions, type SelectPackageForOSFn } from '@/tabs/ModsTab'
import { PlanBarContent, PlanBarWash, type PlanBarModel } from '@/components/mods/PlanBar'
import { useTouchLayout } from '@/hooks/useMediaQuery'
import { upgradeCandidateMap, withoutUninstalled } from '@/lib/updates'
import { systemAlertLevel, pullsFocus } from '@/lib/alerts'
import { deriveSystemStatus } from '@/lib/systemStatus'
import { useFollowedDevs } from '@/hooks/useFollowedDevs'
import { deriveModsStatus, noPlanSolveFacts, type PlanSolveFacts } from '@/lib/modsStatus'
import { PackageInfo, MaintenanceCommandInfo, SystemTaskInfo, SSHKey, SavedDevice, PlatformCapabilities, Step, UpgradeCandidate, DeviceOutlook, ReenableStatus, ColorScheme, SyncKey, ErrorCode, type SettingsInfo } from '@/lib/types'
import { ConfigService, ConnectionService, DeviceService, FilesService, OSService, PackagesService, SystemService } from '@bindings/reManager'
import { onEvent } from '@/lib/events'

const STOP_PARTWAY_WARNING = 'Stopping partway can leave mods not working until this runs again.'
const VELLUM_STOP_WARNING = 'Stopping partway can leave Vellum broken until it is repaired.'

export default function App() {
  const [step, setStep] = useState<Step>('connect')
  const [sshAgentAvailable, setSSHAgentAvailable] = useState(false)
  // Defaults to desktop so a failed call leaves the user's own key files rather than an empty manager.
  const [capabilities, setCapabilities] = useState<PlatformCapabilities>({ managedKeys: true, localKeyFiles: true, sshAgent: true, updateCheck: true, keyboardResizesView: false, iPad: false, supportUpload: false, revealInFileManager: true })
  const [availableKeys, setAvailableKeys] = useState<SSHKey[]>([])
  const [device, setDevice] = useState<string>('')
  const [deviceInfo, setDeviceInfo] = useState<Record<string, string>>({})
  const [installedPackages, setInstalledPackages] = useState<Map<string, string>>(new Map())
  const [installTimes, setInstallTimes] = useState<Map<string, number>>(new Map())
  const [refreshingPackages, setRefreshingPackages] = useState(false)
  const [vellumInstalled, setVellumInstalled] = useState<boolean | null>(null)
  const {
    warningsChecked, setWarningsChecked,
    warningsCheckedRef,
    osMismatchDetected, setOsMismatchDetected,
    osMismatchDetectedRef,
    storedOsVersion, setStoredOsVersion,
    currentOsVersion, setCurrentOsVersion,
    compatibilityStatus, setCompatibilityStatus,
    hashtab, setHashtab,
    osChannel, setOsChannel,
    incompatibleWithLatestOS, setIncompatibleWithLatestOS,
    timezoneMismatch, setTimezoneMismatch,
    deviceTimezone, setDeviceTimezone,
    selectedTimezone, setSelectedTimezone,
    reenableStatus, setReenableStatus,
    updateServiceStatus, setUpdateServiceStatus,
    xochitlRunning, setXochitlRunning,
    xoviActive, setXoviActive,
    activeLauncher, setActiveLauncher,
    currentLauncher, setCurrentLauncher,
    guideStatus, setGuideStatus,
    showGuideRestartDialog, setShowGuideRestartDialog,
  } = useWarnings()

  const keyboardVisible = useKeyboardVisible()
  const { collapsed: tabsCollapsed, expand: expandTabs } = useCollapseOnScroll()

  useLayoutEffect(() => {
    window.scrollTo({ top: 0 })
    expandTabs()
  }, [step])

  const [activeTab, setActiveTab] = useState<'mods' | 'system'>('mods')
  const [modsLocked, setModsLocked] = useState(false)
  const freshDeviceRef = useRef(false)
  const autoSelectedRef = useRef(false)
  const [installQueue, setInstallQueue] = useState<Set<string>>(new Set())
  const [uninstallQueue, setUninstallQueue] = useState<Set<string>>(new Set())

  const [showSettingsDialog, setShowSettingsDialog] = useState(false)
  const [settingsSection, setSettingsSection] = useState<SettingsSection | undefined>(undefined)
  const openSettings = (section?: SettingsSection) => {
    setSettingsSection(section)
    setShowSettingsDialog(true)
  }
  const [showSoftwareManager, setShowSoftwareManager] = useState(false)
  const [showSupportBundles, setShowSupportBundles] = useState(false)
  const [supportScreen, setSupportScreen] = useState<SupportScreen>('report')
  const [appVersion, setAppVersion] = useState('dev')
  const [proxyMode, setProxyMode] = useState(true)
  const [suppressSystemFileWarnings, setSuppressSystemFileWarnings] = useState(false)
  const [preventSleep, setPreventSleep] = useState(true)
  const [theme, setTheme] = useState(() => localStorage.getItem('theme') || 'system')
  const [terminalTheme, setTerminalTheme] = useState('match')
  const [editorTheme, setEditorTheme] = useState('match')
  const [checkForUpdates, setCheckForUpdates] = useState(true)
  const [sshAgentSocketPath, setSSHAgentSocketPath] = useState('')
  const [systemPrefersDark, setSystemPrefersDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  const [dnsErrorShown, setDnsErrorShown] = useState(false)
  const [showDnsErrorModal, setShowDnsErrorModal] = useState(false)

  const [packages, setPackages] = useState<PackageInfo[]>([])
  // Null means the solve has not returned, which is not nothing to upgrade, so the count falls back to the catalog diff.
  const [upgradeCandidates, setUpgradeCandidates] = useState<Map<string, UpgradeCandidate> | null>(null)

  const refreshUpgradeCandidates = useCallback(async () => {
    try {
      const c = await PackagesService.GetUpgradeCandidates()
      setUpgradeCandidates(upgradeCandidateMap(c))
    } catch (err) {
      debugLog('GetUpgradeCandidates failed:', err)
      setUpgradeCandidates(null)
    }
  }, [])


  const { follows } = useFollowedDevs()
  // Loaded once the index is ready: a device with nothing cached looks the same as an index that has not arrived.
  const [deviceOutlooks, setDeviceOutlooks] = useState<Map<string, DeviceOutlook>>(new Map())

  const refreshDeviceOutlooks = useCallback(async () => {
    try {
      const outlooks = await ConnectionService.GetDeviceOutlooks()
      setDeviceOutlooks(new Map((outlooks || []).map(o => [o.deviceId, o])))
    } catch (err) {
      debugLog('GetDeviceOutlooks failed:', err)
    }
  }, [])

  // Empty is not unavailable: the catalog loads asynchronously at startup, so a browse view that opens first must not report it unavailable.
  const [catalogStatus, setCatalogStatus] = useState<'loading' | 'ready' | 'error'>('loading')

  useEffect(() => {
    const offLoaded = onEvent('metadata:loaded', () => {
      refreshDeviceOutlooks()
      // Once a device is connected the list is filtered for it; the unfiltered catalog would mark every package compatible.
      const connectedDevice = deviceRef.current
      const loadPackages = connectedDevice
        ? Promise.all([DeviceService.GetDeviceInfo(), DeviceService.GetDeviceArchitecture(connectedDevice)])
            .then(([info, arch]) => PackagesService.GetPackages(connectedDevice, info?.firmware || '', arch))
        : PackagesService.GetPackages('', '', '')
      // A deep link waits on this; an empty list would send it to a search for the name.
      loadPackages
        .then(pkgs => { if (pkgs?.length) setPackages(pkgs) })
        .catch(() => {})
        .finally(() => setCatalogStatus('ready'))
    })
    const offError = onEvent('metadata:error', () => setCatalogStatus('error'))
    PackagesService.CatalogLoadFailed()
      .then(failed => { if (failed) setCatalogStatus(prev => prev === 'loading' ? 'error' : prev) })
      .catch(() => {})
    return () => { offLoaded(); offError() }
  }, [refreshDeviceOutlooks])

  // RefreshMetadata does not emit metadata:loaded, so the unfiltered list is fetched here; success also recovers a catalog that failed at startup.
  const refreshBrowseCatalog = useCallback(async () => {
    await PackagesService.RefreshMetadata()
    setPackages(await PackagesService.GetPackages('', '', '') || [])
    setCatalogStatus('ready')
  }, [])

  // The browse page and the mods tab are never mounted together, so whichever is on screen answers the ref.
  const openPackageByNameRef = useRef<((name: string) => void) | null>(null)
  const [deepLinkedPackage, setDeepLinkedPackage] = useState('')

  // Pulled rather than carried on the event: a cold start hands the URL to Go before this webview exists; both paths call TakeDeepLink, which clears it.
  useEffect(() => {
    const take = () => {
      SystemService.TakeDeepLink()
        .then(link => { if (link?.target === 'package' && link.name) setDeepLinkedPackage(link.name) })
        .catch(() => {})
    }
    take()
    return onEvent('deeplink:received', take)
  }, [])

  // A cold start fetches the catalog at the same time, so the link usually arrives first; wait rather than report a package missing.
  useEffect(() => {
    if (!deepLinkedPackage || catalogStatus === 'loading') return
    if (step === 'connect') { setStep('browse'); return }
    if (step !== 'browse' && !modsLocked) setActiveTab('mods')
    const openPackage = openPackageByNameRef.current
    if (!openPackage) return
    openPackage(deepLinkedPackage)
    setDeepLinkedPackage('')
  }, [deepLinkedPackage, catalogStatus, step, activeTab, modsLocked])

  useBackHandler(() => setStep('connect'), step === 'browse')

  const [systemTasks, setSystemTasks] = useState<SystemTaskInfo[]>([])
  const [maintenanceCommands, setMaintenanceCommands] = useState<Record<string, MaintenanceCommandInfo[]>>({})

  const [savedDevices, setSavedDevices] = useState<SavedDevice[]>([])
  // Until the first load lands, an empty list looks like no devices and the connect step would show the add form first.
  const [initialDataLoaded, setInitialDataLoaded] = useState(false)
  const [connectedDeviceId, setConnectedDeviceId] = useState('')
  const [queueError, setQueueError] = useState<string | null>(null)

  const [commandOutputOpen, setCommandOutputOpen] = useState(false)
  const [guideRestartError, setGuideRestartError] = useState<{ message: string; detail: string } | null>(null)

  const systemStatus = deriveSystemStatus({
    installedPackages,
    maintenanceCommands,
    systemTasks,
    vellumInstalled,
    updateService: updateServiceStatus,
    xochitlRunning,
    xoviActive,
    activeLauncher,
    currentLauncher,
    hashtab,
    osMismatchDetected,
    osChannel,
    incompatibleWithLatestOS,
    reenableStatus,
    timezone: timezoneMismatch,
    guide: guideStatus,
  })
  const [upgradeQueue, setUpgradeQueue] = useState<Set<string>>(new Set())


  useEffect(() => {
    if (!upgradeCandidates) return
    setUpgradeQueue(prev => {
      const stillOffered = new Set([...prev].filter(name => upgradeCandidates.has(name)))
      return stillOffered.size === prev.size ? prev : stillOffered
    })
  }, [upgradeCandidates])

  const [planSolve, setPlanSolve] = useState<PlanSolveFacts>(noPlanSolveFacts)
  const [planOpen, setPlanOpen] = useState(false)
  const [planBar, setPlanBar] = useState<PlanBarModel | null>(null)
  const [osSyncStatus, setOsSyncStatus] = useState<OsSyncStatus | null>(null)
  const planActionsRef = useRef<PlanActions | null>(null)
  const touchLayout = useTouchLayout()

  useEffect(() => {
    if (modsLocked && activeTab === 'mods') setActiveTab('system')
  }, [modsLocked, activeTab])

  const shownPlanBar = planBar && (activeTab === 'mods' || planBar.everyTab) ? planBar : null
  const planBarClearance = shownPlanBar ? (touchLayout ? '72px' : '64px') : '0px'
  // Kept after the strip leaves so it slides away showing what it said.
  const lastStripModel = useRef<PlanBarModel | null>(null)
  if (shownPlanBar) lastStripModel.current = shownPlanBar
  const stripModel = shownPlanBar ?? lastStripModel.current
  const stripShown = !!shownPlanBar && !planOpen
  const modsStatus = useMemo(() => deriveModsStatus({
    packages, installedPackages, upgradeCandidates, follows, installQueue, upgradeQueue, uninstallQueue, solve: planSolve,
  }), [packages, installedPackages, upgradeCandidates, follows, installQueue, upgradeQueue, uninstallQueue, planSolve])

  const deviceRef = useRef(device)
  deviceRef.current = device
  const installedPackagesRef = useRef(installedPackages)
  installedPackagesRef.current = installedPackages

  const resolvedAppTheme = useMemo((): 'dark' | 'light' => {
    if (theme === 'system') {
      return systemPrefersDark ? 'dark' : 'light'
    }
    return theme === 'dark' ? 'dark' : 'light'
  }, [theme, systemPrefersDark])

  const resolvedTerminalTheme = useMemo((): 'dark' | 'light' => {
    if (terminalTheme === 'match') {
      return resolvedAppTheme
    }
    return terminalTheme === 'dark' ? 'dark' : 'light'
  }, [terminalTheme, resolvedAppTheme])

  const resolvedEditorTheme = useMemo((): 'dark' | 'light' => {
    if (editorTheme === 'match') {
      return resolvedAppTheme
    }
    return editorTheme === 'dark' ? 'dark' : 'light'
  }, [editorTheme, resolvedAppTheme])

  const rescanAllPackages = async () => {
    try {
      const installed = await PackagesService.GetInstalledPackagesWithVersions()
      const versions = new Map<string, string>()
      const times = new Map<string, number>()
      for (const [name, val] of Object.entries(installed || {})) {
        const parts = (val as string).split('|')
        versions.set(name, parts[0])
        if (parts[1]) times.set(name, parseInt(parts[1], 10))
      }
      const installedBefore = installedPackagesRef.current
      setInstalledPackages(versions)
      setInstallTimes(times)
      setUpgradeCandidates(prev => withoutUninstalled(prev, installedBefore, versions))
      void refreshUpgradeCandidates()
      return true
    } catch (err) {
      console.error('Failed to rescan package status:', err)
      return false
    }
  }

  const resetOperationStateRef = useRef<() => void>(() => {})
  const restorePendingPromptRef = useRef<() => Promise<void>>(async () => {})
  const resetConnectChecksRef = useRef<() => void>(() => {})
  const refreshDeviceStateRef = useRef<(deviceType: string) => Promise<void>>(async () => {})
  // The connect checks may land before the restore does, so only their absence is waited on; they were cleared when the connection dropped.
  const handleConnectionRestored = useCallback(async (deviceType: string) => {
    const checks = warningsCheckedRef.current ? Promise.resolve() : waitForConnectChecks()
    resetOperationStateRef.current()
    void restorePendingPromptRef.current()
    if (deviceType) {
      setDevice(deviceType)
    }
    const reload = (async () => {
      await refreshDeviceStateRef.current(deviceType)
      await rescanAllPackages()
    })()
    await Promise.all([reload, checks])
  }, [])

  const handleConnectionLost = useCallback(() => {
    resetOperationStateRef.current()
    resetConnectChecksRef.current()
  }, [])

  const {
    connectionStatus,
    setConnectionStatus,
    restoreSession,
    reconnectAttempt,
    setReconnectAttempt,
    reconnectMaxAttempts,
    retryPhase,
    connectionErrorCode,
    connectionError,
    connectionErrorDetails,
    reportConnectionFailure,
    showFilesystemRestoreError,
    isRetryingFilesystemRestore,
    writableRootBusy,
    setWritableRootBusy,
    handleRetryRestore: handleFilesystemRestoreRetry,
    handleDismissRestoreError: handleFilesystemRestoreDismiss,
  } = useConnectionEvents({
    onConnectionRestored: handleConnectionRestored,
    onConnectionLost: handleConnectionLost,
    onUnsavedConnectionFailed: () => handleDisconnect(),
    deviceRef,
  })

  // A dropped link usually recovers in a second; a failed connection is terminal, so it shows at once.
  // Acknowledged per version for the session, not persisted: a reboot into the other partition can land on a different beta.
  const [betaAcknowledgedVersion, setBetaAcknowledgedVersion] = useState('')
  const betaModalOpen = warningsChecked && systemStatus.osPrerelease && osChannel?.activeVersion !== betaAcknowledgedVersion
  const acknowledgeBeta = () => setBetaAcknowledgedVersion(osChannel?.activeVersion ?? '')

  const disconnected = step !== 'connect' && step !== 'browse' && connectionStatus !== 'connected'
  // The backend message restates the dialog title, so drop that part.
  const connectionDetail = (connectionError || '').replace(/^connection (failed|lost):\s*/i, '')
    || 'Could not reach your reMarkable.'
  // Unsaved connections are disconnected on failure before this point, so a saved id is always present here.
  const [manualReconnecting, setManualReconnecting] = useState(false)
  // Disconnect stays available mid-reconnect, so the abandoned attempt must not land on the connect page.
  const reconnectAbandonedRef = useRef(false)
  const handleReconnect = async () => {
    if (!connectedDeviceId) return
    reconnectAbandonedRef.current = false
    setManualReconnecting(true)
    try {
      const result = await ConnectionService.ConnectToSavedDevice(connectedDeviceId)
      if (reconnectAbandonedRef.current) return
      if (result.success) {
        await restoreSession(result.device || 'unknown')
      } else {
        reportConnectionFailure(result.code ? result : result.message)
      }
    } catch (err) {
      if (!reconnectAbandonedRef.current) reportConnectionFailure(err)
    } finally {
      setManualReconnecting(false)
    }
  }

  const [connectionModalVisible, setConnectionModalVisible] = useState(false)
  useEffect(() => {
    if (!disconnected) { setConnectionModalVisible(false); return }
    if (connectionStatus === 'failed') { setConnectionModalVisible(true); return }
    const timer = window.setTimeout(() => setConnectionModalVisible(true), 1500)
    return () => window.clearTimeout(timer)
  }, [disconnected, connectionStatus])

  const [shellRunning, setShellRunning] = useState(false)

  const [guideAcknowledged, setGuideAcknowledged] = useState(false)
  useEffect(() => {
    if (activeTab !== 'system' || !guideStatus?.unseen || guideAcknowledged) return
    setGuideAcknowledged(true)
    FilesService.MarkGuideSeen().catch(() => {})
  }, [activeTab, guideStatus, guideAcknowledged])

  // Package updates are deliberately absent: the Mods tab carries that count.
  const systemAlert = systemAlertLevel(
    warningsChecked ? Object.values(systemStatus.rows) : [],
    { guideNews: !!guideStatus?.unseen && !guideAcknowledged, shellRunning },
  )

  // Only on a first look at a tablet; moving the user mid-session because a check flipped would be wrong. Info does not qualify.
  useEffect(() => {
    if (!warningsChecked || !freshDeviceRef.current || autoSelectedRef.current) return
    autoSelectedRef.current = true
    if (pullsFocus(systemAlert)) {
      setActiveTab('system')
    }
  }, [warningsChecked, systemAlert])

  const {
    vellumBrokenInstall,
    setVellumBrokenInstall,
  } = useVellumEvents({
    setVellumInstalled,
  })

  const checkDnsProxyError = async (dnsError: boolean) => {
    if (!dnsError || dnsErrorShown) return
    const currentProxyMode = await SystemService.GetSettings().then(s => s.proxyMode)
    if (!currentProxyMode) {
      setShowDnsErrorModal(true)
      setDnsErrorShown(true)
    }
  }

  const selectedTimezoneRef = useRef(selectedTimezone)
  selectedTimezoneRef.current = selectedTimezone

  // A status the backend could not read rejects; the row keeps what it last showed rather than a default the tablet never reported.
  const refreshXochitlStatus = async () => {
    const status = await DeviceService.GetXochitlStatus().catch(() => null)
    if (!status) return
    setXochitlRunning(status.running)
    setXoviActive(status.xoviActive)
    setActiveLauncher(status.activeLauncher || '')
    setCurrentLauncher(status.currentLauncher || '')
  }

  const refreshHashtabStatus = async () => {
    const status = await DeviceService.CheckHashtabVersion().catch(() => null)
    if (status) setHashtab(status)
  }

  // The update service can take seconds to settle after a task flips it, so poll until enabled and running agree.
  const refreshDeviceStatus = async () => {
    const tzStatus = await DeviceService.GetTimezoneStatus().catch(() => null)
    let updateStatus = await DeviceService.GetUpdateServiceStatus().catch(() => null)
    for (let i = 0; i < 6 && updateStatus && updateStatus.enabled !== updateStatus.running; i++) {
      await new Promise(r => setTimeout(r, 500))
      updateStatus = await DeviceService.GetUpdateServiceStatus().catch(() => null)
    }
    if (updateStatus) setUpdateServiceStatus(updateStatus)
    await refreshXochitlStatus()
    await refreshHashtabStatus()
    if (!tzStatus) return
    setTimezoneMismatch(tzStatus.needsUpdate ? tzStatus : null)
    if (tzStatus.deviceTimezone) {
      setDeviceTimezone(tzStatus.deviceTimezone)
      if (!selectedTimezoneRef.current) {
        setSelectedTimezone(tzStatus.savedTimezone || tzStatus.deviceTimezone)
      }
    }
  }

  const refreshAfterPackageChange = async (dnsError: boolean) => {
    await checkDnsProxyError(dnsError)
    await rescanAllPackages()
    if (osMismatchDetectedRef.current) {
      PackagesService.GetPackageCompatibilityStatus().then(setCompatibilityStatus).catch(() => {})
    }
    await refreshHashtabStatus()
    await refreshXochitlStatus()
  }

  // An upgrade can move the OS the catalog is filtered against, so device info and the catalog are read before the installed list.
  const refreshAfterUpgrade = async (ok: boolean, dnsError: boolean) => {
    if (ok) {
      try {
        const info = await DeviceService.GetDeviceInfo()
        setDeviceInfo(Object.fromEntries(Object.entries(info ?? {}).map(([k, v]) => [k, v ?? ''])))
        const arch = await DeviceService.GetDeviceArchitecture(deviceRef.current)
        const filteredPkgs = await PackagesService.GetPackages(deviceRef.current, info?.firmware || '', arch)
        setPackages(filteredPkgs || [])
      } catch (err) {
        debugLog('Failed to refresh device info after upgrade:', err)
      }
    }
    // apk can upgrade some packages before it fails, so a failed run still leaves installed versions to re-read.
    await rescanAllPackages()
    await refreshXochitlStatus()
    await checkDnsProxyError(dnsError)
  }

  const {
    operation,
    hookRun,
    prompt: operationPrompt,
    busy: operationBusy,
    start: startOperation,
    stop: stopOperation,
    stopHook,
    abandon: abandonOperation,
    respond: respondToOperation,
    close: closeOperation,
    reset: resetOperation,
    restorePendingPrompt,
  } = useOperation()
  restorePendingPromptRef.current = restorePendingPrompt
  // The plan runs its phases outside the operation hook, so it reports its own run here for everything that waits on a tablet command.
  const [planApplying, setPlanApplying] = useState(false)
  const commandRunning = operationBusy || planApplying
  // A vellum run clears itself when it ends, so a failure is kept here for the surface it ran on.
  const [vellumFailure, setVellumFailure] = useState<{ target: string; message: string; output: string } | null>(null)
  const [vellumInstalledDialog, setVellumInstalledDialog] = useState<'installed' | 'reinstalled' | null>(null)
  const [showVellumUninstalled, setShowVellumUninstalled] = useState(false)

  const { transfers, lastFinished: lastTransferFinished, transferError, setTransferError } = useTransferEvents()

  useEffect(() => {
    const loadInitialData = async () => {
      try {
        const [keys, pkgs, tasks, devices, version, settings, agentAvail, capabilities] = await Promise.all([
          ConnectionService.GetDefaultSSHKeys(),
          PackagesService.GetPackages('', '', ''),
          PackagesService.GetSystemTasksInfo(),
          ConnectionService.GetSavedDevices(),
          SystemService.GetAppVersion(),
          SystemService.GetSettings(),
          ConnectionService.IsSSHAgentAvailable().catch(() => false),
          SystemService.GetPlatformCapabilities().catch(() => ({ managedKeys: true, localKeyFiles: true, sshAgent: true, updateCheck: true, keyboardResizesView: false, iPad: false, supportUpload: false, revealInFileManager: true })),
        ])

        setAvailableKeys(keys || [])
        setSSHAgentAvailable(!!agentAvail)
        setCapabilities(capabilities)

        // Asked at mount, this can land after metadata:loaded's list and must not replace it with the empty one.
        if (pkgs?.length) {
          setPackages(pkgs)
          setCatalogStatus('ready')
        }
        setSystemTasks(tasks || [])
        setSavedDevices(devices || [])
        refreshDeviceOutlooks()
        setAppVersion(version || 'dev')
        setProxyMode(settings?.proxyMode ?? true)
        setSuppressSystemFileWarnings(settings?.suppressSystemFileWarnings ?? false)
        setPreventSleep(settings?.preventSleep ?? true)
        const loadedTheme = settings?.theme || 'system'
        setTheme(loadedTheme)
        localStorage.setItem('theme', loadedTheme)
        setTerminalTheme(settings?.terminalTheme || 'match')
        setEditorTheme(settings?.editorTheme || 'match')
        const shouldCheckUpdates = settings?.checkForUpdates ?? true
        setCheckForUpdates(shouldCheckUpdates)
        setSSHAgentSocketPath(settings?.sshAgentSocketPath || '')

        SystemService.IsOutsideApplicationsFolder().catch(() => false).then((outside) => {
          if (outside) {
            toast.warning('Move reManager to Applications', {
              description: 'Otherwise macOS may ask for permissions again on every launch.',
              action: {
                label: 'Move',
                onClick: () => SystemService.MoveToApplications().catch((err) => {
                  toast.error(handleError(err, 'Could not move reManager. Drag it into Applications instead.'))
                }),
              },
              duration: Infinity,
              closeButton: true,
            })
            return
          }
          if (!shouldCheckUpdates) return
          SystemService.CheckForAppUpdate().then((result) => {
            if (result.updateAvailable && result.latestVersion) {
              toast.info(`Update available: ${result.latestVersion}`, {
                description: `You're running ${result.currentVersion}`,
                action: {
                  label: 'View release',
                  onClick: () => SystemService.OpenExternalURL(result.releaseURL)
                },
                duration: 6000,
              })
            }
          }).catch((err) => {
            debugLog('Update check failed:', err)
          })
        })

        const current = await ConnectionService.GetCurrentConnection().catch(() => null)
        if (current) {
          setConnectedDeviceId(current.deviceId)
          setDevice(current.deviceType)
          const checks = waitForConnectChecks()
          void ConnectionService.ResendConnectState()
          await refreshDeviceStateRef.current(current.deviceType)
          await checks
          setStep('select')
        }
      } catch (err) {
        debugLog('Could not load initial data:', err)
      } finally {
        setInitialDataLoaded(true)
      }
    }
    loadInitialData()
  }, [])

  useEffect(() => {
    const offSettings = onSyncedChange(SyncKey.SettingsSyncKey, async () => {
      const settings = await SystemService.GetSettings()
      setProxyMode(settings.proxyMode)
      setSuppressSystemFileWarnings(settings.suppressSystemFileWarnings)
      const syncedTheme = settings.theme || 'system'
      setTheme(syncedTheme)
      localStorage.setItem('theme', syncedTheme)
      setTerminalTheme(settings.terminalTheme || 'match')
      setEditorTheme(settings.editorTheme || 'match')
    })
    const offDevices = onSyncedChange(SyncKey.DevicesSyncKey, async () => {
      setSavedDevices(await ConnectionService.GetSavedDevices() || [])
      refreshDeviceOutlooks()
    })
    const offDeviceInfo = onSyncedChange(SyncKey.DeviceInfoSyncKey, refreshDeviceOutlooks)
    return () => {
      offSettings()
      offDevices()
      offDeviceInfo()
    }
  }, [refreshDeviceOutlooks])

  useEffect(() => {
    void applyThemeWithPortal(theme)
  }, [theme])

  useEffect(() => {
    SystemService.GetSystemColorScheme().then(scheme => {
      if (scheme !== ColorScheme.ColorSchemeUnknown) {
        setSystemPrefersDark(scheme === ColorScheme.ColorSchemeDark)
      }
    }).catch(() => {})

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const handleChange = (e: MediaQueryListEvent) => setSystemPrefersDark(e.matches)
    mediaQuery.addEventListener('change', handleChange)

    let unsubscribeTheme: (() => void) | undefined
    {
      unsubscribeTheme = onEvent('system-theme-changed', scheme => {
        setSystemPrefersDark(scheme === ColorScheme.ColorSchemeDark)
      })
    }

    return () => {
      mediaQuery.removeEventListener('change', handleChange)
      unsubscribeTheme?.()
    }
  }, [])

  useEffect(() => {
    if (activeTab === 'system' && step !== 'connect') {
      fetchUpdateServiceStatus()
    }
  }, [activeTab, step])

  useEffect(() => {
    if (connectionStatus !== 'connected' || commandRunning) return
    const interval = window.setInterval(() => {
      DeviceService.GetXochitlStatusQuick().then(s => { setXochitlRunning(s.running); setXoviActive(s.xoviActive) }).catch(() => {})
    }, 60000)
    return () => clearInterval(interval)
  }, [connectionStatus, commandRunning])

  const resetOperationState = () => {
    setInstallQueue(new Set())
    setUninstallQueue(new Set())
    setUpgradeQueue(new Set())
    resetOperation()
    setVellumFailure(null)
    setVellumBrokenInstall(null)
    reportConnectionFailure(null)
    setReconnectAttempt(0)
    setWritableRootBusy(false)
    setXochitlRunning(true)
  }
  resetOperationStateRef.current = resetOperationState

  const resetConnectChecks = () => {
    setHashtab(null)
    setOsChannel(null)
    setIncompatibleWithLatestOS([])
    setOsMismatchDetected(false)
    osMismatchDetectedRef.current = false
    setCompatibilityStatus(null)
    setStoredOsVersion('')
    setCurrentOsVersion('')
    setReenableStatus(ReenableStatus.$zero)
    setUpdateServiceStatus({ enabled: false, running: false })
    setTimezoneMismatch(null)
    setDeviceTimezone('')
    setSelectedTimezone('')
    setXochitlRunning(true)
    setXoviActive(false)
    setActiveLauncher('')
    setCurrentLauncher('')
    setWarningsChecked(false)
    warningsCheckedRef.current = false
  }
  resetConnectChecksRef.current = resetConnectChecks

  // A ceiling on the blocking connect checks, not a point to paint anyway: a dozen SSH round trips took 7.3s on a Paper Pro Move over Wi-Fi and older devices are slower.
  const CONNECT_CHECKS_TIMEOUT_MS = 60000

  const waitForConnectChecks = () => new Promise<void>((resolve, reject) => {

    let settled = false
    const settle = (fn: () => void) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      unsubscribe?.()
      fn()
    }

    const timer = window.setTimeout(
      () => settle(() => reject(new Error('Your reMarkable did not finish its checks in time.'))),
      CONNECT_CHECKS_TIMEOUT_MS,
    )
    const unsubscribe = onEvent('connect:warnings', () => settle(resolve))
  })

  // Subscribes before the refresh because the checks run alongside and would land while nothing was listening.
  const handleConnected = async (deviceType: string, freshDevice: boolean) => {
    freshDeviceRef.current = freshDevice
    autoSelectedRef.current = false
    const checks = waitForConnectChecks()
    await refreshDeviceState(deviceType)
    await checks
  }

  const refreshDeviceState = async (deviceType: string) => {
    const info = await DeviceService.GetDeviceInfo()
    setDeviceInfo(Object.fromEntries(Object.entries(info ?? {}).map(([k, v]) => [k, v ?? ''])))

    const arch = await DeviceService.GetDeviceArchitecture(deviceType)
    const filteredPkgs = await PackagesService.GetPackages(deviceType, info?.firmware || '', arch)
    setPackages(filteredPkgs || [])

    const installedResult = await PackagesService.GetInstalledPackagesWithOsCheck().catch(() => null)
    debugLog('refreshDeviceState: GetInstalledPackagesWithOsCheck result:', installedResult)
    const installed = await PackagesService.GetInstalledPackagesWithVersions().catch(() => null)
    if (installed) {
      const versions = new Map<string, string>()
      const times = new Map<string, number>()
      for (const [name, val] of Object.entries(installed)) {
        const parts = (val as string).split('|')
        versions.set(name, parts[0])
        if (parts[1]) times.set(name, parseInt(parts[1], 10))
      }
      setInstalledPackages(versions)
      setInstallTimes(times)
      void refreshUpgradeCandidates()
    }

    const maintCmds = await PackagesService.GetAllMaintenanceCommands()
    setMaintenanceCommands(Object.fromEntries(Object.entries(maintCmds || {}).map(([k, v]) => [k, v ?? []])))
  }
  refreshDeviceStateRef.current = refreshDeviceState

  // The page resets before the backend answers, so a command stuck on the reMarkable cannot hold the app on a stale page.
  const handleDisconnect = async () => {
    reconnectAbandonedRef.current = true
    const disconnecting = ConnectionService.Disconnect()
    resetOperationState()
    resetConnectChecks()
    setDeviceInfo({})
    setInstalledPackages(new Map())
    setVellumInstalled(null)
    setStep('connect')
    setDevice('')
    setConnectedDeviceId('')
    setConnectionStatus('connected')
    setGuideStatus(null)
    setGuideAcknowledged(false)
    setManualReconnecting(false)

    await disconnecting.catch(() => {})
    refreshDeviceOutlooks()
    const devices = await ConnectionService.GetSavedDevices()
    setSavedDevices(devices || [])
  }

  const settingsValues: SettingsInfo = {
    proxyMode, suppressSystemFileWarnings, preventSleep,
    theme, terminalTheme, editorTheme, checkForUpdates, sshAgentSocketPath,
  }

  const showSavedSettings = async () => {
    const saved = await SystemService.GetSettings().catch(() => null)
    if (!saved) return
    setProxyMode(saved.proxyMode)
    setSuppressSystemFileWarnings(saved.suppressSystemFileWarnings)
    setPreventSleep(saved.preventSleep)
    setTheme(saved.theme || 'system')
    localStorage.setItem('theme', saved.theme || 'system')
    setTerminalTheme(saved.terminalTheme || 'match')
    setEditorTheme(saved.editorTheme || 'match')
    setCheckForUpdates(saved.checkForUpdates)
    setSSHAgentSocketPath(saved.sshAgentSocketPath || '')
  }

  const handleUpdateSettings = async (patch: Partial<SettingsInfo>) => {
    const turnedOnUpdateCheck = patch.checkForUpdates === true && !checkForUpdates

    if (patch.proxyMode !== undefined) setProxyMode(patch.proxyMode)
    if (patch.suppressSystemFileWarnings !== undefined) setSuppressSystemFileWarnings(patch.suppressSystemFileWarnings)
    if (patch.preventSleep !== undefined) setPreventSleep(patch.preventSleep)
    if (patch.terminalTheme !== undefined) setTerminalTheme(patch.terminalTheme)
    if (patch.editorTheme !== undefined) setEditorTheme(patch.editorTheme)
    if (patch.checkForUpdates !== undefined) setCheckForUpdates(patch.checkForUpdates)
    if (patch.sshAgentSocketPath !== undefined) setSSHAgentSocketPath(patch.sshAgentSocketPath)
    if (patch.theme !== undefined) {
      setTheme(patch.theme)
      localStorage.setItem('theme', patch.theme)
    }

    try {
      await SystemService.UpdateSettings(patch)
    } catch (err) {
      toast.error(handleError(err, 'Could not save settings.'))
      await showSavedSettings()
      return
    }

    if (patch.sshAgentSocketPath !== undefined) {
      const agentAvail = await ConnectionService.IsSSHAgentAvailable().catch(() => false)
      setSSHAgentAvailable(!!agentAvail)
    }

    if (turnedOnUpdateCheck) {
      SystemService.CheckForAppUpdate().then((result) => {
        if (result.updateAvailable && result.latestVersion) {
          toast.info(`Update available: ${result.latestVersion}`, {
            description: `You're running ${result.currentVersion}`,
            action: {
              label: 'View release',
              onClick: () => SystemService.OpenExternalURL(result.releaseURL)
            },
            duration: 6000,
          })
        }
      }).catch(() => {})
    }
  }

  const handleEnableProxyModeFromModal = async () => {
    setProxyMode(true)
    try {
      await SystemService.UpdateSettings({ proxyMode: true })
    } catch (err) {
      toast.error(handleError(err, 'Could not save settings.'))
      await showSavedSettings()
    }
    setShowDnsErrorModal(false)
  }

  const runVellum = async (target: string, title: string, run: (id: string) => Promise<unknown>) => {
    setVellumFailure(null)
    let output = ''
    const result = await startOperation({
      kind: 'vellum',
      target,
      title,
      background: true,
      reportsOwnFailure: true,
      onOutput: text => { output += text },
      run,
    })
    if (!result.ok && !result.canceled) setVellumFailure({ target, message: result.error ?? '', output })
    return result
  }

  const handleInstallVellum = async (target: 'install' | 'repair') => {
    if (commandRunning) return
    const result = await runVellum(target, target === 'repair' ? 'Repairing Vellum' : 'Installing Vellum', id => PackagesService.BootstrapVellum(id))
    if (!result.ok) return
    setVellumInstalled(true)
    setVellumInstalledDialog(target === 'repair' ? 'reinstalled' : 'installed')
    void rescanAllPackages()
  }

  const handleUninstallVellum = async (removeAllPackages: boolean) => {
    const result = await runVellum('uninstall', 'Uninstalling Vellum', id => PackagesService.UninstallVellum(id, removeAllPackages))
    if (!result.ok) return
    setVellumInstalled(false)
    setInstalledPackages(new Map())
    setShowSettingsDialog(false)
    setShowVellumUninstalled(true)
  }

  const handleCleanupVellum = async () => {
    if (commandRunning) return
    const result = await runVellum('cleanup', 'Removing the broken Vellum install', id => PackagesService.CleanupBrokenVellum(id))
    if (!result.ok) {
      if (!result.canceled) toast.error(handleError(operationFailure(result), 'Could not remove the broken Vellum install.'))
      return
    }
    setVellumBrokenInstall(null)
    setVellumInstalled(false)
  }

  const selectPackageForOSRef = useRef<SelectPackageForOSFn | null>(null)

  const handleSelectPackageForOS = async (name: string, targetOS: string, isCompatible: boolean = true) => {
    if (selectPackageForOSRef.current) {
      await selectPackageForOSRef.current(name, targetOS, isCompatible)
    }
  }

  const handleTabChange = async (value: 'mods' | 'system') => {
    if (value === 'mods' && modsLocked) return
    setActiveTab(value)
    window.scrollTo({ top: 0 })
    expandTabs()
    if (value === 'mods') {
      if (connectionStatus === 'connected' && packages.length === 0) {
        try {
          await PackagesService.RefreshMetadata()
          await refreshDeviceStateRef.current(device)
        } catch (err) {
          debugLog('Failed to reload mod catalog:', err)
        }
      } else {
        await rescanAllPackages()
      }
    }
    if (connectionStatus === 'connected' && !commandRunning) {
      DeviceService.GetXochitlStatus().then(s => { setXochitlRunning(s.running); setXoviActive(s.xoviActive); setActiveLauncher(s.activeLauncher || ''); setCurrentLauncher(s.currentLauncher || '') }).catch(() => {})
    }
  }

  const handleRunReenable = async () => {
    const result = await startOperation({
      kind: 'reenable',
      title: 'Re-enabling system partition mods',
      run: id => PackagesService.RunReenable(id),
    })
    if (!result.ok) return
    setReenableStatus(result.reenable || ReenableStatus.ReenableOK)
    await rescanAllPackages()
  }

  // The rescan runs under the guide's kind so the guide row stays busy through it.
  const handleInstallGuide = async () => {
    setSettlingOperation({ kind: 'guide', target: '' })
    try {
      await installGuideAndRefreshLibrary()
    } finally {
      setSettlingOperation(null)
    }
  }

  const installGuideAndRefreshLibrary = async () => {
    const installed = await startOperation({
      kind: 'guide',
      title: 'Install the reManager guide',
      background: true,
      reportsOwnFailure: true,
      run: id => FilesService.InstallUserGuide(id),
    })
    if (!installed.ok) {
      if (!installed.canceled) toast.error(handleError(operationFailure(installed), 'Could not install the reManager guide.'))
      return
    }
    setGuideStatus(prev => prev && { ...prev, installed: true, needsUpdate: false })
    if (!installedPackages.has('librarian') || !xoviActive) {
      setShowGuideRestartDialog(true)
      return
    }
    const rescanned = await startOperation({
      kind: 'guide',
      target: 'rescan',
      title: 'Rescan the library',
      background: true,
      reportsOwnFailure: true,
      run: id => ConfigService.RescanLibrary(id),
    })
    if (!rescanned.ok) {
      console.error('Failed to rescan library after guide install:', rescanned.error)
      setShowGuideRestartDialog(true)
    }
  }

  const handleFilesystemRestoreReboot = async () => {
    const result = await startOperation({
      kind: 'reboot',
      title: 'Reboot',
      background: true,
      run: id => OSService.RebootDevice(id),
    })
    if (result.ok) handleFilesystemRestoreDismiss()
  }

  const handleTimezoneChange = async (timezone: string) => {
    setSelectedTimezone(timezone)
    try {
      await DeviceService.SaveDeviceTimezone(timezone)
      const status = await DeviceService.GetTimezoneStatus()
      if (status.needsUpdate) {
        setTimezoneMismatch(status)
      } else {
        setTimezoneMismatch(null)
      }
    } catch (err) {
      console.error('Failed to save timezone:', err)
    }
  }

  const handleSetTimezone = async () => {
    if (!selectedTimezone) return
    const result = await startOperation({
      kind: 'timezone',
      title: 'Set timezone',
      background: true,
      run: id => DeviceService.SetDeviceTimezone(id, selectedTimezone, device),
    })
    if (result.ok) {
      setTimezoneMismatch(null)
      setDeviceTimezone(result.value || selectedTimezone)
    }
    const status = await DeviceService.GetTimezoneStatus().catch(() => null)
    if (status?.needsUpdate) setTimezoneMismatch(status)
  }

  const handleOsSynced = useCallback(() => {
    setOsMismatchDetected(false)
    osMismatchDetectedRef.current = false
    setCompatibilityStatus(null)
  }, [setOsMismatchDetected, osMismatchDetectedRef, setCompatibilityStatus])

  const openOSManager = useCallback(() => {
    setActiveTab('system')
    setShowSoftwareManager(true)
  }, [])

  const fetchUpdateServiceStatus = async () => {
    try {
      const status = await DeviceService.GetUpdateServiceStatus()
      setUpdateServiceStatus(status)
    } catch (err) {
      console.error('Failed to fetch update service status:', err)
    }
  }

  // The refresh after the run can take seconds; the row keeps showing the run until then rather than flashing the state it started from.
  const [settlingOperation, setSettlingOperation] = useState<{ kind: OperationKind; target: string } | null>(null)

  const handleSystemTask = async (taskId: string) => {
    setSettlingOperation({ kind: 'system-task', target: taskId })
    try {
      await startOperation({
        kind: 'system-task',
        target: taskId,
        title: systemTasks.find(task => task.id === taskId)?.label ?? 'System task',
        background: true,
        run: id => PackagesService.RunSystemTask(id, taskId, device),
      })
      await refreshDeviceStatus()
    } finally {
      setSettlingOperation(null)
    }
  }

  const handleComponentMaintenance = async (componentId: string, commandId: string) => {
    const cmd = maintenanceCommands[componentId]?.find(c => c.id === commandId)
    if (!cmd) return
    const target = `${componentId}:${commandId}`
    setSettlingOperation({ kind: 'command', target })
    try {
      await startOperation({
        kind: 'command',
        target,
        title: cmd.progressLabel || cmd.label,
        expectedSeconds: cmd.expectedSeconds ?? 0,
        showOutput: !!cmd.showOutput,
        canStop: cmd.allowStop,
        // A hook's own run carries the progress, so the command around it stays on its row until the hook starts one.
        background: !!cmd.hook,
        run: id => PackagesService.RunMaintenanceCommand(id, componentId, commandId, device),
      })
      await refreshDeviceStatus()
    } finally {
      setSettlingOperation(null)
    }
  }

  const vellumRun = operationBusy && operation?.kind === 'vellum' ? operation : null
  const vellumSurface = (target: string) => {
    if (vellumRun?.target === target) return { running: true, stopping: vellumRun.status === 'stopping', output: vellumRun.output, error: null }
    if (vellumFailure?.target === target) return { running: false, stopping: false, output: vellumFailure.output, error: vellumFailure.message }
    return null
  }
  const vellumInstall = vellumSurface('install')
  const vellumRepair = vellumSurface('repair')
  const vellumUninstall = vellumSurface('uninstall')

  const operationOnModal = operation && (!operation.background || operation.status === 'failed') ? operation : null
  const showingProgress = !!hookRun || !!operationOnModal || !!vellumUninstall

  const runningOperation = operationBusy ? operation : settlingOperation
  const [runningComponentId, runningCommandId] = runningOperation?.kind === 'command'
    ? runningOperation.target.split(':')
    : [undefined, undefined]

  const appContextValue = useMemo(() => ({
    device,
    deviceInfo,
    connectionStatus,
    connectionError,
    connectedDeviceId,
    installedPackages,
    setInstalledPackages,
    installTimes,
    packages,
    setPackages,
    vellumInstalled,
    setVellumInstalled,
    commandRunning,
    runningOperation,
    startOperation,
    writableRootBusy,
    transfers,
    lastTransferFinished,
    transferError,
    setTransferError,
    appVersion,
    resolvedAppTheme,
    resolvedTerminalTheme,
    resolvedEditorTheme,
    suppressSystemFileWarnings,
    refreshDeviceState,
    rescanAllPackages,
    checkDnsProxyError,
    refreshAfterPackageChange,
    refreshAfterUpgrade,
  }), [
    device, deviceInfo, connectionStatus, connectionError, connectedDeviceId,
    installedPackages, installTimes, packages, vellumInstalled,
    commandRunning, runningOperation, startOperation, writableRootBusy,
    transfers, lastTransferFinished, transferError,
    appVersion, resolvedAppTheme, resolvedTerminalTheme, resolvedEditorTheme,
    suppressSystemFileWarnings,
  ])

  return (
    <AppProvider value={appContextValue}>
    <TooltipProvider>
    {/* Reserved only when the tab bar renders; the connect step has none and cannot scroll past the gap. */}
    <div
      className={cn(
        "screen-edge flex min-h-dvh flex-col p-gutter pl-gutter-left pr-gutter-right touch:pt-[calc(var(--safe-area-top)+8px)] pb-[calc(var(--spacing-gutter)+var(--safe-area-bottom)+var(--plan-bar-clearance))]",
        step === 'connect' || step === 'browse'
          ? "max-md:touch:pb-[calc(var(--spacing-gutter)+var(--safe-area-bottom))]"
          : "max-md:touch:pb-[calc(var(--spacing-gutter)+var(--safe-area-bottom)+var(--spacing-tab-bar-clearance)+var(--plan-bar-clearance))]",
      )}
      style={{ '--plan-bar-clearance': step === 'connect' || step === 'browse' ? '0px' : planBarClearance } as React.CSSProperties}
    >
      <div
        aria-hidden
        className="pointer-events-none fixed inset-x-0 top-0 z-[60] h-[calc(var(--safe-area-top)+0.75rem)] bg-linear-to-b from-background to-background/0 backdrop-blur [mask-image:linear-gradient(to_bottom,black_55%,transparent)] hidden touch:block"
      />
      {/* Manual, so a toast handing focus back to the tab clicked before it does not switch to it. */}
      <Tabs
        activationMode="manual"
        value={activeTab}
        onValueChange={(v) => handleTabChange(v as 'mods' | 'system')}
        className="flex flex-1 flex-col gap-3"
      >
        {step === 'connect' || step === 'browse' ? null : <PageHeader
          device={device}
          deviceMachine={deviceInfo.machine}
          firmware={deviceInfo.firmware}
          onSettings={openSettings}
          onDisconnect={handleDisconnect}
          nav={(
            <MainTabBar
              activeTab={activeTab}
              collapsed={tabsCollapsed && !planOpen}
              hidden={keyboardVisible}
              onExpand={expandTabs}
              modsDot={modsStatus.dot}
              modsDotBesidePlanBar={modsStatus.dotWithoutPlan}
              systemAlert={systemAlert}
              device={device}
              planBar={shownPlanBar}
              modsDisabled={modsLocked}
            />
          )}
        />}

        {initialDataLoaded && <ConnectPage
          step={step}
          setStep={setStep}
          savedDevices={savedDevices}
          deviceOutlooks={deviceOutlooks}
          setSavedDevices={setSavedDevices}
          setDevice={setDevice}
          deviceInfo={deviceInfo}
          setConnectedDeviceId={setConnectedDeviceId}
          onConnected={handleConnected}
          onSettings={openSettings}
          initialKeys={availableKeys}
          initialAgentAvailable={sshAgentAvailable}
          capabilities={capabilities}
        />}

        <Dialog open={connectionModalVisible} onOpenChange={() => {}} closable={false}>
          <DialogContent>
            {/* Stays open from the loss through the reload so it reads as one interruption. */}
            {connectionStatus === 'restoring' ? (
              <>
                <DialogHeader severity="info" icon={WifiHigh}>
                  <DialogTitle>Reconnected</DialogTitle>
                  <DialogDescription>Loading from your reMarkable</DialogDescription>
                </DialogHeader>
                <Progress
                  indeterminate
                  className="h-1 bg-muted"
                  indicatorClassName="bg-[var(--info)]"
                  aria-label="Loading from your reMarkable"
                />
              </>
            ) : (
              <>
                <DialogHeader
                  severity={connectionStatus === 'failed' ? 'destructive' : 'warning'}
                  icon={WifiSlash}
                >
                  <DialogTitle>
                  {connectionStatus === 'failed' ? 'Connection failed' : 'Connection lost'}
                    </DialogTitle>
                  <DialogDescription>
                    {connectionStatus === 'failed'
                      ? connectionDetail
                      : 'Trying to reconnect. Make sure your reMarkable is awake and on Wi-Fi or USB.'}
                  </DialogDescription>
                </DialogHeader>
                {connectionStatus === 'failed' && <OutputDisclosure output={connectionErrorDetails} />}
                {connectionStatus !== 'failed' && (
                  <RetryCountdown attempt={reconnectAttempt || 1} maxAttempts={reconnectMaxAttempts || 5} phase={retryPhase} />
                )}
              </>
            )}
              <DialogFooter>
                <Button variant="outline" onClick={handleDisconnect}>Disconnect</Button>
                {/* Not offered when the same secret would be rejected again or another reMarkable answered; both need a fresh connect. */}
                {connectionStatus === 'failed' && connectedDeviceId && connectionErrorCode !== ErrorCode.ErrAuthFailed && connectionErrorCode !== ErrorCode.ErrDeviceChanged && (
                  <Button onClick={handleReconnect} disabled={manualReconnecting}>
                    {manualReconnecting && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
                    {manualReconnecting ? 'Reconnecting…' : 'Reconnect'}
                  </Button>
                )}
              </DialogFooter>
            </DialogContent>
        </Dialog>

        {step !== 'connect' && step !== 'browse' && (() => {
          return (
            <Dialog open={betaModalOpen} onOpenChange={(o) => { if (!o) acknowledgeBeta() }}>
              <DialogContent>
                <DialogHeader severity="warning" icon={Warning}>
                  <DialogTitle>Beta OS</DialogTitle>
                  <DialogDescription>
                    Your reMarkable is on {osChannel?.activeVersion}, newer than the latest stable release ({osChannel?.latestPublic}). reMarkable&apos;s <a href="https://support.remarkable.com/s/article/End-user-agreement-for-Opt-In-Beta" className="underline hover:text-foreground" onClick={(e) => { e.preventDefault(); SystemService.OpenExternalURL('https://support.remarkable.com/s/article/End-user-agreement-for-Opt-In-Beta') }}>beta program</a> doesn&apos;t permit installing third-party software, so mods aren&apos;t built for beta releases and you won&apos;t receive community support for issues on them. You can downgrade to a stable version in the OS Manager.
                  </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                  <Button variant="outline" onClick={acknowledgeBeta}>Got it</Button>
                  <Button onClick={() => { acknowledgeBeta(); setActiveTab('system'); setShowSoftwareManager(true) }}>
                    Open OS Manager
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          )
        })()}

        {step === 'browse' && (
          <BrowsePage packages={packages} catalogStatus={catalogStatus} onBack={() => setStep('connect')} onSettings={openSettings} onRefreshCatalog={refreshBrowseCatalog} openPackageByNameRef={openPackageByNameRef} />
        )}

        {step !== 'connect' && step !== 'browse' && (
          <>
              <ModsTab
                activeTab={activeTab}
                osMismatchDetected={osMismatchDetected}
                storedOsVersion={storedOsVersion}
                currentOsVersion={currentOsVersion}
                compatibilityStatus={compatibilityStatus}
                onOsSynced={handleOsSynced}
                onOpenOSManager={openOSManager}
                bootstrapping={!!vellumInstall?.running}
                bootstrapOutput={vellumInstall?.output ?? ''}
                bootstrapError={vellumInstall?.error ?? null}
                onInstallVellum={() => handleInstallVellum('install')}
                vellumBrokenInstall={vellumBrokenInstall}
                vellumCleaning={vellumRun?.target === 'cleanup'}
                onCleanupVellum={handleCleanupVellum}
                installQueue={installQueue}
                setInstallQueue={setInstallQueue}
                uninstallQueue={uninstallQueue}
                setUninstallQueue={setUninstallQueue}
                upgradeQueue={upgradeQueue}
                setUpgradeQueue={setUpgradeQueue}
              upgradeCandidates={upgradeCandidates}
                queueError={queueError}
                setQueueError={setQueueError}
                refreshingPackages={refreshingPackages}
                setRefreshingPackages={setRefreshingPackages}
              setPlanSolve={setPlanSolve}
              modsStatus={modsStatus}
              setPlanOpen={setPlanOpen}
              setPlanApplying={setPlanApplying}
              setPlanBar={setPlanBar}
              setModsLocked={setModsLocked}
              setOsSyncStatus={setOsSyncStatus}
              planActionsRef={planActionsRef}
                selectPackageForOSRef={selectPackageForOSRef}
                openPackageByNameRef={openPackageByNameRef}
              />

            <SystemTab
              shellRunning={shellRunning}
              setShellRunning={setShellRunning}
              maintenanceCommands={maintenanceCommands}
              updateServiceStatus={updateServiceStatus}
              xochitlRunning={xochitlRunning}
              currentLauncher={currentLauncher}
              xoviActive={xoviActive}
              hashtab={hashtab}
              systemStatus={systemStatus}
              osChannel={osChannel}
              osMismatchDetected={osMismatchDetected}
              compatibilityStatus={compatibilityStatus}
              active={activeTab === 'system'}
              osSyncStatus={osSyncStatus}
              onReviewPackages={() => planActionsRef.current?.open()}
              onApplyOsSync={() => planActionsRef.current?.applyOsSync()}
              reenableStatus={reenableStatus}
              timezoneMismatch={timezoneMismatch}
              selectedTimezone={selectedTimezone}
              deviceTimezone={deviceTimezone}
              runningSystemTask={runningOperation?.kind === 'system-task' ? runningOperation.target : null}
              currentRunningCommand={runningComponentId && runningCommandId ? { componentId: runningComponentId, commandId: runningCommandId } : null}
              settingTimezone={runningOperation?.kind === 'timezone'}
              handleSystemTask={handleSystemTask}
              handleComponentMaintenance={handleComponentMaintenance}
              handleRunReenable={handleRunReenable}
              handleTimezoneChange={handleTimezoneChange}
              handleSetTimezone={handleSetTimezone}
              handleSelectPackageForOS={handleSelectPackageForOS}
              runningReenable={runningOperation?.kind === 'reenable'}
              guideStatus={guideStatus}
              installingGuide={runningOperation?.kind === 'guide'}
              handleInstallGuide={handleInstallGuide}
              onRepairVellum={() => handleInstallVellum('repair')}
              onUninstallVellum={handleUninstallVellum}
              onSettings={openSettings}
              onDisconnect={handleDisconnect}
              showSoftwareManager={showSoftwareManager}
              onShowSoftwareManagerChange={setShowSoftwareManager}
            />
            {/* The side sheet leaves the desktop strip in view, so it slides down out of the way while the sheet is open. */}
            {!touchLayout && stripModel && (
              <div
                aria-hidden={!stripShown || undefined}
                className={cn(
                  'fixed inset-x-0 bottom-0 z-40 border-t pb-[var(--safe-area-bottom)] bg-background/95 backdrop-blur transition-[translate,background-color] duration-300 ease-out hover:bg-accent/40 supports-[backdrop-filter]:bg-background/80 motion-reduce:transition-none',
                  stripShown ? 'translate-y-0' : 'pointer-events-none translate-y-full',
                )}
              >
                <PlanBarWash wash={stripModel.wash} />
                <PlanBarContent model={stripModel} focusable={stripShown} className="py-3 pl-gutter-left pr-gutter-right" />
              </div>
            )}
          </>
        )}
      </Tabs>

      <Dialog open={showingProgress} onOpenChange={() => {}} closable={false}>
        <DialogContent
          size={commandOutputOpen ? '2xl' : 'default'}
          className="transition-[max-width] duration-200 ease-out"
        >
          {hookRun ? (
            <ProgressModal
              title={hookRun.title}
              progressText=""
              icon={Wrench}
              expectedSeconds={hookRun.expectedSeconds}
              runKey={hookRun.operationId}
              terminalOutput={hookRun.output}
              isComplete={hookRun.status === 'succeeded' || hookRun.status === 'failed'}
              failed={hookRun.status === 'failed'}
              canStop={hookRun.status === 'running'}
              onStop={stopHook}
              stopWarning={STOP_PARTWAY_WARNING}
              stopping={hookRun.status === 'stopping'}
              onAbandon={abandonOperation}
              onClose={closeOperation}
              terminalTheme={resolvedTerminalTheme}
              onOutputOpenChange={setCommandOutputOpen}
            />
          ) : operationOnModal ? (
            <ProgressModal
              title={operationOnModal.title}
              progressText=""
              icon={Wrench}
              expectedSeconds={operationOnModal.expectedSeconds}
              runKey={operationOnModal.id}
              terminalOutput={operationOnModal.output}
              isComplete={operationOnModal.status === 'succeeded' || operationOnModal.status === 'failed'}
              failed={operationOnModal.status === 'failed'}
              canStop={operationOnModal.status === 'running'}
              onStop={stopOperation}
              stopWarning={operationOnModal.canStop ? undefined : STOP_PARTWAY_WARNING}
              stopping={operationOnModal.status === 'stopping'}
              onAbandon={abandonOperation}
              onClose={closeOperation}
              terminalTheme={resolvedTerminalTheme}
              showOutput={operationOnModal.showOutput}
              onOutputOpenChange={setCommandOutputOpen}
            />
          ) : vellumUninstall ? (
            <ProgressModal
              title="Uninstalling Vellum"
              // Names no button on purpose: Repair is gone after a reconnect, so the advice would go stale when needed.
              progressText={vellumUninstall.running ? 'Removing Vellum…' : 'Uninstall failed. Vellum may be partly removed.'}
              icon={Package}
              terminalOutput={vellumUninstall.output}
              isComplete={!vellumUninstall.running}
              failed={!vellumUninstall.running}
              canStop={vellumUninstall.running && !vellumUninstall.stopping}
              onStop={stopOperation}
              stopWarning={VELLUM_STOP_WARNING}
              stopping={vellumUninstall.stopping}
              onAbandon={abandonOperation}
              onClose={() => setVellumFailure(null)}
              terminalTheme={resolvedTerminalTheme}
              onOutputOpenChange={setCommandOutputOpen}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <HookDialog
        open={!!operationPrompt}
        request={operationPrompt?.request ?? null}
        onRespond={respondToOperation}
        onDismiss={() => respondToOperation('cancel')}
      />

      <Dialog open={showGuideRestartDialog} onOpenChange={setShowGuideRestartDialog}>
        <DialogContent>
          <DialogHeader severity="info" icon={ArrowsCounterClockwise}>
            <DialogTitle>Restart reMarkable UI?</DialogTitle>
            <DialogDescription>
              The reManager guide is installed. It appears on your reMarkable after the UI restarts.
            </DialogDescription>
          </DialogHeader>
          {guideRestartError && (
            <Alert variant="destructive">
              <WarningCircle className="h-4 w-4" />
              <AlertDescription>{guideRestartError.message}</AlertDescription>
            </Alert>
          )}
          {guideRestartError && <OutputDisclosure output={guideRestartError.detail} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowGuideRestartDialog(false)}>
              Not now
            </Button>
            <Button onClick={async () => {
              setGuideRestartError(null)
              const result = await startOperation({
                kind: 'restart-ui',
                title: 'Restart the reMarkable UI',
                background: true,
                reportsOwnFailure: true,
                run: id => xoviActive
                  ? PackagesService.RunMaintenanceCommand(id, 'xovi-extensions', 'start', device)
                  : ConfigService.RestartXochitl(id),
              })
              if (!result.ok) {
                if (result.canceled) return
                const failure = operationFailure(result)
                setGuideRestartError({ message: handleError(failure, 'Could not restart the reMarkable UI.'), detail: errorDetail(failure) })
                return
              }
              // Closed only once it worked, so a failure has somewhere to land.
              setShowGuideRestartDialog(false)
            }} disabled={connectionStatus !== 'connected' || commandRunning}>
              {runningOperation?.kind === 'restart-ui' && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SettingsDialog
        open={showSettingsDialog}
        onOpenChange={setShowSettingsDialog}
        settings={settingsValues}
        onUpdate={handleUpdateSettings}
        onOpenSupport={(screen) => { setSupportScreen(screen); setShowSupportBundles(true) }}
        supportPageOpen={showSupportBundles}
        connectedDeviceId={connectedDeviceId}
        capabilities={capabilities}
        initialSection={settingsSection}
        appVersion={appVersion}
      />

      <SupportBundlePage
        open={showSupportBundles}
        onOpenChange={setShowSupportBundles}
        initialScreen={supportScreen}
        isConnected={!!device}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
        supportUpload={capabilities.supportUpload}
      />

      <Dialog
        open={!!vellumRepair}
        closable={!vellumRepair?.running}
        onOpenChange={(open) => {
          if (!open) setVellumFailure(null)
        }}
      >
        <DialogContent
          size={commandOutputOpen ? '2xl' : 'default'}
          className="transition-[max-width] duration-200 ease-out"
        >
          {/* Named for the operation, not its outcome; the error takes the subtitle. */}
          <ProgressModal
            title="Repairing Vellum"
            progressText={vellumRepair?.error || 'Reinstalling the latest version of Vellum. Your installed mods are kept.'}
            terminalOutput={vellumRepair?.output ?? ''}
            isComplete={!!vellumRepair?.error}
            failed={!!vellumRepair?.error}
            icon={Package}
            terminalTheme={resolvedTerminalTheme}
            canStop={!!vellumRepair?.running && !vellumRepair.stopping}
            onStop={stopOperation}
            stopWarning={VELLUM_STOP_WARNING}
            stopping={!!vellumRepair?.stopping}
            onAbandon={abandonOperation}
            onRetry={() => handleInstallVellum('repair')}
            retryDisabled={connectionStatus !== 'connected'}
            onOutputOpenChange={setCommandOutputOpen}
            onClose={() => setVellumFailure(null)}
          />
        </DialogContent>
      </Dialog>

      <VellumInstallSuccessDialog
        open={vellumInstalledDialog !== null}
        reinstalled={vellumInstalledDialog === 'reinstalled'}
        onOpenChange={(open) => {
          if (!open) setVellumInstalledDialog(null)
        }}
      />

      <VellumUninstallSuccessDialog
        open={showVellumUninstalled}
        onOpenChange={setShowVellumUninstalled}
      />

      <DnsErrorModal
        open={showDnsErrorModal}
        onClose={() => setShowDnsErrorModal(false)}
        onEnableProxyMode={handleEnableProxyModeFromModal}
      />

      <FilesystemRestoreErrorDialog
        open={showFilesystemRestoreError}
        onRetry={handleFilesystemRestoreRetry}
        onReboot={handleFilesystemRestoreReboot}
        onDismiss={handleFilesystemRestoreDismiss}
        isRetrying={isRetryingFilesystemRestore}
        isConnected={connectionStatus === 'connected'}
      />

      <Toaster position="bottom-right" richColors />
    </div>
    </TooltipProvider>
    </AppProvider>
  )
}
