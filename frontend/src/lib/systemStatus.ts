import type { PanelSeverity } from '@/components/Panel'
import { launcherSelected } from '@/lib/format'
import { compareVersions } from '@/lib/version'
import {
  HashtabVersionStatus,
  MaintenanceCommandInfo,
  OSChannelStatus,
  ReenableStatus,
  SystemTaskInfo,
  TimezoneStatus,
  UpdateServiceStatus,
  UserGuideStatus,
} from '@/lib/types'

export const PROMOTED_COMMANDS = {
  'xovi-extensions': { start: 'start', stop: 'stock' },
  'qt-resource-rebuilder': { rebuild: 'rebuild_hashtable' },
  launcherctl: { start: 'start-launcher', stop: 'stop-launcher' },
} as const

export const promotedIds = (pkgName: string): readonly string[] =>
  Object.values(PROMOTED_COMMANDS[pkgName as keyof typeof PROMOTED_COMMANDS] ?? {})

export interface DeviceFacts {
  installedPackages: Map<string, string>
  maintenanceCommands: Record<string, MaintenanceCommandInfo[]>
  systemTasks: SystemTaskInfo[]
  vellumInstalled: boolean | null
  updateService: UpdateServiceStatus
  xochitlRunning: boolean
  xoviActive: boolean
  activeLauncher: string
  currentLauncher: string
  hashtab: HashtabVersionStatus | null
  osMismatchDetected: boolean
  osChannel: OSChannelStatus | null
  incompatibleWithLatestOS: string[]
  reenableStatus: ReenableStatus
  timezone: TimezoneStatus | null
  guide: UserGuideStatus | null
}

export type StatusRowId =
  | 'autoUpdates' | 'os' | 'packages' | 'ui' | 'guide' | 'launcher' | 'mods' | 'hashtable' | 'reenable' | 'timezone'

export interface StatusRow {
  shown: boolean
  severity: PanelSeverity | null
}

export function deriveSystemStatus(facts: DeviceFacts) {
  const promotedCommand = (pkgName: string, id: string) =>
    facts.installedPackages.has(pkgName)
      ? facts.maintenanceCommands[pkgName]?.find(c => c.id === id)
      : undefined

  const xoviStart = promotedCommand('xovi-extensions', PROMOTED_COMMANDS['xovi-extensions'].start)
  // xovi-extensions declares no stop; `stock` restarts the interface without the mod framework, the only route back to mods-stopped.
  const xoviStop = promotedCommand('xovi-extensions', PROMOTED_COMMANDS['xovi-extensions'].stop)
  const hashtableRebuild = promotedCommand('qt-resource-rebuilder', PROMOTED_COMMANDS['qt-resource-rebuilder'].rebuild)
  const launcherActive = launcherSelected(facts.activeLauncher)
  const launcherConfigured = launcherSelected(facts.currentLauncher)
  const launcherAction = launcherActive
    ? promotedCommand('launcherctl', PROMOTED_COMMANDS.launcherctl.stop)
    : promotedCommand('launcherctl', PROMOTED_COMMANDS.launcherctl.start)
  const autoUpdateTasks = facts.systemTasks.filter(t => t.id === 'enable-updates' || t.id === 'disable-updates')
  const restartUiTask = facts.systemTasks.find(t => t.id === 'restart-xochitl')

  const autoUpdatesActive = facts.updateService.enabled || facts.updateService.running
  const hashtableBroken = !!hashtableRebuild && !!facts.hashtab
    && (!facts.hashtab.installed || facts.hashtab.needsRebuild)
  const uiNeedsRestart = !facts.xochitlRunning && !launcherConfigured && !launcherActive
  const launcherStalled = launcherConfigured && !launcherActive
  const xoviNeedsStart = facts.xochitlRunning && !facts.xoviActive && !launcherConfigured && !hashtableBroken
  const osComparedToLatest = facts.osChannel
    ? compareVersions(facts.osChannel.activeVersion, facts.osChannel.latestPublic)
    : 0
  const osUpdateAvailable = osComparedToLatest < 0
  const osPrerelease = osComparedToLatest > 0
  const osUpdateBlocked = osUpdateAvailable && facts.incompatibleWithLatestOS.length > 0

  const row = (shown: boolean, severity: PanelSeverity | null): StatusRow =>
    ({ shown, severity: shown ? severity : null })

  const rows: Record<StatusRowId, StatusRow> = {
    autoUpdates: row(autoUpdateTasks.length > 0, autoUpdatesActive ? 'warning' : null),
    os: row(true, osUpdateAvailable ? 'info' : null),
    packages: row(facts.osMismatchDetected, 'warning'),
    ui: row(!!restartUiTask, uiNeedsRestart ? 'destructive' : null),
    guide: row(!!facts.guide && !facts.guide.skipped, facts.guide?.needsUpdate ? 'info' : null),
    launcher: row(!!launcherAction, launcherStalled ? (facts.xochitlRunning ? 'warning' : 'destructive') : null),
    mods: row(!!xoviStart, xoviNeedsStart ? 'warning' : null),
    hashtable: row(!!hashtableRebuild, hashtableBroken ? 'warning' : null),
    reenable: row(!!facts.vellumInstalled, facts.reenableStatus === ReenableStatus.ReenableNeeded ? 'warning' : null),
    timezone: row(true, facts.timezone?.needsUpdate ? 'warning' : null),
  }

  return {
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
    osPrerelease,
    osUpdateBlocked,
  }
}

export type SystemStatus = ReturnType<typeof deriveSystemStatus>
