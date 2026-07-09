import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { Toaster, toast } from 'sonner'
import { applyThemeWithPortal } from './main'
import { debugLog } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { ProgressModal } from '@/components/ProgressModal'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'
import { ConsolidatedWarningBanner, hasActiveWarnings } from '@/components/ConsolidatedWarningBanner'
import { BannerSwitcher } from '@/components/BannerSwitcher'
import { VellumInstallSuccessDialog } from '@/components/VellumInstallSuccessDialog'
import { VellumUninstallSuccessDialog } from '@/components/VellumUninstallSuccessDialog'
import { SettingsDialog } from '@/components/SettingsDialog'
import { PageHeader } from '@/components/PageHeader'
import { SupportBundlePage } from '@/components/SupportBundlePage'
import { DnsErrorModal } from '@/components/DnsErrorModal'
import { FilesystemRestoreErrorDialog } from '@/components/FilesystemRestoreErrorDialog'
import { UserGuideOffer } from '@/components/UserGuideOffer'
import { ConnectPage } from '@/pages/ConnectPage'
import { AppProvider } from '@/contexts/AppContext'
import { useConnectionEvents } from '@/hooks/useConnectionEvents'
import { useVellumEvents } from '@/hooks/useVellumEvents'
import { useInstallEvents } from '@/hooks/useInstallEvents'
import { useMaintenanceEvents } from '@/hooks/useMaintenanceEvents'
import { useWarnings } from '@/hooks/useWarnings'
import { UtilitiesTab } from '@/tabs/UtilitiesTab'
import { MaintenanceTab } from '@/tabs/MaintenanceTab'
import { ModsTab } from '@/tabs/ModsTab'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Loader2, Check, AlertTriangle, AlertCircle, WifiOff, Info, CheckCircle2, BookOpen, Cpu } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { PackageInfo, CompatibilityResult, MaintenanceCommandInfo, SystemTaskInfo, SSHKey, SavedDevice, UpdateServiceStatus, InstallSimulationResult, UninstallSimulationResult, InstalledPackagesResult, HashtabVersionStatus, TimezoneStatus, Step } from '@/lib/types'

declare global {
  interface Window {
    go: {
      main: {
        App: {
          Connect(host: string, password: string): Promise<{ success: boolean; message: string; code?: string; retryable?: boolean; device?: string }>
          ConnectWithKey(host: string, keyPath: string, passphrase: string): Promise<{ success: boolean; message: string; code?: string; retryable?: boolean; device?: string }>
          ConnectWithAgent(host: string): Promise<{ success: boolean; message: string; code?: string; retryable?: boolean; device?: string }>
          IsSSHAgentAvailable(): Promise<boolean>
          CancelConnect(): Promise<void>
          Disconnect(): Promise<void>
          IsConnected(): Promise<boolean>
          RunCommand(cmd: string): Promise<string>
          GetReadmeContent(url: string): Promise<string>
          RunCommandWithOutput(cmd: string, requiresPTY: boolean): Promise<void>
          StopCommand(): Promise<void>
          StartShell(rows: number, cols: number): Promise<void>
          WriteToShell(data: string): Promise<void>
          ResizeShell(rows: number, cols: number): Promise<void>
          StopShell(): Promise<void>
          IsShellActive(): Promise<boolean>
          GetDeviceInfo(): Promise<Record<string, string>>
          GetUpdateServiceStatus(): Promise<UpdateServiceStatus>
          GetXochitlStatus(): Promise<{ running: boolean; xoviActive: boolean }>
          GetDefaultSSHKeys(): Promise<SSHKey[]>
          SelectKeyFile(): Promise<string>
          GetSavedDevices(): Promise<SavedDevice[]>
          SaveDevice(id: string, name: string, host: string, authType: string, password: string, keyPath: string, keyPassphrase: string): Promise<string>
          DeleteSavedDevice(id: string): Promise<void>
          ConnectToSavedDevice(id: string): Promise<{ success: boolean; message: string; code?: string; retryable?: boolean; device?: string }>
          UpdateDeviceName(id: string, name: string): Promise<void>
          CheckVellumInstalled(): Promise<boolean>
          BootstrapVellum(): Promise<void>
          CheckPackageInstalled(pkgName: string): Promise<boolean>
          CheckHashtabVersion(): Promise<HashtabVersionStatus>
          GetPackages(deviceType: string, firmware: string, arch: string): Promise<PackageInfo[]>
          RefreshMetadata(): Promise<void>
          GetInstalledPackages(): Promise<string[]>
          GetInstalledPackagesWithOsCheck(): Promise<InstalledPackagesResult>
          GetInstalledPackagesWithVersions(): Promise<Record<string, string>>
          GetPackageForOS(name: string, targetOS: string, deviceType: string, arch: string): Promise<PackageInfo | null>
          RunReenable(): Promise<void>
          SimulatePackageUpgrade(): Promise<{ packages: string[]; hasUpgrades: boolean }>
          RunPackageUpgrade(): Promise<void>
          RunUpgrade(): Promise<void>
          GetPackageCompatibilityStatus(): Promise<{
            installedPackages: string[]
            compatiblePackages: string[]
            incompatiblePackages: string[]
            currentOsVersion: string
            storedOsVersion: string
            fetchFailed: boolean
          }>
          GetMaintenanceCommands(pkgName: string): Promise<MaintenanceCommandInfo[]>
          GetAllMaintenanceCommands(): Promise<Record<string, MaintenanceCommandInfo[]>>
          GetSystemTasksInfo(): Promise<SystemTaskInfo[]>
          GetDeviceDisplayName(machine: string): Promise<string>
          GetDeviceArchitecture(deviceType: string): Promise<string>
          InstallPackages(packageNames: string[], deviceType: string): Promise<void>
          UninstallPackages(packageNames: string[], deviceType: string): Promise<void>
          SimulateInstall(packageNames: string[], deviceType: string): Promise<InstallSimulationResult>
          SimulateUninstall(packageNames: string[]): Promise<UninstallSimulationResult>
          RunMaintenanceCommand(pkgName: string, commandId: string, deviceType: string): Promise<void>
          RunSystemTask(taskId: string, deviceType: string): Promise<void>
          RespondToDialog(response: string): Promise<void>
          CancelInstallation(): Promise<void>
          GetAppVersion(): Promise<string>
          CheckForAppUpdate(): Promise<{ updateAvailable: boolean; latestVersion: string; currentVersion: string; releaseURL: string; error?: string }>
          GetSettings(): Promise<{ tabVisibility: Record<string, boolean>; proxyMode: boolean; suppressSystemFileWarnings: boolean; suppressGuideOffer: boolean; preventSleep: boolean; theme: string; terminalTheme: string; editorTheme: string; checkForUpdates: boolean; sshAgentSocketPath: string }>
          SaveSettings(tabVisibility: Record<string, boolean>, proxyMode: boolean, suppressSystemFileWarnings: boolean, preventSleep: boolean, theme: string, terminalTheme: string, editorTheme: string, checkForUpdates: boolean, sshAgentSocketPath: string): Promise<void>
          InstallUserGuide(): Promise<void>
          CheckUserGuide(): Promise<{ installed: boolean; needsUpdate: boolean; skipped: boolean; docId: string }>
          DismissGuideOffer(): Promise<void>
          EnableGuideOffer(): Promise<void>
          GetSystemColorScheme(): Promise<string>
          UninstallVellum(removeAllPackages: boolean): Promise<void>
          CleanupBrokenVellum(): Promise<void>
          GetDeviceTimezone(): Promise<string>
          GetTimezoneStatus(): Promise<TimezoneStatus>
          SaveDeviceTimezone(timezone: string): Promise<void>
          SetDeviceTimezone(timezone: string, deviceType: string): Promise<void>
          ListDirectory(path: string): Promise<{ name: string; path: string; size: number; isDir: boolean; modTime: number; mode: string }[]>
          DownloadFile(remotePath: string): void
          SelectFilesForUpload(): Promise<string[]>
          UploadFilesFromPaths(localPaths: string[], remotePath: string): void
          DownloadFolder(remotePath: string): void
          UploadFolder(remotePath: string): void
          CancelFolderTransfer(): void
          DeletePath(path: string): Promise<void>
          RenamePath(oldPath: string, newPath: string): Promise<void>
          CreateDirectory(path: string): Promise<void>
          ReadConfigFile(): Promise<string>
          WriteConfigFile(content: string): Promise<void>
          BackupConfigFile(): Promise<string>
          SelectPDFFile(): Promise<string>
          SelectPDFFiles(): Promise<string[]>
          SelectImportFiles(): Promise<string[]>
          GetPDFFileInfo(localPath: string): Promise<{ path: string; size: number; pageCount: number }>
          GetImportFileInfo(localPath: string): Promise<{ path: string; size: number; pageCount: number; fileType: string; visibleName: string }>
          ImportEpubFromPath(localPath: string, visibleName: string, restartXochitl: boolean): Promise<void>
          ImportPDFFromPath(localPath: string, visibleName: string, restartXochitl: boolean, pageCountOverride: number, coverPageNumber: number | null): Promise<void>
          ImportRmdocFromPath(localPath: string, visibleName: string, restartXochitl: boolean): Promise<void>
          ListConfigBackups(): Promise<Array<{ name: string; timestamp: number; size: number }>>
          RestoreConfigBackup(backupName: string): Promise<void>
          SelectBackupFile(): Promise<string>
          CreateDeviceBackup(destPath: string): void
          SelectRestoreFile(): Promise<string>
          RestoreDeviceBackup(archivePath: string): void
          CancelBackup(): void
          RevealInFileManager(path: string): void
          RetryRestoreFilesystem(): Promise<void>
          RebootDevice(): Promise<void>
          GetPartitionInfo(): Promise<{
            active: { number: number; version: string; label: string; isActive: boolean; isNextBoot: boolean }
            fallback: { number: number; version: string; label: string; isActive: boolean; isNextBoot: boolean }
            deviceType: string
            encrypted: boolean
            updatePending: boolean
          }>
          SwitchBootPartition(partitionNumber: number): Promise<{
            success: boolean; method: string; previousBoot: number; newBoot: number; message: string
          }>
          GetAvailableOSVersions(): Promise<{
            version: string; isLatest: boolean; isInstalled: boolean; filename: string; size: number
          }[]>
          GetOSInstallMode(): Promise<string>
          InstallOSVersion(version: string): void
          CancelOSInstall(): void
          CheckOSCompatibility(targetVersion: string): Promise<CompatibilityResult>
          IsSleepScreenSupported(): Promise<boolean>
          GetSleepScreen(): Promise<string>
          SetSleepScreen(imagePath: string): Promise<void>
          ResetSleepScreen(): Promise<void>
          RestartXochitl(): Promise<void>
          RescanLibrary(): Promise<void>
          DeleteAllLogs(): Promise<void>
          GetSupportBundlePreview(): Promise<{ included: string[]; excluded: string[] }>
          GenerateSupportBundle(deviceID: string): void
          UploadSupportBundle(deviceID: string): void
          AppendSupportBundle(remoteID: string, deviceID: string): void
          DeleteSupportBundle(remoteID: string): Promise<void>
          GetBundleHistory(): Promise<{ id: string; remoteId: string; url: string; deviceName: string; deviceId: string; uploadedAt: number; lastAppended?: number; expiresAt?: number }[]>
          CopyToClipboard(text: string): void
          GetSupportBundleSessionID(): Promise<string>
          ResetSupportBundleSession(): Promise<void>
          SubmitProblemReport(description: string, email: string, bundleURL: string, deviceID: string): Promise<void>
        }
      }
    }
    runtime: {
      EventsOn(eventName: string, callback: (...args: unknown[]) => void): () => void
      BrowserOpenURL(url: string): void
    }
  }
}

export default function App() {
  const [step, setStep] = useState<Step>('connect')
  const [sshAgentAvailable, setSSHAgentAvailable] = useState(false)
  const [availableKeys, setAvailableKeys] = useState<SSHKey[]>([])
  const [device, setDevice] = useState<string>('')
  const [deviceInfo, setDeviceInfo] = useState<Record<string, string>>({})
  const [installedPackages, setInstalledPackages] = useState<Map<string, string>>(new Map())
  const [refreshingPackages, setRefreshingPackages] = useState(false)
  const [vellumInstalled, setVellumInstalled] = useState<boolean | null>(null)
  const {
    warningsChecked, setWarningsChecked,
    osUpgradeDetected, setOsUpgradeDetected,
    prevOsVersion, setPrevOsVersion,
    newOsVersion, setNewOsVersion,
    osMismatchDetected, setOsMismatchDetected,
    osMismatchDetectedRef,
    storedOsVersion, setStoredOsVersion,
    currentOsVersion, setCurrentOsVersion,
    checklistLoading, setChecklistLoading,
    compatibilityStatus, setCompatibilityStatus,
    hashtabMismatch, setHashtabMismatch,
    hashtabMissing, setHashtabMissing,
    prerelease, setPrerelease,
    timezoneMismatch, setTimezoneMismatch,
    deviceTimezone, setDeviceTimezone,
    selectedTimezone, setSelectedTimezone,
    reenableStatus, setReenableStatus,
    showAutoUpdateBanner, setShowAutoUpdateBanner,
    updateServiceStatus, setUpdateServiceStatus,
    xochitlRunning, setXochitlRunning,
    xoviActive, setXoviActive,
    guideOffer, setGuideOffer,
    guideInstalling, setGuideInstalling,
    showGuideRestartDialog, setShowGuideRestartDialog,
  } = useWarnings()

  const [installing, setInstalling] = useState(false)
  const [activeTab, setActiveTab] = useState<'mods' | 'maintenance' | 'utilities'>('mods')
  const [installQueue, setInstallQueue] = useState<Set<string>>(new Set())
  const [uninstallQueue, setUninstallQueue] = useState<Set<string>>(new Set())
  const [uninstalling, setUninstalling] = useState(false)
  const [commandRunning, setCommandRunning] = useState(false)

  const [showSettingsDialog, setShowSettingsDialog] = useState(false)
  const [showSoftwareManager, setShowSoftwareManager] = useState(false)
  const [showSupportBundles, setShowSupportBundles] = useState(false)
  const [appVersion, setAppVersion] = useState('dev')
  const [tabVisibility, setTabVisibility] = useState<Record<string, boolean>>({
    mods: true,
    maintenance: true,
    utilities: true,
  })
  const [proxyMode, setProxyMode] = useState(true)
  const [suppressSystemFileWarnings, setSuppressSystemFileWarnings] = useState(false)
  const [suppressGuideOffer, setSuppressGuideOffer] = useState(false)
  const [preventSleep, setPreventSleep] = useState(true)
  const [theme, setTheme] = useState('system')
  const [terminalTheme, setTerminalTheme] = useState('match')
  const [editorTheme, setEditorTheme] = useState('match')
  const [checkForUpdates, setCheckForUpdates] = useState(true)
  const [sshAgentSocketPath, setSSHAgentSocketPath] = useState('')
  const [systemPrefersDark, setSystemPrefersDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  const [dnsErrorShown, setDnsErrorShown] = useState(false)
  const [showDnsErrorModal, setShowDnsErrorModal] = useState(false)


  const [packages, setPackages] = useState<PackageInfo[]>([])
  const [systemTasks, setSystemTasks] = useState<SystemTaskInfo[]>([])
  const [maintenanceCommands, setMaintenanceCommands] = useState<Record<string, MaintenanceCommandInfo[]>>({})

  const [savedDevices, setSavedDevices] = useState<SavedDevice[]>([])
  const [connectedDeviceId, setConnectedDeviceId] = useState('')
  const [queueError, setQueueError] = useState<string | null>(null)
  const [pendingInstallConfirm, setPendingInstallConfirm] = useState<{
    packages: string[]
    requested: string[]
  } | null>(null)
  const [pendingXoviInfo, setPendingXoviInfo] = useState<string[] | null>(null)
  const [tripletapDecision, setTripletapDecision] = useState<'yes' | 'no' | null>(null)
  const [xoviAckChecked, setXoviAckChecked] = useState(false)
  const [pendingUninstallConfirm, setPendingUninstallConfirm] = useState<{
    selected: string[]
    packages: string[]
    blocked: Record<string, string[]> | null
    useRecursive: boolean
    worldDeps?: string[]
  } | null>(null)

  const [runningReenable, setRunningReenable] = useState(false)
  const [upgradesAvailable, setUpgradesAvailable] = useState(false)
  const [reinstallInProgress, setReinstallInProgress] = useState(false)

  const xoviNotRunning = useMemo(() =>
    installedPackages.has('xovi') && xochitlRunning && !xoviActive && !hashtabMismatch && !hashtabMissing,
    [installedPackages, xochitlRunning, xoviActive, hashtabMismatch, hashtabMissing]
  )


  const deviceRef = useRef(device)
  deviceRef.current = device



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
      const installedVersions = await window.go.main.App.GetInstalledPackagesWithVersions()
      setInstalledPackages(new Map(Object.entries(installedVersions || {})))
      return true
    } catch (err) {
      console.error('Failed to rescan package status:', err)
      return false
    }
  }

  const resetOperationStateRef = useRef<() => void>(() => {})
  const refreshDeviceStateRef = useRef<(deviceType: string) => Promise<void>>(async () => {})
  const handleConnectionRestored = useCallback(async (deviceType: string) => {
    resetOperationStateRef.current()
    if (deviceType) {
      setDevice(deviceType)
    }
    try {
      await refreshDeviceStateRef.current(deviceType)
    } catch (err) {
      debugLog('Failed to refresh device info on reconnect:', err)
    }
    await rescanAllPackages()
  }, [])

  const handleConnectionLost = useCallback(() => {
    resetOperationStateRef.current()
  }, [])

  const {
    connectionStatus,
    setConnectionStatus,
    reconnectAttempt,
    setReconnectAttempt,
    reconnectMaxAttempts,
    connectionError,
    setConnectionError,
    showFilesystemRestoreError,
    isRetryingFilesystemRestore,
    writeableRootBusy,
    setWriteableRootBusy,
    handleRetryRestore: handleFilesystemRestoreRetry,
    handleRebootDevice: handleFilesystemRestoreReboot,
    handleDismissRestoreError: handleFilesystemRestoreDismiss,
  } = useConnectionEvents({
    onConnectionRestored: handleConnectionRestored,
    onConnectionLost: handleConnectionLost,
    onUnsavedConnectionFailed: () => handleDisconnect(),
    deviceRef,
  })

  const {
    bootstrapping,
    setBootstrapping,
    bootstrapOutput,
    setBootstrapOutput,
    bootstrapError,
    setBootstrapError,
    bootstrapSuccess,
    setBootstrapSuccess,
    vellumBrokenInstall,
    setVellumBrokenInstall,
    vellumCleaning,
    setVellumCleaning,
    vellumUninstalling,
    setVellumUninstalling,
    vellumUninstallOutput,
    setVellumUninstallOutput,
    vellumUninstallError,
    setVellumUninstallError,
    vellumUninstallSuccess,
    setVellumUninstallSuccess,
  } = useVellumEvents({
    setVellumInstalled,
    setInstalledPackages,
    setShowSettingsDialog,
    rescanAllPackages,
  })

  const checkDnsProxyError = async (dnsError: boolean) => {
    if (!dnsError || dnsErrorShown) return
    const currentProxyMode = await window.go.main.App.GetSettings().then(s => s.proxyMode)
    if (!currentProxyMode) {
      setShowDnsErrorModal(true)
      setDnsErrorShown(true)
    }
  }



  const selectedTimezoneRef = useRef(selectedTimezone)
  selectedTimezoneRef.current = selectedTimezone

  const {
    output,
    setOutput,
    currentComponent,
    progressStatus,
    progressIndex,
    setProgressIndex,
    progressTotal,
    setProgressTotal,
    progressPercentage,
    setProgressPercentage,
    showProgressModal,
    setShowProgressModal,
    progressModalType,
    setProgressModalType,
    showRebuildDialog,
    setShowRebuildDialog,
    dialogRequest,
    setDialogRequest,
    dialogRespondedRef,
    runningHookTitle,
    setRunningHookTitle,
    lastInstallSuccess,
    setLastInstallSuccess,
    lastOperationType,
    setLastOperationType,
    showStartUIDialog,
    setShowStartUIDialog,
  } = useInstallEvents({
    setInstalling,
    setUninstalling,
    setCommandRunning,
    setInstallQueue,
    setXochitlRunning,
    setXoviActive,
    setHashtabMismatch,
    setHashtabMissing,
    setCompatibilityStatus,
    checkDnsProxyError,
    rescanAllPackages,
    osMismatchDetectedRef,
  })

  const {
    maintenanceOutput,
    setMaintenanceOutput,
    commandContext,
    setCommandContext,
    currentRunningCommand,
    setCurrentRunningCommand,
    runningSystemTask,
    setRunningSystemTask,
    runningSystemTaskRef,
    settingTimezone,
    setSettingTimezone,
    settingTimezoneRef,
    manuallyStoppedRef,
    rescanning,
    setRescanning,
  } = useMaintenanceEvents({
    setOutput,
    setCommandRunning,
    setShowProgressModal,
    setProgressModalType,
    setDialogRequest,
    setRunningHookTitle,
    setInstalling,
    setHashtabMismatch,
    setHashtabMissing,
    setTimezoneMismatch,
    setDeviceTimezone,
    setSelectedTimezone,
    setUpdateServiceStatus,
    setXochitlRunning,
    setXoviActive,
    setUpgradesAvailable,
    setOsMismatchDetected,
    osMismatchDetectedRef,
    setChecklistLoading,
    setCompatibilityStatus,
    setDeviceInfo,
    setPackages,
    checkDnsProxyError,
    rescanAllPackages,
    deviceRef,
    selectedTimezoneRef,
    setActiveTab,
  })

  const startMaintenanceOperation = (options?: { showModal?: boolean; resetProgress?: boolean }) => {
    if (options?.showModal !== false) setShowProgressModal(true)
    setProgressModalType('maintenance')
    if (options?.resetProgress !== false) setProgressPercentage(0)
    setCommandRunning(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')
  }

  useEffect(() => {
    const loadInitialData = async () => {
      try {
        debugLog('[DEBUG] loadInitialData: starting')
        const [keys, pkgs, tasks, devices, version, settings, agentAvail] = await Promise.all([
          window.go.main.App.GetDefaultSSHKeys(),
          window.go.main.App.GetPackages('', '', ''),
          window.go.main.App.GetSystemTasksInfo(),
          window.go.main.App.GetSavedDevices(),
          window.go.main.App.GetAppVersion(),
          window.go.main.App.GetSettings(),
          window.go.main.App.IsSSHAgentAvailable().catch(() => false),
        ])
        debugLog('[DEBUG] loadInitialData: got pkgs', pkgs?.length, pkgs)

        setAvailableKeys(keys || [])
        setSSHAgentAvailable(!!agentAvail)

        setPackages(pkgs || [])
        setSystemTasks(tasks || [])
        setSavedDevices(devices || [])
        setAppVersion(version || 'dev')
        setTabVisibility(settings?.tabVisibility || { mods: true, maintenance: true, utilities: true })
        setProxyMode(settings?.proxyMode ?? true)
        setSuppressSystemFileWarnings(settings?.suppressSystemFileWarnings ?? false)
        setSuppressGuideOffer(settings?.suppressGuideOffer ?? false)
        setPreventSleep(settings?.preventSleep ?? true)
        const loadedTheme = settings?.theme || 'system'
        setTheme(loadedTheme)
        localStorage.setItem('theme', loadedTheme)
        setTerminalTheme(settings?.terminalTheme || 'match')
        setEditorTheme(settings?.editorTheme || 'match')
        const shouldCheckUpdates = settings?.checkForUpdates ?? true
        setCheckForUpdates(shouldCheckUpdates)
        setSSHAgentSocketPath(settings?.sshAgentSocketPath || '')

        if (shouldCheckUpdates) {
          window.go.main.App.CheckForAppUpdate().then((result) => {
            if (result.updateAvailable && result.latestVersion) {
              toast.info(`Update available: ${result.latestVersion}`, {
                description: `You're running ${result.currentVersion}`,
                action: {
                  label: 'View Release',
                  onClick: () => window.runtime.BrowserOpenURL(result.releaseURL)
                },
                duration: 6000,
              })
            }
          }).catch((err) => {
            debugLog('[DEBUG] Update check failed:', err)
          })
        }
      } catch (err) {
        debugLog('Could not load initial data:', err)
      }
    }
    loadInitialData()
  }, [])

  useEffect(() => {
    window.go.main.App.GetSystemColorScheme().then((scheme: string) => {
      if (scheme === 'dark' || scheme === 'light') {
        setSystemPrefersDark(scheme === 'dark')
      }
    }).catch(() => {})

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const handleChange = (e: MediaQueryListEvent) => setSystemPrefersDark(e.matches)
    mediaQuery.addEventListener('change', handleChange)

    let unsubscribeTheme: (() => void) | undefined
    if (typeof window.runtime !== 'undefined') {
      unsubscribeTheme = window.runtime.EventsOn('system-theme-changed', (...args: unknown[]) => {
        const scheme = args[0] as string
        if (scheme === 'dark' || scheme === 'light') {
          setSystemPrefersDark(scheme === 'dark')
        }
      })
    }

    return () => {
      mediaQuery.removeEventListener('change', handleChange)
      unsubscribeTheme?.()
    }
  }, [])

  useEffect(() => {
    if (activeTab === 'maintenance' && step !== 'connect') {
      fetchUpdateServiceStatus()
    }
  }, [activeTab, step])

  useEffect(() => {
    if (!updateServiceStatus.enabled) {
      setShowAutoUpdateBanner(false)
    }
  }, [updateServiceStatus.enabled])

  useEffect(() => {
    if (connectionStatus !== 'connected' || commandRunning) return
    const interval = window.setInterval(() => {
      window.go.main.App.GetXochitlStatus().then(s => { setXochitlRunning(s.running); setXoviActive(s.xoviActive) }).catch(() => {})
    }, 60000)
    return () => clearInterval(interval)
  }, [connectionStatus, commandRunning])



  useEffect(() => {
    if (activeTab === 'mods' && !tabVisibility.mods) {
      setActiveTab('maintenance')
    }
    if (activeTab === 'utilities' && !tabVisibility.utilities) {
      setActiveTab('maintenance')
    }
  }, [tabVisibility.mods, tabVisibility.utilities, activeTab])

  const resetOperationState = () => {
    setInstallQueue(new Set())
    setUninstallQueue(new Set())
    setOutput('')
    setHashtabMismatch(null)
    setHashtabMissing(false)
    setPrerelease(null)
    setOsMismatchDetected(false)
    osMismatchDetectedRef.current = false
    setOsUpgradeDetected(false)
    setCompatibilityStatus(null)
    setStoredOsVersion('')
    setCurrentOsVersion('')
    setChecklistLoading(false)
    setWarningsChecked(false)
    setBootstrapping(false)
    setBootstrapOutput('')
    setBootstrapError(null)
    setVellumUninstalling(false)
    setVellumUninstallOutput('')
    setVellumBrokenInstall(null)
    setVellumCleaning(false)
    setConnectionError(null)
    setReconnectAttempt(0)
    setWriteableRootBusy(false)
    setUpgradesAvailable(false)
    setSettingTimezone(false)
    settingTimezoneRef.current = false
    setCommandRunning(false)
    runningSystemTaskRef.current = null
    setRunningSystemTask(null)
    setInstalling(false)
    setUninstalling(false)
    setRunningReenable(false)
    setXochitlRunning(true)
  }
  resetOperationStateRef.current = resetOperationState

  const refreshDeviceState = async (deviceType: string) => {
    const info = await window.go.main.App.GetDeviceInfo()
    setDeviceInfo(info)

    const arch = await window.go.main.App.GetDeviceArchitecture(deviceType)
    const filteredPkgs = await window.go.main.App.GetPackages(deviceType, info.firmware || '', arch)
    setPackages(filteredPkgs || [])

    const installedResult = await window.go.main.App.GetInstalledPackagesWithOsCheck()
    debugLog('refreshDeviceState: GetInstalledPackagesWithOsCheck result:', installedResult)
    const installedVersions = await window.go.main.App.GetInstalledPackagesWithVersions()
    setInstalledPackages(new Map(Object.entries(installedVersions || {})))
    if (installedResult.osUpgraded) {
      debugLog('refreshDeviceState: osUpgradeDetected=true', installedResult.prevVersion, '->', installedResult.newVersion)
      setOsUpgradeDetected(true)
      setPrevOsVersion(installedResult.prevVersion)
      setNewOsVersion(installedResult.newVersion)
    }

    const maintCmds = await window.go.main.App.GetAllMaintenanceCommands()
    setMaintenanceCommands(maintCmds || {})
  }
  refreshDeviceStateRef.current = refreshDeviceState


  useEffect(() => {
    if (pendingXoviInfo === null) {
      setTripletapDecision(null)
      setXoviAckChecked(false)
    }
  }, [pendingXoviInfo])


  const handleDisconnect = async () => {
    await window.go.main.App.Disconnect()
    resetOperationState()
    setDeviceInfo({})
    setInstalledPackages(new Map())
    setVellumInstalled(null)
    setStep('connect')
    setDevice('')
    setConnectedDeviceId('')
    setConnectionStatus('connected')
    setGuideOffer(null)
    setGuideInstalling(false)

    const devices = await window.go.main.App.GetSavedDevices()
    setSavedDevices(devices || [])
  }

  const handleSettingsDialogOpenChange = (open: boolean) => {
    setShowSettingsDialog(open)
    if (!open) {
      setVellumUninstallOutput('')
      setVellumUninstallError(null)
      setVellumUninstallSuccess(false)
    }
  }

  const handleSaveSettings = async (newTabVisibility: Record<string, boolean>, newProxyMode: boolean, newSuppressSystemFileWarnings: boolean, newPreventSleep: boolean, newTheme: string, newTerminalTheme: string, newEditorTheme: string, newCheckForUpdates: boolean, newSSHAgentSocketPath: string) => {
    const wasCheckForUpdatesOff = !checkForUpdates
    setTabVisibility(newTabVisibility)
    setProxyMode(newProxyMode)
    setSuppressSystemFileWarnings(newSuppressSystemFileWarnings)
    setPreventSleep(newPreventSleep)
    setTheme(newTheme)
    localStorage.setItem('theme', newTheme)
    await applyThemeWithPortal(newTheme)
    setTerminalTheme(newTerminalTheme)
    setEditorTheme(newEditorTheme)
    setCheckForUpdates(newCheckForUpdates)
    setSSHAgentSocketPath(newSSHAgentSocketPath)
    await window.go.main.App.SaveSettings(newTabVisibility, newProxyMode, newSuppressSystemFileWarnings, newPreventSleep, newTheme, newTerminalTheme, newEditorTheme, newCheckForUpdates, newSSHAgentSocketPath)

    const agentAvail = await window.go.main.App.IsSSHAgentAvailable().catch(() => false)
    setSSHAgentAvailable(!!agentAvail)

    if (wasCheckForUpdatesOff && newCheckForUpdates) {
      window.go.main.App.CheckForAppUpdate().then((result) => {
        if (result.updateAvailable && result.latestVersion) {
          toast.info(`Update available: ${result.latestVersion}`, {
            description: `You're running ${result.currentVersion}`,
            action: {
              label: 'View Release',
              onClick: () => window.runtime.BrowserOpenURL(result.releaseURL)
            },
            duration: 6000,
          })
        }
      }).catch(() => {})
    }
  }

  const handleEnableProxyModeFromModal = async () => {
    setProxyMode(true)
    await window.go.main.App.SaveSettings(tabVisibility, true, suppressSystemFileWarnings, preventSleep, theme, terminalTheme, editorTheme, checkForUpdates, sshAgentSocketPath)
    setShowDnsErrorModal(false)
  }

  const handleUninstallVellum = (removeAllPackages: boolean) => {
    window.go.main.App.UninstallVellum(removeAllPackages)
  }


  const handleInstallQueue = async (allPackages?: string[]) => {
    const toInstall = allPackages || Array.from(installQueue).filter((name) => !installedPackages.has(name))

    if (toInstall.length === 0) {
      setInstallQueue(new Set())
      return
    }

    setShowProgressModal(true)
    setProgressModalType('install')
    setProgressIndex(0)
    setProgressTotal(toInstall.length)
    setProgressPercentage(0)
    setInstalling(true)
    setOutput('')
    setCommandContext('install')
    setLastOperationType('install')

    await window.go.main.App.InstallPackages(toInstall, device)
    setInstallQueue(new Set())
  }

  const handleUninstallQueue = async (allPackages?: string[]) => {
    const toUninstall = allPackages || Array.from(uninstallQueue)

    if (toUninstall.length === 0) {
      setUninstallQueue(new Set())
      return
    }

    setShowProgressModal(true)
    setProgressModalType('install')
    setProgressIndex(0)
    setProgressTotal(toUninstall.length)
    setProgressPercentage(0)
    setUninstalling(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')
    setLastOperationType('uninstall')

    await window.go.main.App.UninstallPackages(toUninstall, device)
    setUninstallQueue(new Set())
  }


  const selectPackageForOSRef = useRef<((name: string, targetOS: string, isCompatible?: boolean) => Promise<void>) | null>(null)

  const handleSelectPackageForOS = async (name: string, targetOS: string, isCompatible: boolean = true) => {
    if (selectPackageForOSRef.current) {
      await selectPackageForOSRef.current(name, targetOS, isCompatible)
    }
  }

  const handleTabChange = async (value: 'mods' | 'maintenance' | 'utilities') => {
    setActiveTab(value)
    if (value === 'mods') {
      if (connectionStatus === 'connected' && packages.length === 0) {
        try {
          await window.go.main.App.RefreshMetadata()
          await refreshDeviceStateRef.current(device)
        } catch (err) {
          debugLog('Failed to reload mod catalog:', err)
        }
      } else {
        await rescanAllPackages()
      }
    }
    if (connectionStatus === 'connected' && !commandRunning) {
      window.go.main.App.GetXochitlStatus().then(s => { setXochitlRunning(s.running); setXoviActive(s.xoviActive) }).catch(() => {})
    }
  }

  const handleRunReenable = async () => {
    setRunningReenable(true)
    startMaintenanceOperation()

    await window.go.main.App.RunReenable()

    setRunningReenable(false)
    setReenableStatus('ok')
    setRescanning(true)
    await rescanAllPackages()
    setRescanning(false)
    setCommandRunning(false)
    setCommandContext(null)
    setOsUpgradeDetected(false)
  }

  const handleRepairVellum = () => {
    if (bootstrapping) return
    setReinstallInProgress(true)
    window.go.main.App.BootstrapVellum()
  }

  const handleTimezoneChange = async (timezone: string) => {
    setSelectedTimezone(timezone)
    try {
      await window.go.main.App.SaveDeviceTimezone(timezone)
      const status = await window.go.main.App.GetTimezoneStatus()
      if (status.needsUpdate) {
        setTimezoneMismatch(status)
      } else {
        setTimezoneMismatch(null)
      }
    } catch (err) {
      console.error('Failed to save timezone:', err)
    }
  }

  const handleSetTimezone = () => {
    if (!selectedTimezone) return
    settingTimezoneRef.current = true
    setSettingTimezone(true)
    startMaintenanceOperation({ showModal: false, resetProgress: false })
    window.go.main.App.SetDeviceTimezone(selectedTimezone, device)
  }

  const handleChecklistUpgrade = async () => {
    setChecklistLoading(true)
    startMaintenanceOperation()

    await window.go.main.App.RunUpgrade()
  }

  const fetchUpdateServiceStatus = async () => {
    try {
      const status = await window.go.main.App.GetUpdateServiceStatus()
      setUpdateServiceStatus(status)
    } catch (err) {
      console.error('Failed to fetch update service status:', err)
    }
  }

  const handleSystemTask = async (taskId: string) => {
    runningSystemTaskRef.current = taskId
    setRunningSystemTask(taskId)
    startMaintenanceOperation({ showModal: false })

    await window.go.main.App.RunSystemTask(taskId, device)
  }

  const handleComponentMaintenance = async (componentId: string, commandId: string) => {
    const cmds = maintenanceCommands[componentId]
    if (!cmds) return

    const cmd = cmds.find(c => c.id === commandId)
    if (!cmd) return

    if (cmd.allowStop) {
      setCurrentRunningCommand({ componentId, commandId })
    }
    if (!cmd.hook) {
      startMaintenanceOperation()
    } else {
      setCommandContext('maintenance')
    }

    await window.go.main.App.RunMaintenanceCommand(componentId, commandId, device)
  }

  const handleStopCommand = async () => {
    manuallyStoppedRef.current = true
    await window.go.main.App.StopCommand()
    setCurrentRunningCommand(null)
    setCommandRunning(false)
    setMaintenanceOutput((prev) => prev + '\n=== Command stopped ===\n')
  }

  const handleCancelInstallation = async () => {
    await window.go.main.App.CancelInstallation()
    setInstalling(false)
    setUninstalling(false)
    setOutput((prev) => prev + '\n=== Cancelled ===\n')
    await rescanAllPackages()
  }

  const getModalTitle = () => {
    if (installing) return 'Installing Components'
    if (uninstalling) return 'Removing Components'
    if (commandRunning) return 'Running Maintenance'
    return 'Operation Complete'
  }

  const getProgressText = () => {
    if (progressModalType === 'install') {
      if (runningHookTitle) {
        return `${runningHookTitle}...`
      }
      if (installing || uninstalling) {
        let action: string
        if (uninstalling) {
          action = 'Removing'
        } else if (progressStatus === 'downloading' || progressStatus === 'transferring') {
          action = 'Downloading'
        } else {
          action = 'Installing'
        }
        return currentComponent
          ? `${action} ${currentComponent} (${progressIndex + 1} of ${progressTotal})`
          : `${action} components...`
      }
      const action = lastOperationType === 'uninstall' ? 'Removal' : 'Installation'
      return lastInstallSuccess === false
        ? `${action} failed`
        : `${action} complete!`
    } else if (progressModalType === 'maintenance') {
      if (rescanning) return 'Refreshing package list...'
      return commandRunning ? 'Running command...' : 'Command complete!'
    }
    return ''
  }

  const appContextValue = useMemo(() => ({
    device,
    deviceInfo,
    connectionStatus,
    connectionError,
    connectedDeviceId,
    installedPackages,
    setInstalledPackages,
    packages,
    setPackages,
    vellumInstalled,
    setVellumInstalled,
    commandRunning,
    setCommandRunning,
    installing,
    uninstalling,
    writeableRootBusy,
    appVersion,
    resolvedAppTheme,
    resolvedTerminalTheme,
    resolvedEditorTheme,
    tabVisibility,
    suppressSystemFileWarnings,
    refreshDeviceState,
    rescanAllPackages,
    startMaintenanceOperation,
    checkDnsProxyError,
  }), [
    device, deviceInfo, connectionStatus, connectionError, connectedDeviceId,
    installedPackages, packages, vellumInstalled,
    commandRunning, installing, uninstalling, writeableRootBusy,
    appVersion, resolvedAppTheme, resolvedTerminalTheme, resolvedEditorTheme,
    tabVisibility, suppressSystemFileWarnings,
  ])

  return (
    <AppProvider value={appContextValue}>
    <div className="min-h-screen p-6">
      <div className="space-y-6">
        <PageHeader
          device={device}
          deviceMachine={deviceInfo.machine}
          firmware={deviceInfo.firmware}
          onSettings={() => setShowSettingsDialog(true)}
          onDisconnect={handleDisconnect}
          onSupport={() => setShowSupportBundles(true)}
        />

        <ConnectPage
          step={step}
          setStep={setStep}
          savedDevices={savedDevices}
          setSavedDevices={setSavedDevices}
          setDevice={setDevice}
          deviceInfo={deviceInfo}
          setConnectedDeviceId={setConnectedDeviceId}
          onConnected={refreshDeviceState}
          initialKeys={availableKeys}
          initialAgentAvailable={sshAgentAvailable}
        />

        {/* Connection Status Banner */}
        {step !== 'connect' && connectionStatus !== 'connected' && (
          <div className="mb-4">
            <div className={`rounded-lg p-4 flex items-center justify-between gap-4 ${
              connectionStatus === 'failed'
                ? 'bg-destructive/10 border border-destructive/20'
                : 'bg-orange-500/10 border border-orange-500/20'
            }`}>
              <div className="flex items-center gap-3">
                {connectionStatus === 'reconnecting' ? (
                  <Loader2 className="h-5 w-5 text-orange-500 animate-spin flex-shrink-0" />
                ) : (
                  <WifiOff className={`h-5 w-5 flex-shrink-0 ${connectionStatus === 'failed' ? 'text-destructive' : 'text-orange-500'}`} />
                )}
                <span className="text-sm">
                  {connectionStatus === 'lost' && 'Connection lost. Attempting to reconnect...'}
                  {connectionStatus === 'reconnecting' && `Reconnecting... (attempt ${reconnectAttempt}/${reconnectMaxAttempts})`}
                  {connectionStatus === 'failed' && (connectionError || 'Connection failed')}
                </span>
              </div>
              <Button size="sm" onClick={handleDisconnect}>
                Disconnect
              </Button>
            </div>
          </div>
        )}

        {step !== 'connect' && (() => {
          const warningsData = {
            osUpgrade: osUpgradeDetected ? { prevVersion: prevOsVersion, newVersion: newOsVersion } : undefined,
            hashtabMismatch: hashtabMismatch ? { hashtabVersion: hashtabMismatch.hashtabVersion, firmwareVersion: hashtabMismatch.firmwareVersion } : undefined,
            timezoneMismatch: timezoneMismatch ? { deviceTimezone: timezoneMismatch.deviceTimezone, savedTimezone: timezoneMismatch.savedTimezone } : undefined,
            autoUpdatesEnabled: showAutoUpdateBanner,
            reenableNeeded: reenableStatus === 'needed',
            xoviNotRunning,
            hashtabMissing,
          }
          return (
            <BannerSwitcher banners={[
              {
                id: 'xochitl',
                priority: 1,
                severity: 'destructive',
                icon: AlertCircle,
                active: warningsChecked && !xochitlRunning && !commandRunning && !showProgressModal && connectionStatus === 'connected',
                content: (
                  <Banner
                    severity="destructive"
                    icon={AlertCircle}
                    title="reMarkable screen may be unresponsive"
                    actions={
                      installedPackages.has('xovi') ? (
                        <Button size="sm" onClick={() => setShowStartUIDialog(true)}>
                          Fix
                        </Button>
                      ) : (
                        <Button size="sm" onClick={() => handleSystemTask('restart-xochitl')}>
                          Start
                        </Button>
                      )
                    }
                  >
                    The xochitl UI service is not running on the reMarkable.
                  </Banner>
                ),
              },
              {
                id: 'warnings',
                priority: 3,
                severity: 'warning',
                icon: AlertTriangle,
                active: warningsChecked && hasActiveWarnings(warningsData),
                content: (
                  <ConsolidatedWarningBanner
                    warnings={warningsData}
                    onGoToMaintenance={() => setActiveTab('maintenance')}
                    onDismiss={() => {
                      setOsUpgradeDetected(false)
                      setHashtabMismatch(null)
                      setHashtabMissing(false)

                      setTimezoneMismatch(null)
                      setShowAutoUpdateBanner(false)
                      setReenableStatus('')
                    }}
                  />
                ),
              },
              {
                id: 'prerelease',
                priority: 2,
                severity: 'warning',
                icon: Cpu,
                active: warningsChecked && !!prerelease,
                content: (
                  <Banner
                    severity="warning"
                    icon={Cpu}
                    title="Beta OS"
                    actions={tabVisibility.utilities ? (
                      <Button
                        size="sm"
                        onClick={() => {
                          setActiveTab('utilities')
                          setShowSoftwareManager(true)
                        }}
                      >
                        Open OS Manager
                      </Button>
                    ) : undefined}
                  >
                    Your reMarkable is on {prerelease?.activeVersion}, newer than the latest stable release ({prerelease?.latestPublic}). reMarkable's <a href="https://support.remarkable.com/s/article/End-user-agreement-for-Opt-In-Beta" className="underline hover:text-foreground" onClick={(e) => { e.preventDefault(); window.runtime.BrowserOpenURL('https://support.remarkable.com/s/article/End-user-agreement-for-Opt-In-Beta') }}>beta program</a> doesn't permit installing third-party software, so packages aren't built for beta releases and you won't receive community support for issues on them. You can downgrade to a stable version in the OS Manager.
                  </Banner>
                ),
              },
              {
                id: 'guide',
                priority: 4,
                severity: 'info',
                icon: BookOpen,
                active: !!guideOffer,
                content: (
                  <UserGuideOffer
                    type={guideOffer ?? 'install'}
                    installing={guideInstalling}
                    onInstall={async () => {
                      setGuideInstalling(true)
                      try {
                        await window.go.main.App.InstallUserGuide()
                        setGuideOffer(null)
                        if (installedPackages.has('librarian')) {
                          await window.go.main.App.RescanLibrary()
                          toast.success('User guide installed on your reMarkable')
                        } else {
                          toast.success('User guide installed on your reMarkable')
                          setShowGuideRestartDialog(true)
                        }
                      } catch (err) {
                        toast.error('Failed to install guide: ' + (err instanceof Error ? err.message : String(err)))
                      } finally {
                        setGuideInstalling(false)
                      }
                    }}
                    onDismiss={() => setGuideOffer(null)}
                    onDismissPermanently={async () => {
                      setGuideOffer(null)
                      await window.go.main.App.DismissGuideOffer()
                    }}
                  />
                ),
              },
            ]} />
          )
        })()}

        {step !== 'connect' && (
          <>
          <Tabs value={activeTab} onValueChange={(v) => handleTabChange(v as 'mods' | 'maintenance' | 'utilities')}>
            <TabsList className={`grid w-full mb-4 grid-cols-${[tabVisibility.mods, true, tabVisibility.utilities].filter(Boolean).length}`}>
              {tabVisibility.mods && <TabsTrigger value="mods">Mods</TabsTrigger>}
              <TabsTrigger value="maintenance">Maintenance</TabsTrigger>
              {tabVisibility.utilities && <TabsTrigger value="utilities">Utilities</TabsTrigger>}
            </TabsList>

            {tabVisibility.mods && (
              <ModsTab
                warningsChecked={warningsChecked}
                osMismatchDetected={osMismatchDetected}
                storedOsVersion={storedOsVersion}
                currentOsVersion={currentOsVersion}
                checklistLoading={checklistLoading}
                compatibilityStatus={compatibilityStatus}
                bootstrapping={bootstrapping}
                bootstrapOutput={bootstrapOutput}
                bootstrapError={bootstrapError}
                vellumBrokenInstall={vellumBrokenInstall}
                vellumCleaning={vellumCleaning}
                installQueue={installQueue}
                setInstallQueue={setInstallQueue}
                uninstallQueue={uninstallQueue}
                setUninstallQueue={setUninstallQueue}
                queueError={queueError}
                setQueueError={setQueueError}
                refreshingPackages={refreshingPackages}
                setRefreshingPackages={setRefreshingPackages}
                setPendingInstallConfirm={setPendingInstallConfirm}
                setPendingUninstallConfirm={setPendingUninstallConfirm}
                setActiveTab={setActiveTab}
                handleChecklistUpgrade={handleChecklistUpgrade}
                selectPackageForOSRef={selectPackageForOSRef}
              />
            )}


            <MaintenanceTab
              systemTasks={systemTasks}
              maintenanceCommands={maintenanceCommands}
              updateServiceStatus={updateServiceStatus}
              xochitlRunning={xochitlRunning}
              xoviActive={xoviActive}
              hashtabMismatch={hashtabMismatch}
              hashtabMissing={hashtabMissing}
              reenableStatus={reenableStatus}
              upgradesAvailable={upgradesAvailable}
              timezoneMismatch={timezoneMismatch}
              selectedTimezone={selectedTimezone}
              deviceTimezone={deviceTimezone}
              runningSystemTask={runningSystemTask}
              currentRunningCommand={currentRunningCommand}
              settingTimezone={settingTimezone}
              showStartUIDialog={showStartUIDialog}
              setShowStartUIDialog={setShowStartUIDialog}
              handleSystemTask={handleSystemTask}
              handleComponentMaintenance={handleComponentMaintenance}
              handleRunReenable={handleRunReenable}
              handleTimezoneChange={handleTimezoneChange}
              handleSetTimezone={handleSetTimezone}
              handleSelectPackageForOS={handleSelectPackageForOS}
              runningReenable={runningReenable}
              onRepairVellum={handleRepairVellum}
              bootstrapping={bootstrapping}
            />

            {tabVisibility.utilities && (
              <UtilitiesTab
                activeTab={activeTab}
                handleSelectPackageForOS={handleSelectPackageForOS}
                onSettings={() => setShowSettingsDialog(true)}
                onDisconnect={handleDisconnect}
                showSoftwareManager={showSoftwareManager}
                onShowSoftwareManagerChange={setShowSoftwareManager}
              />
            )}
          </Tabs>

          </>
        )}
      </div>


      {/* Progress Modal */}
      <Dialog
        open={showProgressModal || pendingInstallConfirm !== null || pendingXoviInfo !== null || pendingUninstallConfirm !== null}
        onOpenChange={(open) => {
          if (!installing && !uninstalling && !commandRunning) {
            if (!open) {
              setShowProgressModal(false)
              setPendingInstallConfirm(null)
              setPendingXoviInfo(null)
              setPendingUninstallConfirm(null)
              setProgressModalType(null)
              setProgressIndex(0)
              setProgressTotal(0)
              setProgressPercentage(0)
              setOutput('')
              setMaintenanceOutput('')
              setLastInstallSuccess(null)
              setLastOperationType(null)
              if (connectionStatus === 'connected') {
                window.go.main.App.GetXochitlStatus().then(s => { setXochitlRunning(s.running); setXoviActive(s.xoviActive) }).catch(() => {})
              }
            }
          }
        }}
        closable={false}
      >
        <DialogContent className={(showProgressModal || installing || uninstalling || commandRunning) && pendingInstallConfirm === null && pendingUninstallConfirm === null && pendingXoviInfo === null ? "w-[90vw] max-w-none" : undefined}>
          {/* Confirmation step for install */}
          {pendingInstallConfirm !== null && !installing && !uninstalling && (() => {
            const requestedSet = new Set(pendingInstallConfirm.requested)
            const requiredBy: Record<string, string[]> = {}
            for (const pkgName of pendingInstallConfirm.packages) {
              if (!requestedSet.has(pkgName)) {
                for (const reqPkg of pendingInstallConfirm.packages) {
                  if (reqPkg === pkgName) continue
                  const pkgInfo = packages.find((p) => p.name === reqPkg)
                  if (pkgInfo?.depends?.includes(pkgName)) {
                    if (!requiredBy[pkgName]) requiredBy[pkgName] = []
                    requiredBy[pkgName].push(reqPkg)
                  }
                }
              }
            }
            return (
              <>
                <DialogHeader>
                  <DialogTitle>Install Packages</DialogTitle>
                  <DialogDescription>
                    The following {pendingInstallConfirm.packages.length} package{pendingInstallConfirm.packages.length !== 1 ? 's' : ''} will be installed:
                  </DialogDescription>
                </DialogHeader>
                <div className="max-h-[40vh] overflow-y-auto overscroll-y-contain">
                  <ul className="space-y-1 text-sm">
                    {[...pendingInstallConfirm.packages].sort().map((name) => (
                      <li key={name}>
                        {name}
                        {requiredBy[name] && (
                          <span className="text-muted-foreground ml-2">
                            (required by {requiredBy[name].join(', ')})
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setPendingInstallConfirm(null)}>
                    Cancel
                  </Button>
                  <Button onClick={() => {
                    const pkgs = pendingInstallConfirm.packages
                    setPendingInstallConfirm(null)
                    if (pkgs.includes('xovi') && !installedPackages.has('xovi')) {
                      setPendingXoviInfo(pkgs)
                    } else {
                      handleInstallQueue(pkgs)
                    }
                  }}>
                    Install ({pendingInstallConfirm.packages.length})
                  </Button>
                </DialogFooter>
              </>
            )
          })()}

          {/* Pre-install xovi info */}
          {pendingXoviInfo !== null && !installing && !uninstalling && (() => {
            const tripletapInstalled = installedPackages.has('tripletap')
            const tripletapQueued = pendingXoviInfo.includes('tripletap')
            const showAckScreen = tripletapQueued || tripletapInstalled
            return (
              <>
                <DialogHeader>
                  <DialogTitle className="flex items-center gap-2">
                    <AlertTriangle className="h-5 w-5 text-yellow-500" />
                    Mods Require Manual Start After Every Reboot
                  </DialogTitle>
                  <DialogDescription className="pt-2">
                    UI mods don't auto-start after reboot. Forcing auto-start can prevent your device from booting.
                  </DialogDescription>
                </DialogHeader>

                {!showAckScreen ? (
                  <div className="space-y-3">
                    <p className="text-sm font-medium">Would you like to install tripletap?</p>
                    <RadioGroup
                      value={tripletapDecision ?? ''}
                      onValueChange={(v) => setTripletapDecision(v as 'yes' | 'no')}
                      className="gap-2"
                    >
                      <label
                        htmlFor="tt-yes"
                        className={`block rounded-md border p-3 cursor-pointer transition-colors hover:bg-accent ${tripletapDecision === 'yes' ? 'border-primary bg-accent' : ''}`}
                      >
                        <RadioGroupItem value="yes" id="tt-yes" className="sr-only" />
                        <div className="flex items-center justify-between gap-2 min-h-[24px]">
                          <span className="text-sm font-semibold">Yes, install tripletap</span>
                          <Badge variant="outline">Recommended</Badge>
                        </div>
                        <p className="text-sm text-muted-foreground mt-1">Triple-press the power button after every boot to start mods.</p>
                      </label>

                      <label
                        htmlFor="tt-no"
                        className={`block rounded-md border p-3 cursor-pointer transition-colors hover:bg-accent ${tripletapDecision === 'no' ? 'border-primary bg-accent' : ''}`}
                      >
                        <RadioGroupItem value="no" id="tt-no" className="sr-only" />
                        <div className="flex items-center min-h-[24px]">
                          <span className="text-sm font-semibold">No, I'll start mods from my computer</span>
                        </div>
                        <p className="text-sm text-muted-foreground mt-1">
                          Open reManager and run <strong className="font-semibold text-foreground">Start UI with Mods</strong> from Maintenance after every boot, or start via SSH.
                        </p>
                      </label>
                    </RadioGroup>

                    <DialogFooter>
                      <Button variant="outline" onClick={() => setPendingXoviInfo(null)}>
                        Cancel
                      </Button>
                      <Button
                        disabled={tripletapDecision === null}
                        onClick={() => {
                          const pkgs = tripletapDecision === 'yes'
                            ? [...pendingXoviInfo, 'tripletap']
                            : pendingXoviInfo
                          setPendingXoviInfo(null)
                          handleInstallQueue(pkgs)
                        }}
                      >
                        Continue
                      </Button>
                    </DialogFooter>
                  </div>
                ) : (
                  <div className="space-y-3">
                    <p className="text-sm font-medium">How to start mods after reboot</p>
                    <div className="rounded-md border bg-accent p-3">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-semibold">Tripletap</span>
                        <Badge variant="outline" className="gap-1">
                          <Check className="h-3 w-3" />
                          {tripletapInstalled ? 'Installed' : 'Queued for install'}
                        </Badge>
                      </div>
                      <p className="text-sm text-muted-foreground mt-1">Triple-press the power button after every boot to start mods.</p>
                    </div>
                    <p className="border-t pt-2.5 text-sm text-muted-foreground">
                      Can also be started via reManager: Maintenance → Start UI with Mods.
                    </p>

                    <label className="flex items-start gap-3 rounded-md border bg-muted p-3 cursor-pointer">
                      <Checkbox
                        checked={xoviAckChecked}
                        onCheckedChange={(v) => setXoviAckChecked(v === true)}
                        className="mt-0.5"
                      />
                      <span className="text-sm">I'll start my mods manually after every reboot.</span>
                    </label>

                    <DialogFooter>
                      <Button variant="outline" onClick={() => setPendingXoviInfo(null)}>
                        Cancel
                      </Button>
                      <Button
                        disabled={!xoviAckChecked}
                        onClick={() => {
                          const pkgs = pendingXoviInfo
                          setPendingXoviInfo(null)
                          handleInstallQueue(pkgs)
                        }}
                      >
                        Continue
                      </Button>
                    </DialogFooter>
                  </div>
                )}
              </>
            )
          })()}

          {/* Confirmation step for uninstall */}
          {pendingUninstallConfirm !== null && !installing && !uninstalling && (() => {
            const worldDeps = pendingUninstallConfirm.worldDeps
            const dependsOn: Record<string, string[]> = {}
            if (pendingUninstallConfirm.blocked) {
              for (const [pkg, dependents] of Object.entries(pendingUninstallConfirm.blocked)) {
                for (const dep of dependents) {
                  if (!dependsOn[dep]) dependsOn[dep] = []
                  dependsOn[dep].push(pkg)
                }
              }
            }

            const selected = pendingUninstallConfirm.selected
            const additional = pendingUninstallConfirm.packages.filter(pkg => !selected.includes(pkg))

            return (
              <>
                <DialogHeader>
                  <DialogTitle className="flex items-center gap-2">
                    {(additional.length > 0 || (worldDeps && worldDeps.length > 0)) && (
                      <AlertTriangle className="h-5 w-5 text-yellow-500" />
                    )}
                    Uninstall Packages
                  </DialogTitle>
                  <DialogDescription>
                    The following {pendingUninstallConfirm.packages.length} package{pendingUninstallConfirm.packages.length !== 1 ? 's' : ''} will be removed:
                  </DialogDescription>
                </DialogHeader>
                <div className="max-h-[40vh] overflow-y-auto overscroll-y-contain space-y-4">
                  {worldDeps && worldDeps.length > 0 ? (
                    <>
                      <div>
                        <ul className="space-y-1 text-sm">
                          {[...pendingUninstallConfirm.packages].sort().map((name) => (
                            <li key={name}>{name}</li>
                          ))}
                        </ul>
                      </div>
                    </>
                  ) : (
                    <>
                      <div>
                        <p className="font-medium text-sm mb-2">Selected for removal:</p>
                        <ul className="space-y-1 text-sm">
                          {[...selected].sort().map((name) => (
                            <li key={name}>{name}</li>
                          ))}
                        </ul>
                      </div>

                      {additional.length > 0 && (
                        <div>
                          <p className="font-medium text-sm mb-2">Will also be removed:</p>
                          <ul className="space-y-1 text-sm">
                            {[...additional].sort().map((name) => (
                              <li key={name}>
                                {name}
                                {dependsOn[name] && (
                                  <span className="text-muted-foreground"> (requires {dependsOn[name].join(', ')})</span>
                                )}
                              </li>
                            ))}
                          </ul>
                        </div>
                      )}
                    </>
                  )}
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setPendingUninstallConfirm(null)}>
                    Cancel
                  </Button>
                  <Button variant="destructive" onClick={() => {
                    const selected = pendingUninstallConfirm.selected
                    setPendingUninstallConfirm(null)
                    handleUninstallQueue(selected)
                  }}>
                    Uninstall ({pendingUninstallConfirm.packages.length})
                  </Button>
                </DialogFooter>
              </>
            )
          })()}

          {/* Progress step */}
          {(showProgressModal || installing || uninstalling || commandRunning) && pendingInstallConfirm === null && pendingUninstallConfirm === null && (
            <ProgressModal
              title={getModalTitle()}
              progressText={getProgressText()}
              percentage={progressPercentage}
              terminalOutput={progressModalType === 'maintenance' ? maintenanceOutput : output}
              isComplete={!installing && !uninstalling && !commandRunning}
              onClose={() => {
                setShowProgressModal(false)
                setOutput('')
                setMaintenanceOutput('')
                setProgressModalType(null)
                setLastInstallSuccess(null)
                setLastOperationType(null)
                setRescanning(false)
              }}
              canStop={(installing || uninstalling) || currentRunningCommand !== null}
              onStop={(installing || uninstalling) ? handleCancelInstallation : handleStopCommand}
              terminalTheme={resolvedTerminalTheme}
              showProgress={progressModalType === 'install'}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Hook Dialog (e.g., Rebuild Qt Resources) — must render after Progress Modal so it layers on top */}
      <Dialog open={showRebuildDialog} onOpenChange={(open) => {
        setShowRebuildDialog(open)
        if (!open && !dialogRespondedRef.current) {
          dialogRespondedRef.current = true
          manuallyStoppedRef.current = true
          setDialogRequest(null)
          window.go.main.App.RespondToDialog('cancel')
        }
      }}>
        <DialogContent>
          {dialogRequest?.actions && dialogRequest.actions.length > 0 && !dialogRequest.infoOnly && !dialogRequest.confirmText ? (
            <>
              {/* Actions-only dialog (e.g., post-command "Start UI with/without Mods") */}
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <CheckCircle2 className="h-5 w-5 text-green-600 dark:text-green-400" />
                  {dialogRequest.title}
                </DialogTitle>
                <DialogDescription>{dialogRequest.message}</DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-2 pt-2">
                {dialogRequest.actions.map((action) => (
                  <Button
                    key={action.id}
                    variant={action.id === dialogRequest.actions[0]?.id ? 'default' : 'outline'}
                    className="w-full justify-center gap-2"
                    onClick={() => {
                      dialogRespondedRef.current = true
                      setShowRebuildDialog(false)
                      setDialogRequest(null)
                                  window.go.main.App.RespondToDialog(action.id)
                    }}
                  >
                    {action.label}
                  </Button>
                ))}
              </div>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  {dialogRequest?.infoOnly
                    ? <Info className="h-5 w-5 text-blue-500" />
                    : <AlertTriangle className="h-5 w-5 text-yellow-500" />
                  }
                  {dialogRequest?.title || 'Confirmation Required'}
                </DialogTitle>
                <DialogDescription>
                  <div className="space-y-3 pt-4">
                    {dialogRequest?.message?.split('\n\n').map((paragraph, idx) => (
                      <p key={idx}>{paragraph}</p>
                    ))}
                    {dialogRequest?.inProgressMessage && (
                      <p className="font-semibold text-foreground">{dialogRequest.inProgressMessage}</p>
                    )}
                    {dialogRequest?.note && (
                      <div className="flex items-start gap-2 rounded-lg border p-3 bg-muted/50">
                        <Info className="h-4 w-4 text-blue-500 flex-shrink-0 mt-0.5" />
                        <span className="text-xs text-muted-foreground">
                          {dialogRequest.note.split('\n').map((line, idx) => (
                            <span key={idx}>{idx > 0 && <br />}{line}</span>
                          ))}
                        </span>
                      </div>
                    )}
                    {dialogRequest?.steps && dialogRequest.steps.length > 0 && (
                      <>
                        <p className="font-medium text-foreground">This will:</p>
                        <ol className="list-decimal list-outside space-y-1 text-sm pl-8">
                          {dialogRequest.steps.map((step, idx) => (
                            <li key={idx}>{step}</li>
                          ))}
                        </ol>
                      </>
                    )}
                  </div>
                </DialogDescription>
              </DialogHeader>

              <DialogFooter>
                {dialogRequest?.infoOnly ? (
                  <Button
                    onClick={() => {
                      dialogRespondedRef.current = true
                      setShowRebuildDialog(false)
                      setDialogRequest(null)
                      window.go.main.App.RespondToDialog('confirm')
                    }}
                  >
                    {dialogRequest?.confirmText || 'Got it'}
                  </Button>
                ) : (
                  <>
                    <Button
                      variant="outline"
                      onClick={() => {
                        dialogRespondedRef.current = true
                        manuallyStoppedRef.current = true
                        setShowRebuildDialog(false)
                        setDialogRequest(null)
                        window.go.main.App.RespondToDialog('cancel')
                      }}
                    >
                      {dialogRequest?.cancelText || 'Cancel'}
                    </Button>
                    <Button
                      onClick={() => {
                        dialogRespondedRef.current = true
                        setShowRebuildDialog(false)
                        setDialogRequest(null)
                        if (commandContext === 'maintenance') {
                          startMaintenanceOperation()
                        }
                        window.go.main.App.RespondToDialog('confirm')
                      }}
                    >
                      {dialogRequest?.confirmText || 'Proceed'}
                    </Button>
                  </>
                )}
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Guide installed — restart UI prompt */}
      <Dialog open={showGuideRestartDialog} onOpenChange={setShowGuideRestartDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restart reMarkable UI?</DialogTitle>
            <DialogDescription>
              The user guide has been installed, but it won't appear on the reMarkable until the UI is restarted.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowGuideRestartDialog(false)}>
              Not now
            </Button>
            <Button onClick={async () => {
              setShowGuideRestartDialog(false)
              try {
                if (xoviActive) {
                  await window.go.main.App.RunMaintenanceCommand('xovi', 'start', device)
                } else {
                  await window.go.main.App.RestartXochitl()
                }
              } catch (err) {
                toast.error('Failed to restart UI: ' + (err instanceof Error ? err.message : String(err)))
              }
            }}>
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>


      <SettingsDialog
        open={showSettingsDialog}
        onOpenChange={handleSettingsDialogOpenChange}
        isConnected={!!device}
        vellumInstalled={vellumInstalled}
        tabVisibility={tabVisibility}
        proxyMode={proxyMode}
        suppressSystemFileWarnings={suppressSystemFileWarnings}
        preventSleep={preventSleep}
        theme={theme}
        terminalTheme={terminalTheme}
        editorTheme={editorTheme}
        checkForUpdates={checkForUpdates}
        suppressGuideOffer={suppressGuideOffer}
        sshAgentSocketPath={sshAgentSocketPath}
        onSaveSettings={handleSaveSettings}
        onToggleGuideOffer={async (enabled) => {
          if (enabled) {
            await window.go.main.App.EnableGuideOffer()
            setSuppressGuideOffer(false)
            if (device) {
              const status = await window.go.main.App.CheckUserGuide()
              if (!status.skipped) {
                if (!status.installed) {
                  setGuideOffer('install')
                } else if (status.needsUpdate) {
                  setGuideOffer('update')
                }
              }
            }
          } else {
            await window.go.main.App.DismissGuideOffer()
            setSuppressGuideOffer(true)
            setGuideOffer(null)
          }
        }}
        onUninstallVellum={handleUninstallVellum}
        uninstalling={vellumUninstalling}
        uninstallOutput={vellumUninstallOutput}
        uninstallError={vellumUninstallError}
        appVersion={appVersion}
      />

      <SupportBundlePage
        open={showSupportBundles}
        onOpenChange={setShowSupportBundles}
        isConnected={!!device}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
      />

      <Dialog
        open={reinstallInProgress && (bootstrapping || !!bootstrapError)}
        closable={!bootstrapping}
        onOpenChange={(open) => {
          if (!open) {
            setReinstallInProgress(false)
            setBootstrapError(null)
            setBootstrapOutput('')
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{bootstrapError ? 'Repair Failed' : 'Repairing Vellum...'}</DialogTitle>
            <DialogDescription>
              {bootstrapError
                ? 'The Vellum repair did not complete.'
                : 'Reinstalling Vellum with the latest version. Your installed mods are preserved.'}
            </DialogDescription>
          </DialogHeader>

          {bootstrapping && (
            <div className="flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span className="text-sm">Working...</span>
            </div>
          )}

          {bootstrapError && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{bootstrapError}</AlertDescription>
            </Alert>
          )}

          {(bootstrapping || bootstrapError) && bootstrapOutput && (
            <div className="h-[300px] rounded-lg overflow-hidden">
              <TerminalWithCopy output={bootstrapOutput} theme={resolvedTerminalTheme} />
            </div>
          )}

          {bootstrapError && (
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setReinstallInProgress(false)
                  setBootstrapError(null)
                  setBootstrapOutput('')
                }}
              >
                Close
              </Button>
              <Button onClick={handleRepairVellum}>Retry</Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      <VellumInstallSuccessDialog
        open={bootstrapSuccess}
        reinstalled={reinstallInProgress}
        onOpenChange={(open) => {
          if (!open) {
            setBootstrapSuccess(false)
            setBootstrapOutput('')
            setReinstallInProgress(false)
          }
        }}
      />

      <VellumUninstallSuccessDialog
        open={vellumUninstallSuccess}
        onOpenChange={(open) => {
          if (!open) {
            setVellumUninstallSuccess(false)
            setVellumUninstallOutput('')
          }
        }}
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
      />

      <Toaster position="bottom-right" richColors />
    </div>
    </AppProvider>
  )
}
