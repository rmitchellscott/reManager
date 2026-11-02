import { Archive, ArrowRight, ArrowsCounterClockwise, CircleNotch, FileCode, FilePlus, FolderOpen, Info, Package, TerminalWindow, Warning } from '@phosphor-icons/react'
import type { Icon as PhosphorIcon } from '@phosphor-icons/react'
import { Fragment, lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { TabsContent } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Switch } from '@/components/ui/switch'
import { CheckOSDialog } from '@/components/CheckOSDialog'
import { TimezoneCombobox } from '@/components/TimezoneCombobox'
import { InteractiveTerminal } from '@/components/InteractiveTerminal'
import { FileBrowser } from '@/components/files/FileBrowser'
import { BackupRestoreDialog } from '@/components/BackupRestore'
import { ImportPDFDialog } from '@/components/ImportPDFDialog'
import { SoftwareManagerDialog } from '@/components/SoftwareManagerDialog'
import { TakeoverPage } from '@/components/TakeoverPage'
import { cn } from '@/lib/utils'
import { useAppContext } from '@/contexts/AppContext'
import { panelAction, panelControl, PanelGroup, PanelRow, SectionLabel, StepMarker, SEVERITY_TEXT } from '@/components/Panel'
import { OSUpdatePackagesRow } from '@/components/OSUpdatePackagesRow'
import type { OsSyncStatus, SelectPackageForOSFn } from '@/tabs/ModsTab'
import { MaintenanceCommandInfo, HashtabVersionStatus, OSChannelStatus, ReenableStatus, TimezoneStatus, UpdateServiceStatus, UserGuideStatus, PackageCompatibilityStatus } from '@/lib/types'
import { promotedIds, type StatusRowId, type SystemStatus } from '@/lib/systemStatus'
import { ConfigService } from '@bindings/reManager'

const ConfigEditor = lazy(() => import('@/components/ConfigEditor').then((module) => ({ default: module.ConfigEditor })))

const toolButton = `${panelControl} whitespace-normal text-left`

const REENABLE_STATE: Partial<Record<ReenableStatus, string>> = {
  [ReenableStatus.ReenableNeeded]: 'Cleared by an OS update',
  [ReenableStatus.ReenableOK]: 'Enabled',
  [ReenableStatus.ReenableUnneeded]: 'None installed',
}

// An OS update wipes the read-only system partition, so these rows become an ordered recovery rather than independent conditions: each undoes a different piece, and later steps report failure until the earlier ones have run.
const UPDATE_STEP_ORDER: StatusRowId[] = ['autoUpdates', 'packages', 'reenable', 'hashtable', 'mods', 'launcher', 'timezone']

type ToolId = 'terminal' | 'files' | 'config' | 'backup' | 'import'

interface ToolDefinition {
  id: ToolId
  label: string
  description: string
  action: string
  Icon: PhosphorIcon
}

const TOOLS: ToolDefinition[] = [
  { id: 'terminal', label: 'Terminal', description: 'Interactive SSH shell', action: 'Open', Icon: TerminalWindow },
  { id: 'files', label: 'File Browser', description: 'Browse and transfer files', action: 'Open', Icon: FolderOpen },
  { id: 'config', label: 'Configuration', description: 'Edit xochitl.conf', action: 'Open', Icon: FileCode },
  { id: 'backup', label: 'Backup & Restore', description: 'Manage reMarkable backups', action: 'Open', Icon: Archive },
  { id: 'import', label: 'Document Import', description: 'Upload to your library', action: 'Import', Icon: FilePlus },
]

interface SystemTabProps {
  maintenanceCommands: Record<string, MaintenanceCommandInfo[]>
  updateServiceStatus: UpdateServiceStatus
  shellRunning: boolean
  setShellRunning: (v: boolean) => void
  xochitlRunning: boolean
  currentLauncher: string
  xoviActive: boolean
  hashtab: HashtabVersionStatus | null
  systemStatus: SystemStatus
  osChannel: OSChannelStatus | null
  osMismatchDetected: boolean
  compatibilityStatus: PackageCompatibilityStatus | null
  active: boolean
  osSyncStatus: OsSyncStatus | null
  onReviewPackages: () => void
  onApplyOsSync: () => void
  reenableStatus: ReenableStatus
  timezoneMismatch: TimezoneStatus | null
  selectedTimezone: string
  deviceTimezone: string
  runningSystemTask: string | null
  currentRunningCommand: { componentId: string; commandId: string } | null
  settingTimezone: boolean
  handleSystemTask: (taskId: string) => Promise<void>
  handleComponentMaintenance: (componentId: string, commandId: string) => Promise<void>
  handleRunReenable: () => Promise<void>
  handleTimezoneChange: (timezone: string) => Promise<void>
  handleSetTimezone: () => void
  handleSelectPackageForOS: SelectPackageForOSFn
  runningReenable: boolean
  guideStatus: UserGuideStatus | null
  installingGuide: boolean
  handleInstallGuide: () => Promise<void>
  onRepairVellum: () => void
  onUninstallVellum: (removeAllPackages: boolean) => void
  onSettings: () => void
  onDisconnect: () => Promise<void>
  showSoftwareManager: boolean
  onShowSoftwareManagerChange: (open: boolean) => void
}

export function SystemTab({
  maintenanceCommands,
  updateServiceStatus,
  shellRunning,
  setShellRunning,
  xochitlRunning,
  currentLauncher,
  xoviActive,
  hashtab,
  systemStatus,
  osChannel,
  osMismatchDetected,
  compatibilityStatus,
  active,
  osSyncStatus,
  onReviewPackages,
  onApplyOsSync,
  reenableStatus,
  timezoneMismatch,
  selectedTimezone,
  deviceTimezone,
  runningSystemTask,
  currentRunningCommand,
  settingTimezone,
  handleSystemTask,
  handleComponentMaintenance,
  handleRunReenable,
  handleTimezoneChange,
  handleSetTimezone,
  handleSelectPackageForOS,
  runningReenable,
  guideStatus,
  installingGuide,
  handleInstallGuide,
  onRepairVellum,
  onUninstallVellum,
  onSettings,
  onDisconnect,
  showSoftwareManager,
  onShowSoftwareManagerChange,
}: SystemTabProps) {
  const {
    installedPackages,
    packages,
    commandRunning,
    runningOperation,
    startOperation,
    connectionStatus,
    writableRootBusy,
    vellumInstalled,
    device,
    deviceInfo,
    resolvedTerminalTheme,
    resolvedEditorTheme,
    suppressSystemFileWarnings,
  } = useAppContext()

  const maintenancePackages = useMemo<{ name: string; description?: string }[]>(() => {
    return Array.from(installedPackages.keys())
      .filter(name => maintenanceCommands[name]?.length > 0)
      .map(name => ({ name, description: packages.find(p => p.name === name)?.description }))
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [installedPackages, packages, maintenanceCommands])

  const [showCheckOSDialog, setShowCheckOSDialog] = useState(false)
  const [showRepairConfirm, setShowRepairConfirm] = useState(false)
  const [showGuideUpdateConfirm, setShowGuideUpdateConfirm] = useState(false)
  const [showUninstallConfirm, setShowUninstallConfirm] = useState(false)
  const [removePackages, setRemovePackages] = useState(true)
  const [showTerminal, setShowTerminal] = useState(false)
  const [showFileBrowser, setShowFileBrowser] = useState(false)
  const [fileBrowserPath, setFileBrowserPath] = useState<string | undefined>(undefined)
  const [pickingSleepScreen, setPickingSleepScreen] = useState(false)
  const [sleepScreenSupported, setSleepScreenSupported] = useState(false)
  const [currentSleepScreen, setCurrentSleepScreen] = useState<string | null>(null)
  const [sleepScreenRestartMessage, setSleepScreenRestartMessage] = useState<string | null>(null)
  const [showConfigEditor, setShowConfigEditor] = useState(false)
  const [configEditorDirty, setConfigEditorDirty] = useState(false)
  const [confirmDiscardConfig, setConfirmDiscardConfig] = useState(false)
  const [showImportPDF, setShowImportPDF] = useState(false)
  const [showBackupChooser, setShowBackupChooser] = useState(false)
  const [backupDialogMode, setBackupDialogMode] = useState<'backup' | 'restore' | null>(null)
  // BackupRestoreDialog prompts for a file again whenever onClose changes identity.
  const closeBackupDialog = useCallback(() => setBackupDialogMode(null), [])

  useEffect(() => {
    if (connectionStatus !== 'connected') {
      setSleepScreenSupported(false)
      setCurrentSleepScreen(null)
      return
    }
    ConfigService.IsSleepScreenSupported().then(setSleepScreenSupported).catch(() => {})
    ConfigService.GetSleepScreen().then(path => setCurrentSleepScreen(path || null)).catch(() => setCurrentSleepScreen(null))
  }, [connectionStatus])

  // Held as state, not a ref: the tab panel unmounts while hidden, so the observer must re-attach to the element each time it returns.
  const [guideDetail, setGuideDetail] = useState<HTMLSpanElement | null>(null)
  const [guideHighlightSpent, setGuideHighlightSpent] = useState(false)
  const guideHighlighted = !!guideStatus?.unseen && !guideHighlightSpent

  useEffect(() => {
    if (!guideStatus) setGuideHighlightSpent(false)
  }, [guideStatus])

  useEffect(() => {
    if (!guideDetail || !guideHighlighted) return
    let fadeTimer: number | undefined
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting && fadeTimer === undefined) {
        fadeTimer = window.setTimeout(() => setGuideHighlightSpent(true), 2000)
      }
    })
    observer.observe(guideDetail)
    return () => {
      observer.disconnect()
      window.clearTimeout(fadeTimer)
    }
  }, [guideDetail, guideHighlighted])

  const openFileBrowser = (path?: string) => {
    setFileBrowserPath(path)
    setPickingSleepScreen(false)
    setShowFileBrowser(true)
  }

  const closeFileBrowser = () => {
    setShowFileBrowser(false)
    setPickingSleepScreen(false)
    setFileBrowserPath(undefined)
  }

  const closeConfigEditor = () => {
    setConfirmDiscardConfig(false)
    setConfigEditorDirty(false)
    setShowConfigEditor(false)
  }

  const requestCloseConfigEditor = () => {
    if (configEditorDirty) setConfirmDiscardConfig(true)
    else closeConfigEditor()
  }

  const sleepScreenBusy = runningOperation?.kind === 'sleep-screen'

  const handleResetSleepScreen = async () => {
    const result = await startOperation({
      kind: 'sleep-screen',
      title: 'Reset the sleep screen',
      background: true,
      run: id => ConfigService.ResetSleepScreen(id),
    })
    if (!result.ok) return
    setCurrentSleepScreen(null)
    setSleepScreenRestartMessage('Sleep screen reset to the default. It takes effect after the UI restarts.')
  }

  const setSleepScreen = async (path: string) => {
    const result = await startOperation({
      kind: 'sleep-screen',
      title: 'Set the sleep screen',
      background: true,
      run: id => ConfigService.SetSleepScreen(id, path),
    })
    if (!result.ok) return
    setCurrentSleepScreen(path)
    setSleepScreenRestartMessage(`Sleep screen set to "${path.split('/').pop()}". It takes effect after the UI restarts.`)
  }

  const restartUiForSleepScreen = () => {
    setSleepScreenRestartMessage(null)
    void startOperation({
      kind: 'restart-ui',
      title: 'Restart the reMarkable UI',
      background: true,
      run: id => ConfigService.RestartXochitl(id),
    })
  }

  const openTool = (id: ToolId) => {
    if (id === 'terminal') setShowTerminal(true)
    if (id === 'files') openFileBrowser()
    if (id === 'config') setShowConfigEditor(true)
    if (id === 'backup') setShowBackupChooser(true)
    if (id === 'import') setShowImportPDF(true)
  }

  const {
    rows,
    xoviStart,
    xoviStop,
    hashtableRebuild,
    launcherAction,
    launcherActive,
    launcherConfigured,
    autoUpdateTasks,
    restartUiTask,
    autoUpdatesActive,
    hashtableBroken,
    uiNeedsRestart,
    launcherStalled,
    xoviNeedsStart,
    osUpdateAvailable,
    osUpdateBlocked,
  } = systemStatus
  const autoUpdatesBusy = runningSystemTask === 'enable-updates' || runningSystemTask === 'disable-updates'
  const autoUpdatesLocked =
    commandRunning || autoUpdatesBusy || connectionStatus !== 'connected' ||
    autoUpdateTasks.some(t => t.needsWritableRoot && writableRootBusy)
  const autoUpdateSummary = runningSystemTask === 'enable-updates' ? 'Enabling…'
    : runningSystemTask === 'disable-updates' ? 'Disabling…'
    : updateServiceStatus.enabled === updateServiceStatus.running
    ? updateServiceStatus.enabled ? 'On, your reMarkable can update itself' : 'Off'
    : updateServiceStatus.enabled ? 'Enabled, service stopped' : 'Disabled, service still running'

  const isCommandRunning = (componentId: string, commandId: string) =>
    currentRunningCommand?.componentId === componentId && currentRunningCommand?.commandId === commandId

  const advancedPackages = maintenancePackages
    .map(pkg => ({
      pkg,
      commands: (maintenanceCommands[pkg.name] ?? []).filter(c => !promotedIds(pkg.name).includes(c.id)),
    }))
    .filter(entry => entry.commands.length > 0)

  // Vellum refuses package operations until its recorded OS matches the running one, so the packages step is done exactly when that mismatch clears.
  const heldStepsRef = useRef(new Set<StatusRowId>())
  if (!active) heldStepsRef.current.clear()
  const updateSteps = UPDATE_STEP_ORDER
    .filter(id => rows[id].shown || heldStepsRef.current.has(id))
    .map(id => ({ id, done: rows[id].severity === null }))

  const [stepsOsVersion, setStepsOsVersion] = useState<string | null>(null)
  const stepsOsSeenRunningRef = useRef(false)
  const showSteps = stepsOsVersion !== null
  const stepsSettled = updateSteps.every(step => step.done)
  const mismatchOsVersion = compatibilityStatus?.currentOsVersion ?? ''
  const runningFirmware = deviceInfo.firmware ?? ''

  useEffect(() => {
    if (!osMismatchDetected) return
    stepsOsSeenRunningRef.current = false
    setStepsOsVersion(mismatchOsVersion)
  }, [osMismatchDetected, mismatchOsVersion])

  useEffect(() => {
    if (showSteps && stepsSettled && !active) setStepsOsVersion(null)
  }, [showSteps, stepsSettled, active])
  if (showSteps && active) updateSteps.forEach(step => heldStepsRef.current.add(step.id))

  const introVersionsRef = useRef<{ from: string; to: string } | null>(null)
  if (osMismatchDetected && compatibilityStatus) {
    introVersionsRef.current = { from: compatibilityStatus.storedOsVersion, to: compatibilityStatus.currentOsVersion }
  }
  if (!active && !osMismatchDetected) introVersionsRef.current = null
  const introVersions = showSteps ? introVersionsRef.current : null
  const introRef = useRef<HTMLParagraphElement>(null)
  const introHeightRef = useRef(0)
  useLayoutEffect(() => {
    const intro = introRef.current
    if (!intro) { introHeightRef.current = 0; return }
    intro.style.minHeight = `${introHeightRef.current}px`
    introHeightRef.current = Math.max(introHeightRef.current, intro.getBoundingClientRect().height)
  })

  // Device info can still carry the previous connect's firmware when the mismatch lands, so the sequence is dropped only for a firmware seen after the one it was raised on.
  useEffect(() => {
    if (!stepsOsVersion || !runningFirmware) return
    if (runningFirmware === stepsOsVersion) {
      stepsOsSeenRunningRef.current = true
    } else if (stepsOsSeenRunningRef.current) {
      setStepsOsVersion(null)
    }
  }, [stepsOsVersion, runningFirmware])

  const nextStepId = showSteps ? updateSteps.find(step => !step.done)?.id : undefined
  const primaryFor = (id: string, outstanding: boolean) =>
    (showSteps ? id === nextStepId : outstanding) ? 'default' : 'outline'

  const markerFor = (id: string) => {
    if (!showSteps) return undefined
    const position = updateSteps.findIndex(step => step.id === id)
    if (position < 0) return undefined
    return <StepMarker position={position + 1} done={updateSteps[position].done} />
  }

  const packagesBusy = commandRunning || connectionStatus !== 'connected' || writableRootBusy

  const statusRows: { id: string; node: ReactNode }[] = [
    { id: 'autoUpdates', node: autoUpdateTasks.length > 0 ? (
        <PanelRow
          label="Automatic OS updates"
          detail={autoUpdateSummary}
          severity={autoUpdatesBusy ? null : rows.autoUpdates.severity}
          marker={markerFor('autoUpdates')}
        >
          <Switch
            checked={autoUpdatesActive}
            onCheckedChange={(next) => handleSystemTask(next ? 'enable-updates' : 'disable-updates')}
            disabled={autoUpdatesLocked}
            aria-label="Automatic OS updates"
          />
        </PanelRow>
    ) : null },
    { id: 'os', node: (
        <PanelRow
          label="reMarkable OS"
          detail={osChannel && osUpdateAvailable && !showSteps
            ? (
              <span className="inline-flex items-center gap-1">
                {deviceInfo.firmware || osChannel.activeVersion}
                <ArrowRight className="h-3 w-3" />
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="inline-flex items-center gap-1 font-semibold text-[var(--info-text)]">
                      {osUpdateBlocked && <Info className="h-3.5 w-3.5 text-muted-foreground" weight="fill" />}
                      {osChannel.latestPublic}
                      <span className="sr-only">
                        {osUpdateBlocked ? ', compatibility issues found' : ', installed mods are compatible'}
                      </span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>
                    {osUpdateBlocked ? 'Compatibility issues found' : 'Installed mods are compatible'}
                  </TooltipContent>
                </Tooltip>
              </span>
            )
            : deviceInfo.firmware || 'Unknown'}
        >
          <Button
            className={panelAction}
            variant="outline"
            onClick={() => onShowSoftwareManagerChange(true)}
            disabled={connectionStatus !== 'connected'}
          >
            {osUpdateAvailable && !showSteps ? 'Review' : 'Manage'}
          </Button>
        </PanelRow>
    ) },
    { id: 'packages', node: showSteps && (rows.packages.shown || heldStepsRef.current.has('packages')) ? (
      compatibilityStatus || !osMismatchDetected ? (
        <OSUpdatePackagesRow
          marker={markerFor('packages')}
          compatiblePackages={compatibilityStatus?.compatiblePackages || []}
          incompatiblePackages={compatibilityStatus?.incompatiblePackages || []}
          fetchFailed={!!compatibilityStatus?.fetchFailed}
          disabled={packagesBusy}
          targetOs={introVersions?.to ?? deviceInfo.firmware ?? ''}
          sync={osSyncStatus}
          settled={!osMismatchDetected && osSyncStatus?.state !== 'running'}
          onReview={onReviewPackages}
          onApply={onApplyOsSync}
          isNext={nextStepId === 'packages'}
        />
      ) : (
        <PanelRow
          label="Compatibility"
          detail="Checking…"
          marker={markerFor('packages')}
        />
      )
    ) : null },
    { id: 'ui', node: rows.ui.shown && restartUiTask ? (
        <PanelRow
          label="reMarkable UI"
          detail={xochitlRunning ? 'Running' : 'Not running'}
          severity={rows.ui.severity}
        >
          <Button
            className={panelAction}
            variant={uiNeedsRestart ? 'default' : 'outline'}
            onClick={() => handleSystemTask(restartUiTask.id)}
            disabled={commandRunning || launcherConfigured || connectionStatus !== 'connected' || (restartUiTask.needsWritableRoot && writableRootBusy)}
          >
            {runningSystemTask === restartUiTask.id && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            Restart
          </Button>
        </PanelRow>
    ) : null },
    { id: 'sleep-screen', node: sleepScreenSupported && (
      <PanelRow
        label="Sleep screen"
        detail={currentSleepScreen ? (
          <button
            className="cursor-pointer underline-offset-2 hover:underline"
            onClick={() => openFileBrowser(currentSleepScreen.substring(0, currentSleepScreen.lastIndexOf('/')) || '/')}
          >
            {currentSleepScreen.split('/').pop()}
          </button>
        ) : 'Default'}
      >
        {currentSleepScreen ? (
          <Button
            className={panelAction}
            variant="outline"
            onClick={handleResetSleepScreen}
            disabled={commandRunning || connectionStatus !== 'connected'}
          >
            {sleepScreenBusy && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            Reset
          </Button>
        ) : (
          <Button
            className={panelAction}
            variant="outline"
            onClick={() => {
              setFileBrowserPath(undefined)
              setPickingSleepScreen(true)
              setShowFileBrowser(true)
            }}
            disabled={commandRunning || connectionStatus !== 'connected'}
          >
            {sleepScreenBusy && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            Choose
          </Button>
        )}
      </PanelRow>
    ) },
    { id: 'guide', node: rows.guide.shown && guideStatus && (
      <PanelRow
        label="reManager guide"
        detail={
          <span ref={setGuideDetail} className={cn('transition-colors duration-1000', guideHighlighted && SEVERITY_TEXT.info)}>
            {!guideStatus.installed ? 'Not on your reMarkable' : guideStatus.needsUpdate ? 'Out of date' : 'Current'}
          </span>
        }
        severity={rows.guide.severity}
      >
        <Button
          className={panelAction}
          variant="outline"
          onClick={guideStatus.installed ? () => setShowGuideUpdateConfirm(true) : handleInstallGuide}
          disabled={commandRunning || connectionStatus !== 'connected' || (guideStatus.installed && !guideStatus.needsUpdate)}
        >
          {installingGuide && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
          {guideStatus.installed ? 'Update' : 'Install'}
        </Button>
      </PanelRow>
    ) },
    { id: 'launcher', node: launcherAction ? (
        <PanelRow
          label="Launcher"
          marker={markerFor('launcher')}
          detail={!launcherConfigured
            ? 'Stock UI'
            : launcherActive
              ? `${currentLauncher} running`
              : `${currentLauncher} not running`}
          severity={rows.launcher.severity}
        >
          <Button
            className={panelAction}
            variant={primaryFor('launcher', launcherStalled)}
            onClick={() => handleComponentMaintenance('launcherctl', launcherAction.id)}
            disabled={!launcherConfigured || (commandRunning && !isCommandRunning('launcherctl', launcherAction.id)) || connectionStatus !== 'connected' || writableRootBusy}
          >
            {isCommandRunning('launcherctl', launcherAction.id) && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            {launcherActive ? 'Stop' : 'Start'}
          </Button>
        </PanelRow>
    ) : null },
    { id: 'mods', node: xoviStart ? (
        <PanelRow
          label="Mods"
          marker={markerFor('mods')}
          detail={hashtableBroken ? 'Blocked by the hashtable' : xoviActive ? 'Running' : 'Not running'}
          severity={rows.mods.severity}
        >
          <div className="flex gap-2">
            {xoviActive && xoviStop && (
              <Button
                className={panelAction}
                variant="outline"
                onClick={() => handleComponentMaintenance('xovi-extensions', xoviStop.id)}
                disabled={launcherConfigured || (commandRunning && !isCommandRunning('xovi-extensions', xoviStop.id)) || connectionStatus !== 'connected' || writableRootBusy}
              >
                {isCommandRunning('xovi-extensions', xoviStop.id) && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
                Stop
              </Button>
            )}
            <Button
              className={panelAction}
              variant={primaryFor('mods', xoviNeedsStart)}
              onClick={() => handleComponentMaintenance('xovi-extensions', xoviStart.id)}
              disabled={hashtableBroken || launcherConfigured || (commandRunning && !isCommandRunning('xovi-extensions', xoviStart.id)) || connectionStatus !== 'connected' || writableRootBusy}
            >
              {isCommandRunning('xovi-extensions', xoviStart.id) && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
              {xoviActive ? 'Restart' : 'Start'}
            </Button>
          </div>
        </PanelRow>
    ) : null },
    { id: 'hashtable', node: hashtableRebuild ? (
        <PanelRow
          label="Hashtable"
          marker={markerFor('hashtable')}
          detail={!hashtab
            ? 'Not checked'
            : !hashtab.installed
              ? 'Not built'
              : hashtab.needsRebuild
                ? `Built for ${hashtab.hashtabVersion}, running ${hashtab.firmwareVersion}`
                : 'Current'}
          severity={rows.hashtable.severity}
        >
          <Button
            className={panelAction}
            variant={primaryFor('hashtable', hashtableBroken)}
            onClick={() => handleComponentMaintenance('qt-resource-rebuilder', hashtableRebuild.id)}
            disabled={(commandRunning && !isCommandRunning('qt-resource-rebuilder', hashtableRebuild.id)) || connectionStatus !== 'connected' || writableRootBusy}
          >
            {isCommandRunning('qt-resource-rebuilder', hashtableRebuild.id) && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            Rebuild
          </Button>
        </PanelRow>
    ) : null },
    { id: 'reenable', node: vellumInstalled ? (
        <PanelRow
          label="System partition mods"
          marker={markerFor('reenable')}
          detail={REENABLE_STATE[reenableStatus] ?? 'Not checked'}
          severity={rows.reenable.severity}
        >
          <Button
            className={panelAction}
            variant={primaryFor('reenable', reenableStatus === ReenableStatus.ReenableNeeded)}
            onClick={handleRunReenable}
            disabled={commandRunning || runningReenable || connectionStatus !== 'connected' || reenableStatus === ReenableStatus.ReenableUnneeded || writableRootBusy}
          >
            {runningReenable && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
            Re-enable
          </Button>
        </PanelRow>
    ) : null },
    { id: 'timezone', node: (
      <div className="flex items-end justify-between gap-4 py-3 first:pt-0 last:pb-0">
        <div className="flex min-w-0 gap-3">
          {markerFor('timezone') && <div className="mt-0.5">{markerFor('timezone')}</div>}
          <div className="min-w-0">
          <p className="text-sm font-semibold">Timezone</p>
          {timezoneMismatch?.needsUpdate && (
            <p className={cn('text-xs', SEVERITY_TEXT.warning)}>
              Your reMarkable is on {timezoneMismatch.deviceTimezone}
            </p>
          )}
          <div className="mt-1.5">
            <TimezoneCombobox
              value={selectedTimezone || timezoneMismatch?.deviceTimezone || ''}
              onChange={handleTimezoneChange}
              disabled={connectionStatus !== 'connected'}
              takeover={{
                device,
                deviceMachine: deviceInfo.machine,
                firmware: deviceInfo.firmware,
                onSettings,
                onDisconnect,
              }}
            />
          </div>
          </div>
        </div>
        <Button
          className={panelAction}
          variant={primaryFor('timezone', !!timezoneMismatch?.needsUpdate)}
          onClick={handleSetTimezone}
          disabled={settingTimezone || connectionStatus !== 'connected' || !selectedTimezone || selectedTimezone === deviceTimezone || writableRootBusy}
        >
          {settingTimezone && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
          Set
        </Button>
      </div>
    ) },

  ]

  const stepIds: string[] = updateSteps.map(step => step.id)
  const leadingIds = ['os', ...stepIds]
  const nodeById = new Map(statusRows.map(row => [row.id, row.node]))
  const orderedRows = showSteps
    ? [
        ...leadingIds.map(id => ({ id, node: nodeById.get(id) })),
        ...statusRows.filter(row => !leadingIds.includes(row.id)),
      ]
    : statusRows

  const toolsDisabled = connectionStatus !== 'connected'

  return (
    <>
      <TabsContent value="system">
        <hr className="border-border" />
        <div className="mx-auto max-w-5xl pt-6">
          <div className="grid grid-cols-1 gap-8 md:grid-cols-[minmax(0,1fr)_fit-content(22rem)] md:gap-10">
            <section>
              <SectionLabel>{showSteps ? 'OS change detected' : 'Status'}</SectionLabel>
              {introVersions && (
                <p ref={introRef} className="mb-3 text-xs leading-5 text-muted-foreground">
                  Your reMarkable OS changed from {introVersions.from} to {introVersions.to}.{' '}
                  {osMismatchDetected
                    ? 'Review mod compatibility to finish the update.'
                    : stepsSettled ? 'The update is finished.' : 'Finish the remaining steps to complete the update.'}
                </p>
              )}
              <div className="divide-y">
                {orderedRows.map(r => r.node && <Fragment key={r.id}>{r.node}</Fragment>)}
              </div>
            </section>

            <div className="space-y-8 md:min-w-[17rem]">
              <section>
                <SectionLabel>Tools</SectionLabel>

                <div className="grid grid-cols-3 gap-2 md:hidden">
                  {TOOLS.map(({ id, label, Icon }) => (
                    <button
                      key={id}
                      type="button"
                      onClick={() => openTool(id)}
                      disabled={toolsDisabled}
                      className="relative flex min-h-[84px] flex-col items-center justify-center gap-1.5 rounded-lg border p-2 text-center transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
                    >
                      {id === 'terminal' && shellRunning && (
                        <span
                          className="absolute right-2 top-2 h-2 w-2 rounded-full bg-[var(--info)]"
                          aria-label="Shell running"
                        />
                      )}
                      <Icon className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden />
                      <span className="text-[11px] font-medium leading-tight">{label}</span>
                    </button>
                  ))}
                </div>

                <div className="hidden divide-y md:block">
                  {TOOLS.map(({ id, label, description, action }) => {
                    const running = id === 'terminal' && shellRunning
                    return (
                      <PanelRow
                        key={id}
                        label={label}
                        detail={running ? 'Shell running' : description}
                        severity={running ? 'info' : null}
                      >
                        <Button
                          className={panelAction}
                          variant="outline"
                          onClick={() => openTool(id)}
                          disabled={toolsDisabled}
                        >
                          {running ? 'Resume' : action}
                        </Button>
                      </PanelRow>
                    )
                  })}
                </div>
              </section>

            {vellumInstalled && (
              <section>
                <SectionLabel>Advanced</SectionLabel>
                <div className="divide-y">
                  <div className="py-3 first:pt-0 last:pb-0">
                    <h4 className="text-sm font-semibold">Vellum</h4>
                    <p className="text-xs text-muted-foreground">Package manager for reMarkable tablets</p>
                    {/* The 48px touch overlay bleeds 6px past each 36px button, so wrapped rows need 12px between them or the overlays collide. */}
                    <div className="mt-2.5 flex flex-wrap gap-2 touch:gap-y-3">
                      <Button
                        className={toolButton}
                        variant="outline"
                        onClick={() => setShowRepairConfirm(true)}
                        disabled={commandRunning || connectionStatus !== 'connected' || writableRootBusy}
                      >
                        Repair
                      </Button>
                      <Button
                        className={toolButton}
                        variant="outline"
                        onClick={() => setShowCheckOSDialog(true)}
                        disabled={connectionStatus !== 'connected'}
                      >
                        Check OS
                      </Button>
                      <Button
                        className={toolButton}
                        variant="outline"
                        onClick={() => setShowUninstallConfirm(true)}
                        disabled={commandRunning || connectionStatus !== 'connected'}
                      >
                        Uninstall
                      </Button>
                    </div>
                  </div>

                  {advancedPackages.map(({ pkg, commands }) => (
                    <div key={pkg.name} className="py-3 first:pt-0 last:pb-0">
                      <h4 className="text-sm font-semibold">{pkg.name}</h4>
                      {pkg.description && <p className="text-xs text-muted-foreground">{pkg.description}</p>}
                      <div className="mt-2.5 flex flex-wrap gap-2 touch:gap-y-3">
                        {commands.map((cmd) => {
                          const running = isCommandRunning(pkg.name, cmd.id)
                          const blockedByLauncher = pkg.name === 'xovi-extensions' && cmd.id === 'stock' && launcherConfigured
                          return (
                            <Button
                              key={cmd.id}
                              className={toolButton}
                              variant="outline"
                              onClick={() => handleComponentMaintenance(pkg.name, cmd.id)}
                              disabled={blockedByLauncher || (commandRunning && !running) || connectionStatus !== 'connected' || writableRootBusy}
                            >
                              {running && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
                              {cmd.label}
                            </Button>
                          )
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            )}
            </div>
          </div>
        </div>
      </TabsContent>

      <Dialog open={showBackupChooser} onOpenChange={setShowBackupChooser}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Backup &amp; Restore</DialogTitle>
            <DialogDescription>
              Backups cover your documents and configuration. They do not cover installed mods or reManager settings.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter separateExit>
            <Button variant="outline" onClick={() => setShowBackupChooser(false)}>
              Cancel
            </Button>
            <Button variant="outline" onClick={() => { setShowBackupChooser(false); setBackupDialogMode('restore') }} disabled={commandRunning}>
              Restore
            </Button>
            <Button onClick={() => { setShowBackupChooser(false); setBackupDialogMode('backup') }} disabled={commandRunning}>
              Back up
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={sleepScreenRestartMessage !== null} onOpenChange={(open) => !open && setSleepScreenRestartMessage(null)}>
        <DialogContent>
          <DialogHeader severity="info" icon={ArrowsCounterClockwise}>
            <DialogTitle>Restart the reMarkable UI?</DialogTitle>
            <DialogDescription>
              {sleepScreenRestartMessage}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSleepScreenRestartMessage(null)}>
              Not now
            </Button>
            <Button onClick={restartUiForSleepScreen} disabled={commandRunning || connectionStatus !== 'connected'}>
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showGuideUpdateConfirm} onOpenChange={setShowGuideUpdateConfirm}>
        <DialogContent>
          <DialogHeader severity="warning" icon={Warning}>
            <DialogTitle>Update the reManager guide</DialogTitle>
            <DialogDescription>
              The new guide replaces the copy on your reMarkable. Highlights, notes and your reading
              position in the old copy are lost, and the new copy goes to the top level of your library.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowGuideUpdateConfirm(false)}>
              Cancel
            </Button>
            <Button onClick={() => { setShowGuideUpdateConfirm(false); handleInstallGuide() }} disabled={connectionStatus !== 'connected'}>
              Update
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showRepairConfirm} onOpenChange={setShowRepairConfirm}>
        <DialogContent>
          <DialogHeader severity="neutral" icon={Package}>
            <DialogTitle>Repair Vellum</DialogTitle>
            <DialogDescription>
              Reinstalls the latest version of Vellum. Your installed mods are kept.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRepairConfirm(false)}>
              Cancel
            </Button>
            <Button onClick={() => { setShowRepairConfirm(false); onRepairVellum() }} disabled={connectionStatus !== 'connected'}>
              Repair
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showUninstallConfirm} onOpenChange={setShowUninstallConfirm}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Uninstall Vellum</DialogTitle>
            <DialogDescription>
              {removePackages
                ? 'Vellum and everything it installed will be removed from your reMarkable.'
                : 'Vellum will be removed from your reMarkable. Installed mods are left in place.'}
            </DialogDescription>
          </DialogHeader>
          <PanelGroup>
            <PanelRow
              label="Also remove installed mods"
              htmlFor="remove-pkgs"
            >
              <Switch id="remove-pkgs" className="touch:hit-target" checked={removePackages} onCheckedChange={setRemovePackages} />
            </PanelRow>
          </PanelGroup>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowUninstallConfirm(false)}>Cancel</Button>
            <Button variant="destructive" onClick={() => { setShowUninstallConfirm(false); onUninstallVellum(removePackages) }} disabled={connectionStatus !== 'connected'}>
              Uninstall
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CheckOSDialog
        open={showCheckOSDialog}
        onOpenChange={setShowCheckOSDialog}
        isConnected={connectionStatus === 'connected'}
        onSelectPackage={handleSelectPackageForOS}
      />

      <ImportPDFDialog
        open={showImportPDF}
        isConnected={connectionStatus === 'connected'}
        hasLibrarian={installedPackages.has('librarian')}
        xoviActive={xoviActive}
        onOpenChange={setShowImportPDF}
      />

      <SoftwareManagerDialog
        open={showSoftwareManager}
        onOpenChange={onShowSoftwareManagerChange}
        isConnected={connectionStatus === 'connected'}
        vellumInstalled={vellumInstalled}
        onSelectPackageForOS={handleSelectPackageForOS}
      />

      <BackupRestoreDialog
        mode={backupDialogMode}
        isConnected={connectionStatus === 'connected'}
        onClose={closeBackupDialog}
      />

      {/* Kept mounted and hidden rather than conditionally rendered: unmounting InteractiveTerminal runs its cleanup, which stops the shell. */}
      <TakeoverPage
        title="Terminal"
        narrowTitle="Terminal"
        device={device}
        deviceMachine={deviceInfo.machine}
        firmware={deviceInfo.firmware}
        onSettings={onSettings}
        onDisconnect={onDisconnect}
        onClose={() => setShowTerminal(false)}
        fill
        hidden={!showTerminal}
      >
        <InteractiveTerminal
          isConnected={connectionStatus === 'connected'}
          visible={showTerminal}
          onRunningChange={setShellRunning}
          theme={resolvedTerminalTheme as 'dark' | 'light'}
        />
      </TakeoverPage>

      {showFileBrowser && (
        <TakeoverPage
          title={pickingSleepScreen ? 'Choose an image' : 'File Browser'}
          narrowTitle={pickingSleepScreen ? 'Choose image' : 'Files'}
          device={device}
          deviceMachine={deviceInfo.machine}
          firmware={deviceInfo.firmware}
          onSettings={onSettings}
          onDisconnect={onDisconnect}
          onClose={closeFileBrowser}
          showCloseButton={false}
        >
          <FileBrowser
            isConnected={connectionStatus === 'connected'}
            suppressSystemFileWarnings={suppressSystemFileWarnings}
            isVisible={showFileBrowser}
            sleepScreenSupported={sleepScreenSupported}
            currentSleepScreen={currentSleepScreen}
            onSleepScreenChange={setCurrentSleepScreen}
            initialPath={fileBrowserPath}
            pickMode={pickingSleepScreen}
            onClose={closeFileBrowser}
            onPick={(path) => {
              closeFileBrowser()
              void setSleepScreen(path)
            }}
          />
        </TakeoverPage>
      )}

      {showConfigEditor && (
        <TakeoverPage
          title="Configuration"
          narrowTitle="Configuration"
          device={device}
          deviceMachine={deviceInfo.machine}
          firmware={deviceInfo.firmware}
          onSettings={onSettings}
          onDisconnect={onDisconnect}
          onClose={requestCloseConfigEditor}
          fill
        >
          <Suspense fallback={null}>
            <ConfigEditor
              isConnected={connectionStatus === 'connected'}
              theme={resolvedEditorTheme as 'dark' | 'light'}
              onDirtyChange={setConfigEditorDirty}
            />
          </Suspense>
        </TakeoverPage>
      )}

      <Dialog open={confirmDiscardConfig} onOpenChange={setConfirmDiscardConfig}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Discard changes?</DialogTitle>
            <DialogDescription>
              Your edits to xochitl.conf have not been saved.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDiscardConfig(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={closeConfigEditor}>
              Discard
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
