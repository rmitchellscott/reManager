import { useState, useEffect, useRef, useMemo } from 'react'
import { Toaster, toast } from 'sonner'
import { applyThemeWithPortal } from './main'
import { debugLog } from '@/lib/utils'
import { handleError, getUserFriendlyMessage } from '@/lib/errorMessages'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Skeleton } from '@/components/ui/skeleton'
import { ProgressModal } from '@/components/ProgressModal'
import { PackageDetailPanel } from '@/components/PackageDetailPanel'
import { ConsolidatedWarningBanner } from '@/components/ConsolidatedWarningBanner'
import { UpgradeChecklist } from '@/components/UpgradeChecklist'
import { VellumInstallPrompt } from '@/components/VellumInstallPrompt'
import { VellumInstallSuccessDialog } from '@/components/VellumInstallSuccessDialog'
import { VellumUninstallSuccessDialog } from '@/components/VellumUninstallSuccessDialog'
import { SettingsDialog } from '@/components/SettingsDialog'
import { InteractiveTerminal } from '@/components/InteractiveTerminal'
import { FileBrowser } from '@/components/FileBrowser'
import { ConfigEditor } from '@/components/ConfigEditor'
import { BackupRestoreDialog } from '@/components/BackupRestore'
import { CheckOSDialog } from '@/components/CheckOSDialog'
import { SupportBundlePage } from '@/components/SupportBundlePage'
import { DnsErrorModal } from '@/components/DnsErrorModal'
import { FilesystemRestoreErrorDialog } from '@/components/FilesystemRestoreErrorDialog'
import { ImportPDFDialog } from '@/components/ImportPDFDialog'
import { SoftwareManagerDialog } from '@/components/SoftwareManagerDialog'
import { TimezoneCombobox } from '@/components/TimezoneCombobox'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertTitle, AlertDescription, AlertAction } from '@/components/ui/alert'
import { Loader2, Unplug, Check, AlertTriangle, AlertCircle, Trash2, Plus, X, Search, Settings, WifiOff, Eye, EyeOff, RefreshCw, Info, LifeBuoy } from 'lucide-react'

interface PackageInfo {
  name: string
  version: string
  description: string
  upstreamAuthor: string
  categories: string[]
  url: string
  license: string
  devices: string[]
  depends: string[]
  conflicts: string[]
  osMin: string | null
  osMax: string | null
  osConstraints: { version: string; operator: '>=' | '<' | '>' | '<=' | '=' }[] | null
  compatible: boolean
}

interface MaintenanceCommandInfo {
  id: string
  label: string
  description: string
  requiresTerminal: boolean
  allowStop: boolean
  hook?: string
}

interface SystemTaskInfo {
  id: string
  label: string
  description: string
  requiresTerminal: boolean
  needsWriteableRoot: boolean
}

interface SSHKey {
  path: string
  name: string
}

interface SavedDevice {
  id: string
  name: string
  host: string
  authType: 'password' | 'key' | 'agent'
  keyPath?: string
  lastConnected?: number
}

interface UpdateServiceStatus {
  enabled: boolean
  running: boolean
}

interface InstallProgress {
  component: string
  index: number
  total: number
  status: string
  message: string
}

interface InstallResult {
  success: boolean
  errors: string[]
  dnsError?: boolean
}

interface DialogRequest {
  title: string
  message: string
  steps: string[]
  confirmText: string
  inProgressMessage: string
  infoOnly: boolean
}

interface InstallSimulationResult {
  packages: string[]
  requested: string[]
}

interface UninstallSimulationResult {
  packages: string[]
  blocked: Record<string, string[]> | null
  recursivePackages: string[] | null
  worldDeps: string[] | null
  allAffected: string[] | null
}

interface InstalledPackagesResult {
  packages: string[]
  osUpgraded: boolean
  prevVersion: string
  newVersion: string
}

interface HashtabVersionStatus {
  installed: boolean
  hashtabVersion: string
  firmwareVersion: string
  needsRebuild: boolean
}

interface TimezoneStatus {
  deviceTimezone: string
  savedTimezone: string
  needsUpdate: boolean
}

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
          RunCommandWithOutput(cmd: string, requiresPTY: boolean): Promise<void>
          StopCommand(): Promise<void>
          StartShell(rows: number, cols: number): Promise<void>
          WriteToShell(data: string): Promise<void>
          ResizeShell(rows: number, cols: number): Promise<void>
          StopShell(): Promise<void>
          IsShellActive(): Promise<boolean>
          GetDeviceInfo(): Promise<Record<string, string>>
          GetUpdateServiceStatus(): Promise<UpdateServiceStatus>
          GetXochitlStatus(): Promise<boolean>
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
          GetSystemTasksInfo(): Promise<SystemTaskInfo[]>
          GetDeviceDisplayName(machine: string): Promise<string>
          GetDeviceArchitecture(deviceType: string): Promise<string>
          InstallPackages(packageNames: string[], deviceType: string): Promise<void>
          UninstallPackages(packageNames: string[], deviceType: string): Promise<void>
          SimulateInstall(packageNames: string[], deviceType: string): Promise<InstallSimulationResult>
          SimulateUninstall(packageNames: string[]): Promise<UninstallSimulationResult>
          RunMaintenanceCommand(pkgName: string, commandId: string, deviceType: string): Promise<void>
          RunSystemTask(taskId: string, deviceType: string): Promise<void>
          RespondToDialog(confirmed: boolean): Promise<void>
          CancelInstallation(): Promise<void>
          GetAppVersion(): Promise<string>
          CheckForAppUpdate(): Promise<{ updateAvailable: boolean; latestVersion: string; currentVersion: string; releaseURL: string; error?: string }>
          GetSettings(): Promise<{ tabVisibility: Record<string, boolean>; proxyMode: boolean; suppressSystemFileWarnings: boolean; preventSleep: boolean; theme: string; terminalTheme: string; editorTheme: string; checkForUpdates: boolean; sshAgentSocketPath: string }>
          SaveSettings(tabVisibility: Record<string, boolean>, proxyMode: boolean, suppressSystemFileWarnings: boolean, preventSleep: boolean, theme: string, terminalTheme: string, editorTheme: string, checkForUpdates: boolean, sshAgentSocketPath: string): Promise<void>
          GetSystemColorScheme(): Promise<string>
          UninstallVellum(removeAllPackages: boolean): Promise<void>
          CleanupBrokenVellum(): Promise<void>
          GetDeviceTimezone(): Promise<string>
          GetTimezoneStatus(): Promise<TimezoneStatus>
          SaveDeviceTimezone(timezone: string): Promise<void>
          SetDeviceTimezone(timezone: string, deviceType: string): Promise<void>
          ListDirectory(path: string): Promise<{ name: string; path: string; size: number; isDir: boolean; modTime: number; mode: string }[]>
          DownloadFile(remotePath: string): void
          UploadFile(remotePath: string): void
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
          InstallOSVersion(version: string): void
          CancelOSInstall(): void
          IsSleepScreenSupported(): Promise<boolean>
          GetSleepScreen(): Promise<string>
          SetSleepScreen(imagePath: string): Promise<void>
          ResetSleepScreen(): Promise<void>
          RestartXochitl(): Promise<void>
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
          SubmitProblemReport(description: string, email: string, bundleURL: string): Promise<void>
        }
      }
    }
    runtime: {
      EventsOn(eventName: string, callback: (...args: unknown[]) => void): () => void
      BrowserOpenURL(url: string): void
    }
  }
}

type Step = 'connect' | 'select' | 'install' | 'done'

export default function App() {
  const [step, setStep] = useState<Step>('connect')
  const [host, setHost] = useState('10.11.99.1')
  const [authType, setAuthType] = useState<'password' | 'key' | 'agent'>('password')
  const [sshAgentAvailable, setSSHAgentAvailable] = useState(false)
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showKeyPassphrase, setShowKeyPassphrase] = useState(false)
  const [availableKeys, setAvailableKeys] = useState<SSHKey[]>([])
  const [selectedKey, setSelectedKey] = useState<string>('')
  const [customKeyName, setCustomKeyName] = useState<string>('')
  const [keyPassphrase, setKeyPassphrase] = useState('')
  const [connecting, setConnecting] = useState(false)
  const [device, setDevice] = useState<string>('')
  const [deviceInfo, setDeviceInfo] = useState<Record<string, string>>({})
  const [installedPackages, setInstalledPackages] = useState<Map<string, string>>(new Map())
  const [refreshingPackages, setRefreshingPackages] = useState(false)
  const [vellumInstalled, setVellumInstalled] = useState<boolean | null>(null)
  const [warningsChecked, setWarningsChecked] = useState(false)
  const [bootstrapping, setBootstrapping] = useState(false)
  const [bootstrapOutput, setBootstrapOutput] = useState('')
  const [bootstrapError, setBootstrapError] = useState<string | null>(null)
  const [bootstrapSuccess, setBootstrapSuccess] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [output, setOutput] = useState('')
  const [currentComponent, setCurrentComponent] = useState('')
  const [progressStatus, setProgressStatus] = useState('')
  const [showRebuildDialog, setShowRebuildDialog] = useState(false)
  const [dialogRequest, setDialogRequest] = useState<DialogRequest | null>(null)
  const dialogRespondedRef = useRef(false)
  const [runningHookTitle, setRunningHookTitle] = useState<string | null>(null)
  const connectAttemptRef = useRef(0)

  const [activeTab, setActiveTab] = useState<'mods' | 'maintenance' | 'utilities'>('mods')
  const [installQueue, setInstallQueue] = useState<Set<string>>(new Set())
  const [uninstallQueue, setUninstallQueue] = useState<Set<string>>(new Set())
  const [pendingUninstall, setPendingUninstall] = useState<{
    componentIds: string[]
    componentNames: string[]
    dependents: Array<{ id: string; name: string }>
  } | null>(null)
  const [pendingOrphanRemoval, setPendingOrphanRemoval] = useState<{
    itemToRemove: string
    orphans: Array<{ id: string; name: string }>
  } | null>(null)
  const [uninstalling, setUninstalling] = useState(false)
  const [maintenanceOutput, setMaintenanceOutput] = useState('')
  const [commandRunning, setCommandRunning] = useState(false)
  const [rescanning, setRescanning] = useState(false)
  const [currentRunningCommand, setCurrentRunningCommand] = useState<{
    componentId: string
    commandId: string
  } | null>(null)
  const [updateServiceStatus, setUpdateServiceStatus] = useState<UpdateServiceStatus>({
    enabled: false,
    running: false,
  })
  const [showAutoUpdateBanner, setShowAutoUpdateBanner] = useState(false)
  const [xochitlRunning, setXochitlRunning] = useState(true)
  const [commandContext, setCommandContext] = useState<'install' | 'maintenance' | null>(null)
  const commandContextRef = useRef<'install' | 'maintenance' | null>(null)
  const runningSystemTaskRef = useRef<string | null>(null)
  const settingTimezoneRef = useRef(false)
  const manuallyStoppedRef = useRef(false)

  const [showSettingsDialog, setShowSettingsDialog] = useState(false)
  const [showSupportBundles, setShowSupportBundles] = useState(false)
  const [showFileBrowser, setShowFileBrowser] = useState(false)
  const [showImportPDF, setShowImportPDF] = useState(false)
  const [showSoftwareManager, setShowSoftwareManager] = useState(false)
  const [showConfigEditor, setShowConfigEditor] = useState(false)
  const [backupDialogMode, setBackupDialogMode] = useState<'backup' | 'restore' | null>(null)
  const [isTerminalRunning, setIsTerminalRunning] = useState(false)
  const [appVersion, setAppVersion] = useState('dev')
  const [tabVisibility, setTabVisibility] = useState<Record<string, boolean>>({
    mods: true,
    maintenance: true,
    utilities: true,
  })
  const [proxyMode, setProxyMode] = useState(true)
  const [suppressSystemFileWarnings, setSuppressSystemFileWarnings] = useState(false)
  const [preventSleep, setPreventSleep] = useState(true)
  const [theme, setTheme] = useState('system')
  const [terminalTheme, setTerminalTheme] = useState('match')
  const [editorTheme, setEditorTheme] = useState('match')
  const [checkForUpdates, setCheckForUpdates] = useState(true)
  const [sshAgentSocketPath, setSSHAgentSocketPath] = useState('')
  const [systemPrefersDark, setSystemPrefersDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  const [dnsErrorShown, setDnsErrorShown] = useState(false)
  const [showDnsErrorModal, setShowDnsErrorModal] = useState(false)
  const [vellumUninstalling, setVellumUninstalling] = useState(false)
  const [vellumUninstallOutput, setVellumUninstallOutput] = useState('')
  const [vellumUninstallError, setVellumUninstallError] = useState<string | null>(null)
  const [vellumUninstallSuccess, setVellumUninstallSuccess] = useState(false)
  const [vellumBrokenInstall, setVellumBrokenInstall] = useState<string[] | null>(null)
  const [vellumCleaning, setVellumCleaning] = useState(false)

  const [showProgressModal, setShowProgressModal] = useState(false)
  const [progressModalType, setProgressModalType] = useState<'install' | 'maintenance' | null>(null)
  const [progressIndex, setProgressIndex] = useState(0)
  const [progressTotal, setProgressTotal] = useState(0)
  const [progressPercentage, setProgressPercentage] = useState(0)

  const [packages, setPackages] = useState<PackageInfo[]>([])
  const [systemTasks, setSystemTasks] = useState<SystemTaskInfo[]>([])
  const [maintenanceCommands, setMaintenanceCommands] = useState<Record<string, MaintenanceCommandInfo[]>>({})

  const [savedDevices, setSavedDevices] = useState<SavedDevice[]>([])
  const [deviceSortMode, setDeviceSortMode] = useState<'recent' | 'alpha'>(() => {
    const saved = localStorage.getItem('deviceSortMode')
    return saved === 'alpha' ? 'alpha' : 'recent'
  })
  const [showAddForm, setShowAddForm] = useState(false)
  const [showSaveDeviceDialog, setShowSaveDeviceDialog] = useState(false)
  const [saveDeviceError, setSaveDeviceError] = useState('')
  const [deviceName, setDeviceName] = useState('')
  const [deviceToDelete, setDeviceToDelete] = useState<string | null>(null)
  const [editingDevice, setEditingDevice] = useState<SavedDevice | null>(null)
  const [connectingDeviceId, setConnectingDeviceId] = useState<string | null>(null)
  const [connectedDeviceId, setConnectedDeviceId] = useState('')
  const [queueError, setQueueError] = useState<string | null>(null)
  const [lastInstallSuccess, setLastInstallSuccess] = useState<boolean | null>(null)
  const [lastOperationType, setLastOperationType] = useState<'install' | 'uninstall' | null>(null)
  const [selectedPackage, setSelectedPackage] = useState<PackageInfo | null>(null)
  const [sidebarViewOnly, setSidebarViewOnly] = useState(false)
  const [sidebarIncompatible, setSidebarIncompatible] = useState(false)
  const [pendingInstallConfirm, setPendingInstallConfirm] = useState<{
    packages: string[]
    requested: string[]
  } | null>(null)
  const [pendingUninstallConfirm, setPendingUninstallConfirm] = useState<{
    selected: string[]
    packages: string[]
    blocked: Record<string, string[]> | null
    useRecursive: boolean
    worldDeps?: string[]
  } | null>(null)
  const [simulatingInstall, setSimulatingInstall] = useState(false)
  const [simulatingUninstall, setSimulatingUninstall] = useState(false)

  const [osUpgradeDetected, setOsUpgradeDetected] = useState(false)
  const [prevOsVersion, setPrevOsVersion] = useState('')
  const [newOsVersion, setNewOsVersion] = useState('')
  const [runningReenable, setRunningReenable] = useState(false)
  const [reenableStatus, setReenableStatus] = useState('')
  const [pendingPackageUpgrade, setPendingPackageUpgrade] = useState<string[] | null>(null)
  const [simulatingUpgrade, setSimulatingUpgrade] = useState(false)
  const [showNoUpgradesDialog, setShowNoUpgradesDialog] = useState(false)
  const [upgradesAvailable, setUpgradesAvailable] = useState(false)
  const [showCheckOSDialog, setShowCheckOSDialog] = useState(false)

  const [osMismatchDetected, setOsMismatchDetected] = useState(false)
  const osMismatchDetectedRef = useRef(false)
  const [storedOsVersion, setStoredOsVersion] = useState('')
  const [currentOsVersion, setCurrentOsVersion] = useState('')
  const [checklistLoading, setChecklistLoading] = useState(false)
  const [compatibilityStatus, setCompatibilityStatus] = useState<{
    installedPackages: string[]
    compatiblePackages: string[]
    incompatiblePackages: string[]
    currentOsVersion: string
    storedOsVersion: string
    fetchFailed: boolean
  } | null>(null)

  const [hashtabMismatch, setHashtabMismatch] = useState<HashtabVersionStatus | null>(null)

  const [timezoneMismatch, setTimezoneMismatch] = useState<TimezoneStatus | null>(null)
  const [selectedTimezone, setSelectedTimezone] = useState('')
  const [deviceTimezone, setDeviceTimezone] = useState('')
  const [settingTimezone, setSettingTimezone] = useState(false)
  const [runningSystemTask, setRunningSystemTask] = useState<string | null>(null)

  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'lost' | 'reconnecting' | 'failed'>('connected')
  const [reconnectAttempt, setReconnectAttempt] = useState(0)
  const [reconnectMaxAttempts, setReconnectMaxAttempts] = useState(0)
  const [connectionError, setConnectionError] = useState<string | null>(null)

  const [showFilesystemRestoreError, setShowFilesystemRestoreError] = useState(false)
  const [isRetryingFilesystemRestore, setIsRetryingFilesystemRestore] = useState(false)
  const [writeableRootBusy, setWriteableRootBusy] = useState(false)

  const [search, setSearch] = useState('')
  const [categoryFilter, setCategoryFilter] = useState('all')
  const [developerFilter, setDeveloperFilter] = useState('all')
  const [viewMode, setViewMode] = useState<'full' | 'compact'>(() => {
    const saved = localStorage.getItem('packageViewMode')
    return saved === 'compact' ? 'compact' : 'full'
  })

  const sortedDevices = useMemo(() => {
    return [...savedDevices].sort((a, b) => {
      if (deviceSortMode === 'alpha') {
        return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
      }
      return (b.lastConnected || 0) - (a.lastConnected || 0)
    })
  }, [savedDevices, deviceSortMode])

  const categories = useMemo(() => {
    const cats = new Set(packages.flatMap(p => p.categories || []))
    return Array.from(cats).sort()
  }, [packages])

  const developers = useMemo(() => {
    const devs = new Set(packages.map(p => p.upstreamAuthor).filter(Boolean))
    return Array.from(devs).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
  }, [packages])

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

  const installedFiltered = useMemo(() =>
    filteredPackages.filter(pkg => installedPackages.has(pkg.name)),
    [filteredPackages, installedPackages]
  )

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
    commandContextRef.current = commandContext
  }, [commandContext])

  useEffect(() => {
    localStorage.setItem('packageViewMode', viewMode)
  }, [viewMode])

  useEffect(() => {
    localStorage.setItem('deviceSortMode', deviceSortMode)
  }, [deviceSortMode])

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
        if (keys && keys.length > 0) {
          setSelectedKey(keys[0].path)
        } else {
          setSelectedKey('__other__')
        }
        setSSHAgentAvailable(!!agentAvail)

        setPackages(pkgs || [])
        setSystemTasks(tasks || [])
        setSavedDevices(devices || [])
        setAppVersion(version || 'dev')
        setTabVisibility(settings?.tabVisibility || { mods: true, maintenance: true, utilities: true })
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
        setSelectedKey('__other__')
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
      window.go.main.App.GetXochitlStatus().then(setXochitlRunning).catch(() => {})
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

  const handleKeySelect = async (value: string) => {
    if (value === '__other__') {
      const path = await window.go.main.App.SelectKeyFile()
      if (path) {
        setSelectedKey(path)
        const fileName = path.split('/').pop() || path
        setCustomKeyName(fileName)
      }
    } else {
      setSelectedKey(value)
      setCustomKeyName('')
    }
  }

  const resetOperationState = () => {
    setInstallQueue(new Set())
    setUninstallQueue(new Set())
    setOutput('')
    setHashtabMismatch(null)
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
    setShowFileBrowser(false)
    setShowConfigEditor(false)
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
    setSimulatingUpgrade(false)
    setXochitlRunning(true)
  }

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

    const maintCmds: Record<string, MaintenanceCommandInfo[]> = {}
    for (const pkg of filteredPkgs) {
      const cmds = await window.go.main.App.GetMaintenanceCommands(pkg.name)
      if (cmds && cmds.length > 0) {
        maintCmds[pkg.name] = cmds
      }
    }
    setMaintenanceCommands(maintCmds)
  }

  const handleConnectToSavedDevice = async (id: string) => {
    const thisAttempt = ++connectAttemptRef.current
    setConnecting(true)
    setConnectingDeviceId(id)

    try {
      const result = await window.go.main.App.ConnectToSavedDevice(id)

      if (thisAttempt !== connectAttemptRef.current) return

      if (result.success) {
        setConnectedDeviceId(id)
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        await refreshDeviceState(deviceType)
        setStep('select')
      } else {
        toast.error(result.code ? getUserFriendlyMessage(result) : handleError(result.message, 'Connection'))
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) return
      toast.error(handleError(err, 'Connection'))
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
        setConnectingDeviceId(null)
      }
    }
  }

  const handleDeleteClick = (id: string) => {
    setDeviceToDelete(id)
  }

  const handleConfirmDelete = async () => {
    if (!deviceToDelete) return
    try {
      await window.go.main.App.DeleteSavedDevice(deviceToDelete)
      setSavedDevices(prev => prev.filter(d => d.id !== deviceToDelete))
    } catch (err) {
      console.error('Failed to delete saved device:', err)
    }
    setDeviceToDelete(null)
  }

  const handleEditDevice = (device: SavedDevice) => {
    setEditingDevice(device)
    setDeviceName(device.name)
    setHost(device.host)
    setAuthType(device.authType as 'password' | 'key' | 'agent')
    if (device.authType === 'key' && device.keyPath) {
      setSelectedKey(device.keyPath)
    }
    setShowAddForm(true)
  }

  const handleSaveEditedDevice = async () => {
    if (!editingDevice) return
    try {
      const pw = authType === 'password' ? password : ''
      const kp = authType === 'key' ? selectedKey : ''
      const kpp = authType === 'key' ? keyPassphrase : ''

      await window.go.main.App.SaveDevice(editingDevice.id, deviceName, host, authType, pw, kp, kpp)

      const devices = await window.go.main.App.GetSavedDevices()
      setSavedDevices(devices || [])

      setEditingDevice(null)
      setShowAddForm(false)
      resetFormFields()
    } catch (err) {
      console.error('Failed to save device:', err)
    }
  }

  const handleCancelForm = () => {
    setShowAddForm(false)
    setEditingDevice(null)
    resetFormFields()
  }

  const resetFormFields = () => {
    setHost('10.11.99.1')
    setAuthType('password')
    setPassword('')
    setKeyPassphrase('')
    setDeviceName('')
    if (availableKeys.length > 0) {
      setSelectedKey(availableKeys[0].path)
    }
  }

  useEffect(() => {
    if (typeof window.runtime === 'undefined') {
      debugLog('window.runtime is undefined, events will not work')
      return
    }
    debugLog('Setting up event listeners')

    const unsubscribeOutput = window.runtime.EventsOn('command:output', (...args: unknown[]) => {
      const data = args[0] as string
      debugLog('Received command:output:', data)

      if (commandContextRef.current === 'maintenance') {
        setMaintenanceOutput((prev) => prev + data)
      } else {
        setOutput((prev) => prev + data)
      }
    })

    const unsubscribeDone = window.runtime.EventsOn('command:done', async (...args: unknown[]) => {
      const success = args[0] as boolean
      debugLog('Received command:done:', success)

      if (runningSystemTaskRef.current || settingTimezoneRef.current) {
        if (!success && !manuallyStoppedRef.current) {
          setMaintenanceOutput((prev) => prev + '\n[Command failed]\n')
          setShowProgressModal(true)
          runningSystemTaskRef.current = null
          settingTimezoneRef.current = false
          setCommandRunning(false)
          setRunningSystemTask(null)
          setSettingTimezone(false)
          const status = await window.go.main.App.GetUpdateServiceStatus()
          setUpdateServiceStatus(status)
          const xochitlStatus = await window.go.main.App.GetXochitlStatus()
          setXochitlRunning(xochitlStatus)
        }
        return
      }

      if (!success && manuallyStoppedRef.current) {
        manuallyStoppedRef.current = false
        setShowProgressModal(false)
        setMaintenanceOutput('')
        setProgressModalType(null)
        setCommandRunning(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
        return
      }
      if (!success) {
        if (commandContextRef.current === 'maintenance') {
          setMaintenanceOutput((prev) => prev + '\n[Command failed]\n')
          setShowProgressModal(true)
        } else {
          setOutput((prev) => prev + '\n[Command failed]\n')
        }
      }
      // Refresh status after maintenance commands
      if (commandContextRef.current === 'maintenance') {
        const [hashtabStatus, tzStatus] = await Promise.all([
          window.go.main.App.CheckHashtabVersion(),
          window.go.main.App.GetTimezoneStatus(),
        ])
        let updateStatus = await window.go.main.App.GetUpdateServiceStatus()
        const maxAttempts = 6
        for (let i = 0; i < maxAttempts && updateStatus.enabled !== updateStatus.running; i++) {
          await new Promise(r => setTimeout(r, 500))
          updateStatus = await window.go.main.App.GetUpdateServiceStatus()
        }
        setUpdateServiceStatus(updateStatus)
        const xochitlStatus = await window.go.main.App.GetXochitlStatus()
        setXochitlRunning(xochitlStatus)
        if (hashtabStatus.needsRebuild) {
          setHashtabMismatch(hashtabStatus)
        } else {
          setHashtabMismatch(null)
        }
        if (tzStatus.needsUpdate) {
          setTimezoneMismatch(tzStatus)
        } else {
          setTimezoneMismatch(null)
        }
        if (tzStatus.deviceTimezone) {
          setDeviceTimezone(tzStatus.deviceTimezone)
          if (!selectedTimezone) {
            setSelectedTimezone(tzStatus.savedTimezone || tzStatus.deviceTimezone)
          }
        }
        setCommandRunning(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
      } else {
        const xochitlStatus = await window.go.main.App.GetXochitlStatus()
        setXochitlRunning(xochitlStatus)
        setCommandRunning(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
      }
    })

    const unsubscribeProgress = window.runtime.EventsOn('install:progress', (...args: unknown[]) => {
      const progress = args[0] as InstallProgress
      debugLog('Received install:progress:', progress)
      setCurrentComponent(progress.component)
      setProgressStatus(progress.status)
      setProgressTotal(progress.total)

      setProgressIndex(progress.index)
      if (progress.total > 0 && progress.index >= 0) {
        setProgressPercentage(Math.round(((progress.index + 1) / progress.total) * 100))
      }

      if (progress.status === 'downloading' || progress.status === 'transferring' || progress.status === 'installing') {
        if (progress.message) {
          setOutput((prev) => prev + progress.message + '\n')
        }
      } else if (progress.status === 'completed') {
        setOutput((prev) => prev + `${progress.message}\n`)
      } else if (progress.status === 'error') {
        setOutput((prev) => prev + `Error: ${progress.message}\n`)
      }
    })

    const unsubscribeComplete = window.runtime.EventsOn('install:complete', async (...args: unknown[]) => {
      const result = args[0] as InstallResult
      debugLog('Received install:complete:', result)

      setLastInstallSuccess(result.success)

      if (result.success) {
        setOutput((prev) => prev + '\n=== Operation complete! ===\n')
        setProgressPercentage(100)
      } else {
        setOutput((prev) => prev + `\nErrors occurred:\n${result.errors.join('\n')}\n`)
      }

      // Show DNS error modal if proxy mode is disabled and DNS error detected (once per session)
      const currentProxyMode = await window.go.main.App.GetSettings().then(s => s.proxyMode)
      if (result.dnsError && !currentProxyMode && !dnsErrorShown) {
        setShowDnsErrorModal(true)
        setDnsErrorShown(true)
      }

      await rescanAllPackages()

      // Re-check OS compatibility after uninstall (packages may now be compatible)
      if (osMismatchDetectedRef.current) {
        window.go.main.App.GetPackageCompatibilityStatus().then(status => {
          setCompatibilityStatus(status)
        }).catch(() => {})
      }

      // Re-check hashtab version (installs may update the hashtab)
      const hashtabStatus = await window.go.main.App.CheckHashtabVersion()
      if (hashtabStatus.needsRebuild) {
        setHashtabMismatch(hashtabStatus)
      } else {
        setHashtabMismatch(null)
      }

      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunning(xochitlStatus)

      setInstalling(false)
      setUninstalling(false)
      setCommandRunning(false)
      setCurrentComponent('')
      setProgressStatus('')
      setCommandContext(null)
      setDialogRequest(null)
      setRunningHookTitle(null)
      setInstallQueue(new Set())
    })

    const unsubscribeDialog = window.runtime.EventsOn('hook:dialog', (...args: unknown[]) => {
      const dialog = args[0] as DialogRequest
      debugLog('Received hook:dialog:', dialog)
      dialogRespondedRef.current = false
      setDialogRequest(dialog)
      setShowRebuildDialog(true)
    })

    const unsubscribeHookStarted = window.runtime.EventsOn('hook:started', (...args: unknown[]) => {
      const data = args[0] as { title: string }
      setRunningHookTitle(data.title)
    })

    const unsubscribeBootstrapPrompt = window.runtime.EventsOn('vellum:bootstrap-prompt', () => {
      debugLog('Received vellum:bootstrap-prompt')
      setVellumInstalled(false)
    })

    const unsubscribeBootstrapStart = window.runtime.EventsOn('vellum:bootstrap-start', () => {
      debugLog('Received vellum:bootstrap-start')
      setBootstrapping(true)
      setBootstrapOutput('')
      setBootstrapError(null)
      setBootstrapSuccess(false)
    })

    const unsubscribeBootstrapOutput = window.runtime.EventsOn('vellum:bootstrap-output', (...args: unknown[]) => {
      const line = args[0] as string
      setBootstrapOutput((prev) => prev + line)
    })

    const unsubscribeBootstrapComplete = window.runtime.EventsOn('vellum:bootstrap-complete', () => {
      debugLog('Received vellum:bootstrap-complete')
      setBootstrapping(false)
      setBootstrapSuccess(true)
      setVellumInstalled(true)
    })

    const unsubscribeBootstrapError = window.runtime.EventsOn('vellum:bootstrap-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:bootstrap-error:', errMsg)
      setBootstrapping(false)
      setBootstrapError(errMsg)
    })

    const unsubscribeVellumReady = window.runtime.EventsOn('vellum:ready', () => {
      debugLog('Received vellum:ready')
      setVellumInstalled(true)
    })

    const unsubscribeVellumUninstallStart = window.runtime.EventsOn('vellum:uninstall-start', () => {
      debugLog('Received vellum:uninstall-start')
      setVellumUninstalling(true)
      setVellumUninstallOutput('')
      setVellumUninstallError(null)
      setVellumUninstallSuccess(false)
    })

    const unsubscribeVellumUninstallOutput = window.runtime.EventsOn('vellum:uninstall-output', (...args: unknown[]) => {
      const line = args[0] as string
      setVellumUninstallOutput((prev) => prev + line)
    })

    const unsubscribeVellumUninstallComplete = window.runtime.EventsOn('vellum:uninstall-complete', () => {
      debugLog('Received vellum:uninstall-complete')
      setVellumUninstalling(false)
      setVellumInstalled(false)
      setShowSettingsDialog(false)
      setVellumUninstallSuccess(true)
    })

    const unsubscribeVellumUninstallError = window.runtime.EventsOn('vellum:uninstall-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:uninstall-error:', errMsg)
      setVellumUninstalling(false)
      setVellumUninstallError(errMsg)
    })

    const unsubscribeBrokenInstall = window.runtime.EventsOn('vellum:broken-install', (...args: unknown[]) => {
      const missing = args[0] as string[]
      debugLog('Received vellum:broken-install, missing:', missing)
      setVellumBrokenInstall(missing)
    })

    const unsubscribeCleanupStart = window.runtime.EventsOn('vellum:cleanup-start', () => {
      debugLog('Received vellum:cleanup-start')
      setVellumCleaning(true)
    })

    const unsubscribeCleanupComplete = window.runtime.EventsOn('vellum:cleanup-complete', () => {
      debugLog('Received vellum:cleanup-complete')
      setVellumCleaning(false)
      setVellumBrokenInstall(null)
    })

    const unsubscribeCleanupError = window.runtime.EventsOn('vellum:cleanup-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:cleanup-error:', errMsg)
      setVellumCleaning(false)
    })

    const unsubscribeConnectWarnings = window.runtime.EventsOn('connect:warnings', (...args: unknown[]) => {
      const w = args[0] as {
        osMismatch?: { prevVersion: string; newVersion: string }
        reenableStatus?: string
        hashtabMismatch?: HashtabVersionStatus
        autoUpdateEnabled?: { enabled: boolean; running: boolean }
        timezoneStatus?: TimezoneStatus
        timezoneMismatch?: TimezoneStatus
        xochitlNotRunning?: boolean
      }
      debugLog('Received connect:warnings:', w)
      setWarningsChecked(true)

      if (w.osMismatch) {
        debugLog('connect:warnings — setting osMismatchDetected=true', w.osMismatch)
        setOsMismatchDetected(true)
        osMismatchDetectedRef.current = true
        setStoredOsVersion(w.osMismatch.prevVersion)
        setCurrentOsVersion(w.osMismatch.newVersion)
      } else {
        debugLog('connect:warnings — no osMismatch, clearing')
        setOsMismatchDetected(false)
        osMismatchDetectedRef.current = false
      }
      if (w.reenableStatus) {
        setReenableStatus(w.reenableStatus)
      }
      if (w.hashtabMismatch) {
        setHashtabMismatch(w.hashtabMismatch)
      } else {
        setHashtabMismatch(null)
      }
      if (w.autoUpdateEnabled) {
        setShowAutoUpdateBanner(true)
      }
      if (w.timezoneStatus?.deviceTimezone) {
        setDeviceTimezone(w.timezoneStatus.deviceTimezone)
        setSelectedTimezone(w.timezoneStatus.savedTimezone || w.timezoneStatus.deviceTimezone)
      }
      if (w.timezoneMismatch) {
        setTimezoneMismatch(w.timezoneMismatch)
        setDeviceTimezone(w.timezoneMismatch.deviceTimezone)
        setSelectedTimezone(w.timezoneMismatch.savedTimezone || w.timezoneMismatch.deviceTimezone)
      }
      setXochitlRunning(!w.xochitlNotRunning)
    })

    const unsubscribeTimezoneComplete = window.runtime.EventsOn('timezone:complete', (...args: unknown[]) => {
      const timezone = args[0] as string
      debugLog('Timezone set complete:', timezone)
      settingTimezoneRef.current = false
      setSettingTimezone(false)
      setCommandRunning(false)
      setTimezoneMismatch(null)
      setDeviceTimezone(timezone)
      window.go.main.App.GetTimezoneStatus().then(status => {
        if (status.needsUpdate) {
          setTimezoneMismatch(status)
        }
      }).catch(() => {})
    })

    const unsubscribeTimezoneError = window.runtime.EventsOn('timezone:error', (...args: unknown[]) => {
      console.error('Timezone error:', args[0])
      settingTimezoneRef.current = false
      setSettingTimezone(false)
      setCommandRunning(false)
    })


    const unsubscribeUpgradeBlocked = window.runtime.EventsOn('upgrade:blocked', (...args: unknown[]) => {
      const compat = args[0] as {
        compatible: string[]
        incompatible: string[]
        noConstraint: string[]
        fetchFailed: boolean
      }
      debugLog('Received upgrade:blocked:', compat)
      setChecklistLoading(false)
    })

    const unsubscribeUpgradeError = window.runtime.EventsOn('upgrade:error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received upgrade:error:', errMsg)
      setChecklistLoading(false)
    })

    const unsubscribeUpgradeComplete = window.runtime.EventsOn('upgrade:complete', async (...args: unknown[]) => {
      const result = args[0] as { success: boolean; dnsError: boolean }
      debugLog('Received upgrade:complete:', result)
      setChecklistLoading(false)
      if (result.success) {
        setUpgradesAvailable(false)
        setOsMismatchDetected(false)
        osMismatchDetectedRef.current = false
        setCompatibilityStatus(null)
        setRescanning(true)
        try {
          const info = await window.go.main.App.GetDeviceInfo()
          setDeviceInfo(info)
          const arch = await window.go.main.App.GetDeviceArchitecture(device)
          const filteredPkgs = await window.go.main.App.GetPackages(device, info.firmware || '', arch)
          setPackages(filteredPkgs || [])
        } catch (err) {
          debugLog('Failed to refresh device info after upgrade:', err)
        }
        await rescanAllPackages()
        setRescanning(false)
      }
      setCommandRunning(false)
      setCommandContext(null)
      if (result.dnsError && !dnsErrorShown) {
        const currentProxyMode = await window.go.main.App.GetSettings().then(s => s.proxyMode)
        if (!currentProxyMode) {
          setShowDnsErrorModal(true)
          setDnsErrorShown(true)
        }
      }
    })

    const unsubscribePackageUpgradeComplete = window.runtime.EventsOn('package-upgrade:complete', async (...args: unknown[]) => {
      const result = args[0] as { success: boolean; dnsError: boolean }
      debugLog('Received package-upgrade:complete:', result)
      if (result.success) {
        setUpgradesAvailable(false)
        setRescanning(true)
        try {
          const info = await window.go.main.App.GetDeviceInfo()
          setDeviceInfo(info)
          const arch = await window.go.main.App.GetDeviceArchitecture(device)
          const filteredPkgs = await window.go.main.App.GetPackages(device, info.firmware || '', arch)
          setPackages(filteredPkgs || [])
        } catch (err) {
          debugLog('Failed to refresh device info after package upgrade:', err)
        }
        await rescanAllPackages()
        setRescanning(false)
      }
      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunning(xochitlStatus)
      setCommandRunning(false)
      setCommandContext(null)
      if (result.dnsError && !dnsErrorShown) {
        const currentProxyMode = await window.go.main.App.GetSettings().then(s => s.proxyMode)
        if (!currentProxyMode) {
          setShowDnsErrorModal(true)
          setDnsErrorShown(true)
        }
      }
    })

    const unsubscribeSystemTaskComplete = window.runtime.EventsOn('systemtask:complete', async (...args: unknown[]) => {
      const success = args[0] as boolean
      debugLog('Received systemtask:complete:', success)

      const [hashtabStatus, tzStatus] = await Promise.all([
        window.go.main.App.CheckHashtabVersion(),
        window.go.main.App.GetTimezoneStatus(),
      ])
      let updateStatus = await window.go.main.App.GetUpdateServiceStatus()
      const maxAttempts = 6
      for (let i = 0; i < maxAttempts && updateStatus.enabled !== updateStatus.running; i++) {
        await new Promise(r => setTimeout(r, 500))
        updateStatus = await window.go.main.App.GetUpdateServiceStatus()
      }
      setUpdateServiceStatus(updateStatus)
      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunning(xochitlStatus)
      if (hashtabStatus.needsRebuild) {
        setHashtabMismatch(hashtabStatus)
      } else {
        setHashtabMismatch(null)
      }
      if (tzStatus.needsUpdate) {
        setTimezoneMismatch(tzStatus)
      } else {
        setTimezoneMismatch(null)
      }
      if (tzStatus.deviceTimezone) {
        setDeviceTimezone(tzStatus.deviceTimezone)
        if (!selectedTimezone) {
          setSelectedTimezone(tzStatus.savedTimezone || tzStatus.deviceTimezone)
        }
      }
      runningSystemTaskRef.current = null
      setCommandRunning(false)
      setRunningSystemTask(null)
    })


    const unsubscribeConnectionLost = window.runtime.EventsOn('connection:lost', (...args: unknown[]) => {
      const data = args[0] as { reason: string; code?: string; deviceId: string }
      debugLog('Received connection:lost:', data)
      resetOperationState()
      setConnectionStatus('lost')
      setConnectionError(data.code ? getUserFriendlyMessage(data) : handleError(data.reason, 'Connection'))
    })

    const unsubscribeConnectionReconnecting = window.runtime.EventsOn('connection:reconnecting', (...args: unknown[]) => {
      const data = args[0] as { attempt: number; maxAttempts: number; deviceId: string }
      debugLog('Received connection:reconnecting:', data)
      setConnectionStatus('reconnecting')
      setReconnectAttempt(data.attempt)
      setReconnectMaxAttempts(data.maxAttempts)
    })

    const unsubscribeConnectionRestored = window.runtime.EventsOn('connection:restored', async (...args: unknown[]) => {
      const data = args[0] as { deviceId: string; device: string }
      debugLog('Received connection:restored:', data)
      resetOperationState()
      setConnectionStatus('connected')

      const deviceType = data.device || device
      if (data.device) {
        setDevice(data.device)
      }

      try {
        await refreshDeviceState(deviceType)
      } catch (err) {
        debugLog('Failed to refresh device info on reconnect:', err)
      }

      await rescanAllPackages()
    })

    const unsubscribeConnectionFailed = window.runtime.EventsOn('connection:failed', (...args: unknown[]) => {
      const data = args[0] as { reason: string; code?: string; deviceId: string }
      debugLog('Received connection:failed:', data)
      setConnectionStatus('failed')
      setConnectionError(data.code ? getUserFriendlyMessage(data) : handleError(data.reason, 'Connection'))
    })

    const unsubscribeFilesystemRestoreError = window.runtime.EventsOn('filesystem:restore-error', (...args: unknown[]) => {
      const data = args[0] as { message: string }
      debugLog('Received filesystem:restore-error:', data)
      setShowFilesystemRestoreError(true)
    })

    const unsubscribeWriteableRootBusy = window.runtime.EventsOn('writeable-root:busy', (...args: unknown[]) => {
      setWriteableRootBusy(args[0] as boolean)
    })

    const unsubscribeUpgradesAvailable = window.runtime.EventsOn('packages:upgrades-available', (...args: unknown[]) => {
      const data = args[0] as { packages: string[] }
      debugLog('Received packages:upgrades-available:', data)
      setUpgradesAvailable(true)
      toast.info(`${data.packages.length} package upgrade${data.packages.length !== 1 ? 's' : ''} available`, {
        action: {
          label: 'Go to Maintenance',
          onClick: () => setActiveTab('maintenance'),
        },
        duration: 6000,
      })
    })

    return () => {
      unsubscribeOutput()
      unsubscribeDone()
      unsubscribeProgress()
      unsubscribeComplete()
      unsubscribeDialog()
      unsubscribeHookStarted()
      unsubscribeBootstrapPrompt()
      unsubscribeBootstrapStart()
      unsubscribeBootstrapOutput()
      unsubscribeBootstrapComplete()
      unsubscribeBootstrapError()
      unsubscribeVellumReady()
      unsubscribeVellumUninstallStart()
      unsubscribeVellumUninstallOutput()
      unsubscribeVellumUninstallComplete()
      unsubscribeVellumUninstallError()
      unsubscribeBrokenInstall()
      unsubscribeCleanupStart()
      unsubscribeCleanupComplete()
      unsubscribeCleanupError()
      unsubscribeConnectWarnings()
      unsubscribeTimezoneComplete()
      unsubscribeTimezoneError()
      unsubscribeUpgradeBlocked()
      unsubscribeUpgradeError()
      unsubscribeUpgradeComplete()
      unsubscribePackageUpgradeComplete()
      unsubscribeSystemTaskComplete()
      unsubscribeConnectionLost()
      unsubscribeConnectionReconnecting()
      unsubscribeConnectionRestored()
      unsubscribeConnectionFailed()
      unsubscribeFilesystemRestoreError()
      unsubscribeWriteableRootBusy()
      unsubscribeUpgradesAvailable()
    }
  }, [])

  useEffect(() => {
    if (osMismatchDetected && !compatibilityStatus) {
      debugLog('useEffect: osMismatchDetected=true, compatibilityStatus=null — fetching compatibility')
      setChecklistLoading(true)
      window.go.main.App.GetPackageCompatibilityStatus().then(status => {
        debugLog('useEffect: GetPackageCompatibilityStatus result:', status)
        setCompatibilityStatus(status)
        setChecklistLoading(false)
      }).catch(() => {
        setChecklistLoading(false)
      })
    }
  }, [osMismatchDetected, compatibilityStatus])

  const handleConnect = async (saveAfterConnect: boolean) => {
    const thisAttempt = ++connectAttemptRef.current
    setConnecting(true)

    try {
      let result
      if (authType === 'agent') {
        result = await window.go.main.App.ConnectWithAgent(host)
      } else if (authType === 'key') {
        result = await window.go.main.App.ConnectWithKey(host, selectedKey, keyPassphrase)
      } else {
        result = await window.go.main.App.Connect(host, password)
      }

      if (thisAttempt !== connectAttemptRef.current) {
        return
      }

      if (result.success) {
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        await refreshDeviceState(deviceType)

        if (saveAfterConnect) {
          const info = await window.go.main.App.GetDeviceInfo()
          const displayName = getDisplayName(info.machine || '')
          setDeviceName(displayName || host)
          setShowSaveDeviceDialog(true)
        }

        setStep('select')
        setShowAddForm(false)
      } else {
        toast.error(result.code ? getUserFriendlyMessage(result) : handleError(result.message, 'Connection'))
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) {
        return
      }
      toast.error(handleError(err, 'Connection'))
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
      }
    }
  }

  const handleSaveDevice = async () => {
    setSaveDeviceError('')
    try {
      const pw = authType === 'password' ? password : ''
      const kp = authType === 'key' ? selectedKey : ''
      const kpp = authType === 'key' ? keyPassphrase : ''

      await window.go.main.App.SaveDevice('', deviceName, host, authType, pw, kp, kpp)


      const devices = await window.go.main.App.GetSavedDevices()
      setSavedDevices(devices || [])

      setShowSaveDeviceDialog(false)
    } catch (err) {
      const friendlyMessage = handleError(err, 'Save device')
      if (friendlyMessage.includes('keyring')) {
        const devices = await window.go.main.App.GetSavedDevices()
        setSavedDevices(devices || [])
      }
      setSaveDeviceError(friendlyMessage)
    }
  }

  const handleCancelConnect = async () => {
    connectAttemptRef.current++
    await window.go.main.App.CancelConnect()
    setConnecting(false)
  }

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

  const handleFilesystemRestoreRetry = async () => {
    setIsRetryingFilesystemRestore(true)
    try {
      await window.go.main.App.RetryRestoreFilesystem()
      setShowFilesystemRestoreError(false)
    } catch (err) {
      debugLog('Filesystem restore retry failed:', err)
    }
    setIsRetryingFilesystemRestore(false)
  }

  const handleFilesystemRestoreReboot = async () => {
    await window.go.main.App.RebootDevice()
    setShowFilesystemRestoreError(false)
  }

  const handleFilesystemRestoreDismiss = () => {
    setShowFilesystemRestoreError(false)
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

  const getDisplayName = (machine: string) => {
    if (machine.includes('reMarkable 1')) return 'reMarkable 1'
    if (machine.includes('reMarkable 2')) return 'reMarkable 2'
    if (machine.includes('Ferrari')) return 'Paper Pro'
    if (machine.includes('Chiappa')) return 'Paper Pro Move'
    return machine
  }

  const getConflict = (pkg: PackageInfo): string | null => {
    for (const conflict of pkg.conflicts) {
      if (installedPackages.has(conflict)) {
        return `Conflicts with installed: ${conflict}`
      }
      if (installQueue.has(conflict)) {
        return `Conflicts with queued: ${conflict}`
      }
    }
    for (const queuedName of installQueue) {
      const queuedPkg = packages.find(p => p.name === queuedName)
      if (queuedPkg?.conflicts.includes(pkg.name)) {
        return `Conflicts with queued: ${queuedName}`
      }
    }
    return null
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

  const handleSelectPackageForOS = async (name: string, targetOS: string, isCompatible: boolean = true) => {
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

  const handleRefreshPackages = async () => {
    setRefreshingPackages(true)
    try {
      await window.go.main.App.RefreshMetadata()
      if (device) {
        const info = await window.go.main.App.GetDeviceInfo()
        setDeviceInfo(info)
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

  const handleTabChange = async (value: 'mods' | 'maintenance' | 'utilities') => {
    setActiveTab(value)
    if (value === 'mods') {
      await rescanAllPackages()
    }
    if (connectionStatus === 'connected' && !commandRunning) {
      window.go.main.App.GetXochitlStatus().then(setXochitlRunning).catch(() => {})
    }
  }

  const handleRunReenable = async () => {
    setRunningReenable(true)
    setShowProgressModal(true)
    setProgressModalType('maintenance')
    setProgressPercentage(0)
    setCommandRunning(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')

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

  const handleCheckUpgrades = async () => {
    setSimulatingUpgrade(true)
    try {
      const result = await window.go.main.App.SimulatePackageUpgrade()
      if (result.hasUpgrades) {
        setPendingPackageUpgrade(result.packages)
      } else {
        setShowNoUpgradesDialog(true)
      }
    } catch (err) {
      console.error('Failed to check upgrades:', err)
    } finally {
      setSimulatingUpgrade(false)
    }
  }

  const confirmPackageUpgrade = async () => {
    setPendingPackageUpgrade(null)
    setShowProgressModal(true)
    setProgressModalType('maintenance')
    setMaintenanceOutput('')
    setCommandRunning(true)
    setCommandContext('maintenance')
    await window.go.main.App.RunPackageUpgrade()
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
    setProgressModalType('maintenance')
    setMaintenanceOutput('')
    setCommandRunning(true)
    setCommandContext('maintenance')
    window.go.main.App.SetDeviceTimezone(selectedTimezone, device)
  }

  const handleChecklistUpgrade = async () => {
    setChecklistLoading(true)
    setShowProgressModal(true)
    setProgressModalType('maintenance')
    setProgressPercentage(0)
    setCommandRunning(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')

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
    setProgressModalType('maintenance')
    setProgressPercentage(0)
    setCommandRunning(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')

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
    setShowProgressModal(true)
    setProgressModalType('maintenance')
    setProgressPercentage(0)
    setCommandRunning(true)
    setMaintenanceOutput('')
    setCommandContext('maintenance')

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

  return (
    <div className="min-h-screen p-6">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-foreground">reManager</h1>
            <p className="text-muted-foreground">Manage packages on your reMarkable</p>
          </div>
          <div className="flex items-center gap-2 text-sm">
            {device && (
              <span className="text-muted-foreground">
                {getDisplayName(deviceInfo.machine || device)} ({deviceInfo.firmware || 'unknown firmware'})
              </span>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="sm" onClick={() => setShowSettingsDialog(true)}>
                  <Settings className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Settings</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="sm" onClick={() => setShowSupportBundles(true)}>
                  <LifeBuoy className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Support</TooltipContent>
            </Tooltip>
            {device && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="sm" onClick={handleDisconnect}>
                    <Unplug className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Disconnect</TooltipContent>
              </Tooltip>
            )}
          </div>
        </div>

        {step === 'connect' && (
          <>
            {/* Show saved devices list OR add form */}
            {savedDevices.length > 0 && !showAddForm ? (
              <div className="space-y-4">
                <div className="flex justify-end">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setDeviceSortMode(m => m === 'recent' ? 'alpha' : 'recent')}
                      >
                        {deviceSortMode === 'recent' ? 'Recent' : 'A-Z'}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      {deviceSortMode === 'recent' ? 'Sort alphabetically' : 'Sort by recent'}
                    </TooltipContent>
                  </Tooltip>
                </div>
                {sortedDevices.map((savedDevice) => (
                  <Card key={savedDevice.id}>
                    <CardContent className="pt-6">
                      <div className="flex items-center justify-between">
                        <div className="space-y-1">
                          <h3 className="font-semibold text-lg">{savedDevice.name}</h3>
                          <p className="text-sm text-muted-foreground">{savedDevice.host}</p>
                        </div>
                        <div className="flex gap-2">
                          <Button
                            variant="outline"
                            onClick={() => handleEditDevice(savedDevice)}
                            disabled={connecting}
                          >
                            Edit
                          </Button>
                          <Button
                            variant="outline"
                            onClick={() => handleDeleteClick(savedDevice.id)}
                            disabled={connecting}
                          >
                            Remove
                          </Button>
                          {connecting && connectingDeviceId === savedDevice.id ? (
                            <Button onClick={handleCancelConnect} variant="outline">
                              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                              Cancel
                            </Button>
                          ) : (
                            <Button
                              onClick={() => handleConnectToSavedDevice(savedDevice.id)}
                              disabled={connecting}
                            >
                              Connect
                            </Button>
                          )}
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))}
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => setShowAddForm(true)}
                >
                  Add reMarkable
                </Button>
              </div>
            ) : (
              <Card>
                <CardHeader>
                  {editingDevice ? (
                    <Input
                      value={deviceName}
                      onChange={(e) => setDeviceName(e.target.value)}
                      placeholder="Device Name"
                      className="text-2xl font-semibold h-auto py-1 px-2 -mx-2"
                    />
                  ) : (
                    <CardTitle>Connect to reMarkable</CardTitle>
                  )}
                  <CardDescription>
                    Find your IP and password in Settings - General - Help - Copyrights and licenses.
                    <br />
                    Paper Pro and Paper Pro Move require{' '}
                    <button
                      onClick={() => window.runtime.BrowserOpenURL('https://support.remarkable.com/s/article/Developer-mode')}
                      className="underline hover:text-foreground">
                      developer mode
                    </button>{' '}
                    to be enabled.
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="host">IP Address</Label>
                    <Input
                      id="host"
                      value={host}
                      onChange={(e) => setHost(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && !connecting) {
                          const canConnect = authType === 'agent' ? true : authType === 'password' ? !!password : (!!selectedKey && selectedKey !== '__other__')
                          if (canConnect) handleConnect(true)
                        }
                      }}
                      placeholder="10.11.99.1"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label>Authentication</Label>
                    <RadioGroup
                      value={authType}
                      onValueChange={(value) => setAuthType(value as 'password' | 'key' | 'agent')}
                      className="flex gap-4"
                    >
                      <div className="flex items-center gap-2">
                        <RadioGroupItem value="password" id="auth-password" />
                        <Label htmlFor="auth-password" className="cursor-pointer font-normal">Password</Label>
                      </div>
                      <div className="flex items-center gap-2">
                        <RadioGroupItem value="key" id="auth-key" />
                        <Label htmlFor="auth-key" className="cursor-pointer font-normal">
                          SSH Key
                        </Label>
                      </div>
                      {sshAgentAvailable && (
                        <div className="flex items-center gap-2">
                          <RadioGroupItem value="agent" id="auth-agent" />
                          <Label htmlFor="auth-agent" className="cursor-pointer font-normal">
                            SSH Agent
                          </Label>
                        </div>
                      )}
                    </RadioGroup>
                  </div>

                  {authType === 'agent' ? (
                    <p className="text-sm text-muted-foreground">
                      Authentication will use keys from your running SSH agent.
                    </p>
                  ) : authType === 'password' ? (
                    <div className="space-y-2">
                      <Label htmlFor="password">SSH Password</Label>
                      <div className="relative">
                        <Input
                          id="password"
                          type={showPassword ? "text" : "password"}
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter' && !connecting && password) {
                              handleConnect(true)
                            }
                          }}
                          placeholder="Enter SSH password"
                          className="pr-10"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                        >
                          {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        </button>
                      </div>
                    </div>
                  ) : (
                    <>
                      <div className="space-y-2">
                        <Label htmlFor="sshKey">SSH Key</Label>
                        <Select value={selectedKey} onValueChange={handleKeySelect}>
                          <SelectTrigger>
                            <SelectValue placeholder="Select a key">
                              {customKeyName || availableKeys.find(k => k.path === selectedKey)?.name || 'Select a key'}
                            </SelectValue>
                          </SelectTrigger>
                          <SelectContent>
                            {availableKeys.map((key) => (
                              <SelectItem key={key.path} value={key.path}>
                                {key.name}
                              </SelectItem>
                            ))}
                            <SelectItem value="__other__">Other...</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="keyPassphrase">Key Passphrase (if encrypted)</Label>
                        <div className="relative">
                          <Input
                            id="keyPassphrase"
                            type={showKeyPassphrase ? "text" : "password"}
                            value={keyPassphrase}
                            onChange={(e) => setKeyPassphrase(e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' && !connecting) {
                                handleConnect(true)
                              }
                            }}
                            placeholder="Leave empty if not encrypted"
                            className="pr-10"
                          />
                          <button
                            type="button"
                            onClick={() => setShowKeyPassphrase(!showKeyPassphrase)}
                            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                          >
                            {showKeyPassphrase ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                          </button>
                        </div>
                      </div>
                    </>
                  )}

                  <div className="flex justify-between">
                    <div>
                      {savedDevices.length > 0 && (
                        <Button variant="outline" onClick={handleCancelForm} disabled={connecting}>
                          Cancel
                        </Button>
                      )}
                    </div>
                    <div className="flex gap-2">
                      {connecting ? (
                        <Button onClick={handleCancelConnect} variant="outline">
                          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                          Cancel
                        </Button>
                      ) : editingDevice ? (
                        <Button
                          onClick={handleSaveEditedDevice}
                          disabled={!deviceName.trim() || (authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey || selectedKey === '__other__')}
                        >
                          Save
                        </Button>
                      ) : (
                        <>
                          <Button
                            variant="outline"
                            onClick={() => handleConnect(false)}
                            disabled={authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey || selectedKey === '__other__'}
                          >
                            Connect
                          </Button>
                          <Button
                            onClick={() => handleConnect(true)}
                            disabled={authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey || selectedKey === '__other__'}
                          >
                            Save and Connect
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                </CardContent>
              </Card>
            )}
          </>
        )}

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

        {/* Xochitl Not Running Banner */}
        {step !== 'connect' && !xochitlRunning && !commandRunning && !showProgressModal && connectionStatus === 'connected' && (
          <div className="mb-4">
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>reMarkable screen may be unresponsive</AlertTitle>
              <AlertDescription>
                The xochitl UI service is not running on the reMarkable.
              </AlertDescription>
              <AlertAction className="top-1/2 -translate-y-1/2">
                <Button size="sm" variant="destructive" onClick={() => handleSystemTask('restart-xochitl')}>
                  Start
                </Button>
              </AlertAction>
            </Alert>
          </div>
        )}

        {/* Warning Banner */}
        {step !== 'connect' && warningsChecked && (
          <div className="mb-4">
            <ConsolidatedWarningBanner
              warnings={{
                osUpgrade: osUpgradeDetected ? { prevVersion: prevOsVersion, newVersion: newOsVersion } : undefined,
                hashtabMismatch: hashtabMismatch ? { hashtabVersion: hashtabMismatch.hashtabVersion, firmwareVersion: hashtabMismatch.firmwareVersion } : undefined,
                timezoneMismatch: timezoneMismatch ? { deviceTimezone: timezoneMismatch.deviceTimezone, savedTimezone: timezoneMismatch.savedTimezone } : undefined,
                autoUpdatesEnabled: showAutoUpdateBanner,
                reenableNeeded: reenableStatus === 'needed',
              }}
              onGoToMaintenance={() => setActiveTab('maintenance')}
              onDismiss={() => {
                setOsUpgradeDetected(false)
                setHashtabMismatch(null)
                setTimezoneMismatch(null)
                setShowAutoUpdateBanner(false)
                setReenableStatus('')
              }}
            />
          </div>
        )}

        {step !== 'connect' && (
          <>
          <Tabs value={activeTab} onValueChange={(v) => handleTabChange(v as 'mods' | 'maintenance' | 'utilities')}>
            <TabsList className={`grid w-full mb-4 grid-cols-${[tabVisibility.mods, true, tabVisibility.utilities].filter(Boolean).length}`}>
              {tabVisibility.mods && <TabsTrigger value="mods">Mods</TabsTrigger>}
              <TabsTrigger value="maintenance">Maintenance</TabsTrigger>
              {tabVisibility.utilities && <TabsTrigger value="utilities">Utilities</TabsTrigger>}
            </TabsList>

            {tabVisibility.mods && (
            <TabsContent value="mods">
              {vellumInstalled === null || (vellumInstalled && !warningsChecked) || (osMismatchDetected && !compatibilityStatus) ? (
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
              ) : osMismatchDetected && compatibilityStatus ? (
                <div className={uninstallQueue.size > 0 ? 'pb-48' : ''}>
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
                    onSelectPackage={handleSelectPackageForOS}
                  />
                </div>
              ) : (
              <div className={`space-y-6 ${(installQueue.size > 0 || uninstallQueue.size > 0) ? 'pb-48' : ''}`}>
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
                                        <span className="font-medium w-[120px] md:w-[160px] lg:w-[200px] xl:w-[240px] shrink-0 truncate">{pkg.name}</span>
                                        <span className="text-sm text-muted-foreground truncate">{pkg.description}</span>
                                      </div>
                                    ) : (
                                      <>
                                        <div className="flex items-center gap-2">
                                          <span className="font-medium">{pkg.name}</span>
                                          {(pkg.categories || []).map(cat => <Badge key={cat} variant="outline">{cat}</Badge>)}
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
                                    <TooltipContent>Does not support your current OS</TooltipContent>
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
                                const isQueued = installQueue.has(pkg.name)
                                const prevQueued = index > 0 && installQueue.has(availableFiltered[index - 1].name)
                                const nextQueued = index < availableFiltered.length - 1 && installQueue.has(availableFiltered[index + 1].name)
                                const conflict = getConflict(pkg)
                                return (
                                  <div key={pkg.name} className={`py-3 flex items-center gap-4 ${!pkg.compatible ? 'opacity-50' : ''} ${isQueued ? `border-l-4 border-primary pl-3 ${!prevQueued ? 'border-t' : ''} ${!nextQueued ? 'border-b' : ''}` : index % 2 === 1 ? 'bg-muted/50 hover:bg-muted' : 'hover:bg-muted/70'}`}>
                                  <div
                                    className="flex-1 min-w-0 cursor-pointer"
                                    onClick={() => { setSidebarViewOnly(false); setSidebarIncompatible(!pkg.compatible); setSelectedPackage(pkg) }}
                                  >
                                    {viewMode === 'compact' ? (
                                      <div className="flex items-center gap-2">
                                        <span className="font-medium w-[120px] md:w-[160px] lg:w-[200px] xl:w-[240px] shrink-0 truncate">{pkg.name}</span>
                                        <span className="text-sm text-muted-foreground truncate">{pkg.description}</span>
                                      </div>
                                    ) : (
                                      <>
                                        <div className="flex items-center gap-2">
                                          <span className="font-medium">{pkg.name}</span>
                                          {(pkg.categories || []).map(cat => <Badge key={cat} variant="outline">{cat}</Badge>)}
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
                                      <TooltipContent>Does not support your current OS</TooltipContent>
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
                      {packages.length === 0 ? 'No mods available' : 'No mods match your filters'}
                    </p>
                  )}

                  {/* Queue Error */}
                  {queueError && (
                    <div className="bg-destructive/10 border border-destructive/20 text-destructive px-4 py-3 rounded-lg text-sm">
                      {queueError}
                    </div>
                  )}
                </div>
              )}
            </TabsContent>
            )}

            <TabsContent value="maintenance">
              <div className="space-y-6">
                {/* System Commands Section */}
                <Card>
                  <CardHeader>
                    <CardTitle>System Commands</CardTitle>
                    <CardDescription>Device-level maintenance tasks</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-4">
                      {/* Auto-Update Status */}
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">Auto-Update Status:</span>
                        <Badge variant={updateServiceStatus.enabled ? 'default' : 'secondary'}>
                          {updateServiceStatus.enabled ? 'Enabled' : 'Disabled'}
                        </Badge>
                        <Badge variant={updateServiceStatus.running ? 'default' : 'secondary'}>
                          {updateServiceStatus.running ? 'Running' : 'Stopped'}
                        </Badge>
                      </div>

                      <div className="grid grid-cols-2 gap-4">
                        {systemTasks.map((task) => {
                          const isEnableDisabled = task.id === 'enable-updates' && updateServiceStatus.enabled && updateServiceStatus.running
                          const isDisableDisabled = task.id === 'disable-updates' && !updateServiceStatus.enabled && !updateServiceStatus.running
                          const shouldHighlight = (task.id === 'disable-updates' && updateServiceStatus.enabled) || (task.id === 'restart-xochitl' && !xochitlRunning)
                          const isRunning = runningSystemTask === task.id
                          return (
                            <Button
                              key={task.id}
                              onClick={() => handleSystemTask(task.id)}
                              disabled={commandRunning || isEnableDisabled || isDisableDisabled || connectionStatus !== 'connected' || (task.needsWriteableRoot && writeableRootBusy)}
                              variant={shouldHighlight ? 'default' : 'outline'}
                            >
                              {isRunning ? (
                                <>
                                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                  {task.label}
                                </>
                              ) : (
                                task.label
                              )}
                            </Button>
                          )
                        })}
                      </div>

                      {/* Timezone */}
                      <div className="grid grid-cols-2 gap-4 pt-2">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium whitespace-nowrap">Timezone:</span>
                          <TimezoneCombobox
                            value={selectedTimezone || timezoneMismatch?.deviceTimezone || ''}
                            onChange={handleTimezoneChange}
                            disabled={connectionStatus !== 'connected'}
                          />
                        </div>
                        <Button
                          onClick={handleSetTimezone}
                          disabled={settingTimezone || connectionStatus !== 'connected' || !selectedTimezone || selectedTimezone === deviceTimezone || writeableRootBusy}
                          variant={timezoneMismatch?.needsUpdate ? 'default' : 'outline'}
                        >
                          {settingTimezone ? (
                            <>
                              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                              Set Timezone
                            </>
                          ) : (
                            'Set Timezone'
                          )}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                {/* Package Maintenance Section */}
                {vellumInstalled && (
                  <Card>
                    <CardHeader>
                      <CardTitle>Package Maintenance</CardTitle>
                      <CardDescription>Package-specific commands</CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      {/* Vellum Commands */}
                      <div>
                        <h4 className="font-medium">Vellum</h4>
                        <p className="text-sm text-muted-foreground mb-2">Package manager for reMarkable tablets</p>
                        <div className="grid grid-cols-3 gap-2">
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                onClick={handleRunReenable}
                                disabled={commandRunning || runningReenable || connectionStatus !== 'connected' || reenableStatus === 'unneeded' || writeableRootBusy}
                                variant={reenableStatus === 'needed' ? 'default' : 'outline'}
                                size="sm"
                                className="flex-1"
                              >
                                Reenable
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Reenable packages that modify the system partition</TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                onClick={handleCheckUpgrades}
                                disabled={commandRunning || simulatingUpgrade || connectionStatus !== 'connected'}
                                variant={upgradesAvailable ? 'default' : 'outline'}
                                size="sm"
                                className="flex-1"
                              >
                                {simulatingUpgrade ? 'Checking...' : 'Upgrade'}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Check for and install package updates</TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                onClick={() => setShowCheckOSDialog(true)}
                                disabled={connectionStatus !== 'connected'}
                                variant="outline"
                                size="sm"
                                className="flex-1"
                              >
                                Check OS
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Check package compatibility with a target OS version</TooltipContent>
                          </Tooltip>
                        </div>
                      </div>

                      {/* Separator if there are package commands */}
                      {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]?.length > 0).length > 0 && (
                        <div className="border-t" />
                      )}

                      {/* Package-specific Commands */}
                      {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]?.length > 0).length > 0 && (
                        <div className="space-y-4">
                          {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]).sort((a, b) => a.name.localeCompare(b.name)).map((pkg) => (
                            <div key={pkg.name}>
                              <h4 className="font-medium">{pkg.name}</h4>
                              {pkg.description && <p className="text-sm text-muted-foreground mb-2">{pkg.description}</p>}
                              <div className="grid grid-cols-3 gap-2">
                                {maintenanceCommands[pkg.name]?.map((cmd) => {
                                  const isRunning = currentRunningCommand?.componentId === pkg.name &&
                                                   currentRunningCommand?.commandId === cmd.id
                                  const isHashtabRebuild = pkg.name === 'qt-resource-rebuilder' && cmd.id === 'rebuild_hashtable'
                                  const shouldHighlight = isHashtabRebuild && hashtabMismatch

                                  return (
                                    <div key={cmd.id} className="flex gap-2">
                                      <Tooltip>
                                        <TooltipTrigger asChild>
                                          <Button
                                            onClick={() => handleComponentMaintenance(pkg.name, cmd.id)}
                                            disabled={(commandRunning && !isRunning) || connectionStatus !== 'connected' || writeableRootBusy}
                                            variant={shouldHighlight ? 'default' : 'outline'}
                                            size="sm"
                                            className="flex-1"
                                          >
                                            {cmd.label}
                                          </Button>
                                        </TooltipTrigger>
                                        {cmd.description && (
                                          <TooltipContent>{cmd.description}</TooltipContent>
                                        )}
                                      </Tooltip>
                                    </div>
                                  )
                                })}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                )}
              </div>
            </TabsContent>

            {tabVisibility.utilities && (
              <TabsContent value="utilities" forceMount className={activeTab === 'utilities' ? '' : 'hidden'}>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <Card className={isTerminalRunning ? 'md:col-span-2' : ''}>
                    <CardHeader>
                      <CardTitle>Terminal</CardTitle>
                      <CardDescription>Interactive SSH shell</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <InteractiveTerminal
                        isConnected={connectionStatus === 'connected'}
                        visible={activeTab === 'utilities'}
                        onRunningChange={setIsTerminalRunning}
                        theme={resolvedTerminalTheme}
                      />
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>File Browser</CardTitle>
                      <CardDescription>Browse and transfer files</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        className="w-full"
                        variant="outline"
                        onClick={() => setShowFileBrowser(true)}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Open File Browser
                      </Button>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Configuration Editor</CardTitle>
                      <CardDescription>Edit settings</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        className="w-full"
                        variant="outline"
                        onClick={() => setShowConfigEditor(true)}
                        disabled={connectionStatus !== 'connected'}
                      >
                        xochitl.conf
                      </Button>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Backup & Restore</CardTitle>
                      <CardDescription>Manage device backups</CardDescription>
                    </CardHeader>
                    <CardContent className="flex gap-2">
                      <Button
                        className="flex-1"
                        variant="outline"
                        onClick={() => setBackupDialogMode('backup')}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Backup
                      </Button>
                      <Button
                        className="flex-1"
                        variant="outline"
                        onClick={() => setBackupDialogMode('restore')}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Restore
                      </Button>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Document Import</CardTitle>
                      <CardDescription>Upload documents to your reMarkable library</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        className="w-full"
                        variant="outline"
                        onClick={() => setShowImportPDF(true)}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Import
                      </Button>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>OS Manager</CardTitle>
                      <CardDescription>View, install, and switch OS versions.</CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        className="w-full"
                        variant="outline"
                        onClick={() => setShowSoftwareManager(true)}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Manage
                      </Button>
                    </CardContent>
                  </Card>
                </div>
              </TabsContent>
            )}
          </Tabs>

          {/* Queue Section - Fixed at bottom, visible on all tabs */}
          {(installQueue.size > 0 || uninstallQueue.size > 0) && (
            <div className="fixed bottom-0 left-0 right-0 py-4 px-6 bg-background border-t shadow-[0_-4px_6px_-1px_rgba(0,0,0,0.1)] z-50 space-y-4">
              {/* Install Queue */}
              {installQueue.size > 0 && (
                <div>
                  <Accordion type="single" collapsible>
                    <AccordionItem value="install-queue" className="border-none">
                      <AccordionTrigger className="py-2 text-sm font-medium hover:no-underline">
                        Install Queue ({installQueue.size})
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className="space-y-1 mb-3">
                          {Array.from(installQueue).map((name) => {
                            return (
                              <div
                                key={name}
                                className="flex items-center justify-between text-sm py-1"
                              >
                                <span>{name}</span>
                                <button
                                  onClick={() => removeFromQueue(name)}
                                  className="text-muted-foreground hover:text-foreground"
                                >
                                  <X className="h-4 w-4" />
                                </button>
                              </div>
                            )
                          })}
                        </div>
                      </AccordionContent>
                    </AccordionItem>
                  </Accordion>
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
                          const sim = await window.go.main.App.SimulateInstall([...installQueue], device)
                          const filteredPackages = sim.packages.filter((name: string) => !installedPackages.has(name))
                          if (filteredPackages.length === 0) {
                            setQueueError('All packages are already installed')
                            setTimeout(() => setQueueError(null), 4000)
                            setInstallQueue(new Set())
                          } else {
                            setPendingInstallConfirm({ packages: filteredPackages, requested: sim.requested })
                          }
                        } catch (err) {
                          console.error('SimulateInstall failed:', err)
                          const pkgs = [...installQueue].filter((name) => !installedPackages.has(name))
                          if (pkgs.length === 0) {
                            setQueueError('All packages are already installed')
                            setTimeout(() => setQueueError(null), 4000)
                            setInstallQueue(new Set())
                          } else {
                            setPendingInstallConfirm({ packages: pkgs, requested: pkgs })
                          }
                        } finally {
                          setSimulatingInstall(false)
                        }
                      }}
                      disabled={installing || uninstalling || simulatingInstall || connectionStatus !== 'connected'}
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
                      <AccordionTrigger className="py-2 text-sm font-medium text-destructive hover:no-underline">
                        Uninstall Queue ({uninstallQueue.size})
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className="space-y-1 mb-3">
                          {Array.from(uninstallQueue).map((name) => {
                            return (
                              <div
                                key={name}
                                className="flex items-center justify-between text-sm py-1"
                              >
                                <span>{name}</span>
                                <button
                                  onClick={() => removeFromUninstallQueue(name)}
                                  className="text-muted-foreground hover:text-foreground"
                                >
                                  <X className="h-4 w-4" />
                                </button>
                              </div>
                            )
                          })}
                        </div>
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
          </>
        )}
      </div>

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

      {/* Package Upgrade Confirmation Dialog */}
      <Dialog open={pendingPackageUpgrade !== null} onOpenChange={(open) => !open && setPendingPackageUpgrade(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Upgrade Packages</DialogTitle>
            <DialogDescription>
              The following {pendingPackageUpgrade?.length} package{pendingPackageUpgrade?.length !== 1 ? 's' : ''} will be upgraded:
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[40vh] overflow-y-auto">
            <ul className="list-disc list-inside space-y-1 text-sm">
              {pendingPackageUpgrade?.sort().map((pkg) => (
                <li key={pkg}>{pkg}</li>
              ))}
            </ul>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingPackageUpgrade(null)}>
              Cancel
            </Button>
            <Button onClick={confirmPackageUpgrade}>
              Upgrade ({pendingPackageUpgrade?.length})
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* No Updates Available Dialog */}
      <Dialog open={showNoUpgradesDialog} onOpenChange={setShowNoUpgradesDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>No Updates Available</DialogTitle>
            <DialogDescription>
              All packages are up to date.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setShowNoUpgradesDialog(false)}>OK</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ImportPDFDialog
        open={showImportPDF}
        isConnected={connectionStatus === 'connected'}
        onOpenChange={setShowImportPDF}
      />

      <SoftwareManagerDialog
        open={showSoftwareManager}
        onOpenChange={setShowSoftwareManager}
        isConnected={connectionStatus === 'connected'}
        vellumInstalled={vellumInstalled}
        onSelectPackageForOS={handleSelectPackageForOS}
      />

      {/* Backup & Restore Dialog */}
      <BackupRestoreDialog
        mode={backupDialogMode}
        onClose={() => setBackupDialogMode(null)}
      />

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

      {/* Progress Modal */}
      <Dialog
        open={showProgressModal || pendingInstallConfirm !== null || pendingUninstallConfirm !== null}
        onOpenChange={(open) => {
          if (!installing && !uninstalling && !commandRunning) {
            if (!open) {
              setShowProgressModal(false)
              setPendingInstallConfirm(null)
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
                window.go.main.App.GetXochitlStatus().then(setXochitlRunning).catch(() => {})
              }
            }
          }
        }}
        closable={false}
      >
        <DialogContent className="w-[90vw] max-w-none">
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
                    handleInstallQueue(pkgs)
                  }}>
                    Install ({pendingInstallConfirm.packages.length})
                  </Button>
                </DialogFooter>
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
          window.go.main.App.RespondToDialog(false)
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {dialogRequest?.infoOnly
                ? <Info className="h-5 w-5 text-blue-500" />
                : <AlertTriangle className="h-5 w-5 text-yellow-500" />
              }
              {dialogRequest?.title || 'Confirmation Required'}
            </DialogTitle>
            <DialogDescription>
              <div className="space-y-4 pt-4">
                <p>{dialogRequest?.message}</p>
                {dialogRequest?.steps && dialogRequest.steps.length > 0 && (
                  <ol className="list-decimal list-outside space-y-1 text-sm pl-8">
                    {dialogRequest.steps.map((step, idx) => (
                      <li key={idx}>{step}</li>
                    ))}
                  </ol>
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
                  window.go.main.App.RespondToDialog(true)
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
                    window.go.main.App.RespondToDialog(false)
                  }}
                >
                  Cancel
                </Button>
                <Button
                  onClick={() => {
                    dialogRespondedRef.current = true
                    setShowRebuildDialog(false)
                    setDialogRequest(null)
                    window.go.main.App.RespondToDialog(true)
                  }}
                >
                  {dialogRequest?.confirmText || 'Proceed'}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Remove Confirmation Dialog */}
      <Dialog open={deviceToDelete !== null} onOpenChange={(open) => !open && setDeviceToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove "{savedDevices.find(d => d.id === deviceToDelete)?.name}"?</DialogTitle>
            <DialogDescription>
              This will remove the saved connection and credentials.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeviceToDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleConfirmDelete}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Save Device Dialog */}
      <Dialog open={showSaveDeviceDialog} onOpenChange={(open) => {
        setShowSaveDeviceDialog(open)
        if (!open) setSaveDeviceError('')
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Device</DialogTitle>
            <DialogDescription>
              Save this device for quick reconnection in the future. Your credentials will be stored securely.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="deviceName">Device Name</Label>
              <Input
                id="deviceName"
                value={deviceName}
                onChange={(e) => setDeviceName(e.target.value)}
                placeholder={getDisplayName(deviceInfo.machine || '') || 'My reMarkable'}
              />
            </div>
            {saveDeviceError && (
              <p className="text-sm text-destructive">{saveDeviceError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowSaveDeviceDialog(false)}
            >
              {saveDeviceError ? 'Close' : 'Skip'}
            </Button>
            {!saveDeviceError && (
              <Button onClick={handleSaveDevice} disabled={!deviceName.trim()}>
                Save Device
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>


      {/* Package Detail Side Panel */}
      <Sheet open={selectedPackage !== null} onOpenChange={(open) => { if (!open) { setSelectedPackage(null); setSidebarViewOnly(false); setSidebarIncompatible(false) } }}>
        <SheetContent side="right" className="w-[400px] sm:w-[450px] sm:max-w-none flex flex-col">
          {selectedPackage && (
            <PackageDetailPanel
              pkg={selectedPackage}
              isInstalled={installedPackages.has(selectedPackage.name)}
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
            />
          )}
        </SheetContent>
      </Sheet>

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
        sshAgentSocketPath={sshAgentSocketPath}
        onSaveSettings={handleSaveSettings}
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

      <VellumInstallSuccessDialog
        open={bootstrapSuccess}
        onOpenChange={(open) => {
          if (!open) {
            setBootstrapSuccess(false)
            setBootstrapOutput('')
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

      <CheckOSDialog
        open={showCheckOSDialog}
        onOpenChange={setShowCheckOSDialog}
        isConnected={connectionStatus === 'connected'}
        onSelectPackage={handleSelectPackageForOS}
      />

      {showFileBrowser && (
        <div className="fixed inset-0 z-50 bg-background overflow-auto">
          <div className="min-h-screen p-6">
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h1 className="text-3xl font-bold text-foreground">reManager</h1>
                  <p className="text-muted-foreground">Manage packages on your reMarkable</p>
                </div>
                <div className="flex items-center gap-2 text-sm">
                  {device && (
                    <span className="text-muted-foreground">
                      {getDisplayName(deviceInfo.machine || device)} ({deviceInfo.firmware || 'unknown firmware'})
                    </span>
                  )}
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="sm" onClick={() => setShowSettingsDialog(true)}>
                        <Settings className="h-4 w-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>Settings</TooltipContent>
                  </Tooltip>
                  {device && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="ghost" size="sm" onClick={handleDisconnect}>
                          <Unplug className="h-4 w-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Disconnect</TooltipContent>
                    </Tooltip>
                  )}
                </div>
              </div>
              <hr className="border-border" />
              <div className="flex items-center justify-between">
                <h2 className="text-xl font-semibold">File Browser</h2>
                <Button variant="ghost" size="sm" onClick={() => setShowFileBrowser(false)}>
                  <X className="h-4 w-4 mr-2" />
                  Close
                </Button>
              </div>
              <FileBrowser
                isConnected={connectionStatus === 'connected'}
                suppressSystemFileWarnings={suppressSystemFileWarnings}
                isVisible={showFileBrowser}
              />
            </div>
          </div>
        </div>
      )}

      {showConfigEditor && (
        <div className="fixed inset-0 z-50 bg-background overflow-auto">
          <div className="min-h-screen p-6">
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h1 className="text-3xl font-bold text-foreground">reManager</h1>
                  <p className="text-muted-foreground">Manage packages on your reMarkable</p>
                </div>
                <div className="flex items-center gap-2 text-sm">
                  {device && (
                    <span className="text-muted-foreground">
                      {getDisplayName(deviceInfo.machine || device)} ({deviceInfo.firmware || 'unknown firmware'})
                    </span>
                  )}
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="sm" onClick={() => setShowSettingsDialog(true)}>
                        <Settings className="h-4 w-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>Settings</TooltipContent>
                  </Tooltip>
                  {device && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="ghost" size="sm" onClick={handleDisconnect}>
                          <Unplug className="h-4 w-4" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Disconnect</TooltipContent>
                    </Tooltip>
                  )}
                </div>
              </div>

              <hr className="border-border" />

              <div className="flex items-center justify-between">
                <h2 className="text-xl font-semibold">xochitl.conf Editor</h2>
                <Button variant="ghost" size="sm" onClick={() => setShowConfigEditor(false)}>
                  <X className="h-4 w-4 mr-2" />
                  Close
                </Button>
              </div>

              <ConfigEditor isConnected={connectionStatus === 'connected'} theme={resolvedEditorTheme} />
            </div>
          </div>
        </div>
      )}

      <Toaster position="bottom-right" richColors />
    </div>
  )
}
