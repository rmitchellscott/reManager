import { describe, expect, it } from 'vitest'
import { deriveSystemStatus, type DeviceFacts } from '@/lib/systemStatus'
import { systemAlertLevel } from '@/lib/alerts'
import {
  ReenableStatus,
  type HashtabVersionStatus,
  type MaintenanceCommandInfo,
  type SystemTaskInfo,
} from '@/lib/types'

function command(id: string): MaintenanceCommandInfo {
  return { id, label: id, description: '', requiresTerminal: false, allowStop: false }
}

function task(id: string): SystemTaskInfo {
  return { id, label: id, description: '', requiresTerminal: false, needsWritableRoot: false }
}

const catalog: Record<string, MaintenanceCommandInfo[]> = {
  'xovi-extensions': [command('start'), command('stock')],
  'qt-resource-rebuilder': [command('rebuild_hashtable')],
  launcherctl: [command('start-launcher'), command('stop-launcher')],
}

const staleHashtab: HashtabVersionStatus = {
  installed: true,
  hashtabVersion: '3.20.0',
  firmwareVersion: '3.22.0',
  needsRebuild: true,
}

function facts(overrides: Partial<DeviceFacts> = {}): DeviceFacts {
  return {
    installedPackages: new Map(),
    maintenanceCommands: catalog,
    systemTasks: [],
    vellumInstalled: null,
    updateService: { enabled: false, running: false },
    xochitlRunning: true,
    xoviActive: false,
    activeLauncher: '',
    currentLauncher: '',
    hashtab: null,
    osMismatchDetected: false,
    osChannel: null,
    incompatibleWithLatestOS: [],
    reenableStatus: ReenableStatus.$zero,
    timezone: null,
    guide: null,
    ...overrides,
  }
}

function installed(...names: string[]): Map<string, string> {
  return new Map(names.map(name => [name, '1.0.0']))
}

describe('deriveSystemStatus', () => {
  it('shows only the OS and timezone rows, with no severity, when no facts have arrived', () => {
    const { rows } = deriveSystemStatus(facts())

    const shown = Object.entries(rows).filter(([, row]) => row.shown).map(([id]) => id)
    expect(shown.sort()).toEqual(['os', 'timezone'])
    expect(Object.values(rows).every(row => row.severity === null)).toBe(true)
  })

  it('never gives a hidden row a severity', () => {
    const status = deriveSystemStatus(facts({
      osMismatchDetected: false,
      xochitlRunning: false,
      updateService: { enabled: true, running: true },
      hashtab: staleHashtab,
      reenableStatus: ReenableStatus.ReenableNeeded,
      currentLauncher: 'appload',
    }))

    for (const row of Object.values(status.rows)) {
      if (!row.shown) expect(row.severity).toBeNull()
    }
  })

  describe('hashtable', () => {
    it('lights no row for a hashtab mismatch on a tablet without qt-resource-rebuilder', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('xovi-extensions'),
        hashtab: staleHashtab,
      }))

      expect(status.rows.hashtable).toEqual({ shown: false, severity: null })
      expect(status.hashtableBroken).toBe(false)
      expect(status.hashtableRebuild).toBeUndefined()
    })

    it('warns on the hashtable row when qt-resource-rebuilder is installed and the hashtab is stale', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('qt-resource-rebuilder', 'xovi-extensions'),
        hashtab: staleHashtab,
      }))

      expect(status.rows.hashtable).toEqual({ shown: true, severity: 'warning' })
      expect(status.hashtableRebuild?.id).toBe('rebuild_hashtable')
    })

    it('treats a missing hashtab as broken when the rebuilder is installed', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('qt-resource-rebuilder'),
        hashtab: { ...staleHashtab, installed: false, needsRebuild: false },
      }))

      expect(status.rows.hashtable.severity).toBe('warning')
    })

    it('does not warn when the hashtab was never reported', () => {
      const status = deriveSystemStatus(facts({ installedPackages: installed('qt-resource-rebuilder') }))

      expect(status.rows.hashtable).toEqual({ shown: true, severity: null })
    })

    it('does not also ask to start xovi while the hashtable is broken', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('qt-resource-rebuilder', 'xovi-extensions'),
        hashtab: staleHashtab,
        xoviActive: false,
      }))

      expect(status.rows.mods).toEqual({ shown: true, severity: null })
    })
  })

  describe('OS', () => {
    it('reports an update when the active version is behind the latest public release', () => {
      const status = deriveSystemStatus(facts({ osChannel: { activeVersion: '3.20.0.92', latestPublic: '3.22.4.2' } }))

      expect(status.rows.os).toEqual({ shown: true, severity: 'info' })
      expect(status.osUpdateAvailable).toBe(true)
      expect(status.osPrerelease).toBe(false)
    })

    it('compares versions numerically, not as strings', () => {
      const status = deriveSystemStatus(facts({ osChannel: { activeVersion: '3.9.0', latestPublic: '3.10.0' } }))

      expect(status.osUpdateAvailable).toBe(true)
    })

    it('reports a prerelease, without severity, when the active version is ahead of the latest public release', () => {
      const status = deriveSystemStatus(facts({ osChannel: { activeVersion: '3.23.0.64', latestPublic: '3.22.4.2' } }))

      expect(status.rows.os).toEqual({ shown: true, severity: null })
      expect(status.osPrerelease).toBe(true)
      expect(status.osUpdateAvailable).toBe(false)
    })

    it('reports neither when the versions match', () => {
      const status = deriveSystemStatus(facts({ osChannel: { activeVersion: '3.22.4.2', latestPublic: '3.22.4.2' } }))

      expect(status.osUpdateAvailable).toBe(false)
      expect(status.osPrerelease).toBe(false)
    })

    it('reports neither when the OS channel is unknown', () => {
      const status = deriveSystemStatus(facts({ osChannel: null }))

      expect(status.rows.os.severity).toBeNull()
      expect(status.osUpdateAvailable).toBe(false)
      expect(status.osPrerelease).toBe(false)
    })

    it('blocks an available update only when installed packages lack a build for it', () => {
      const osChannel = { activeVersion: '3.20.0', latestPublic: '3.22.0' }

      expect(deriveSystemStatus(facts({ osChannel, incompatibleWithLatestOS: ['xovi'] })).osUpdateBlocked).toBe(true)
      expect(deriveSystemStatus(facts({ osChannel, incompatibleWithLatestOS: [] })).osUpdateBlocked).toBe(false)
      expect(deriveSystemStatus(facts({
        osChannel: { activeVersion: '3.22.0', latestPublic: '3.22.0' },
        incompatibleWithLatestOS: ['xovi'],
      })).osUpdateBlocked).toBe(false)
    })
  })

  describe('mods', () => {
    it('is hidden when xovi-extensions is in the catalog but not installed', () => {
      const status = deriveSystemStatus(facts({ xoviActive: false }))

      expect(status.rows.mods).toEqual({ shown: false, severity: null })
      expect(status.xoviStart).toBeUndefined()
    })

    it('warns when the interface runs without xovi', () => {
      const status = deriveSystemStatus(facts({ installedPackages: installed('xovi-extensions'), xoviActive: false }))

      expect(status.rows.mods).toEqual({ shown: true, severity: 'warning' })
      expect(status.xoviStart?.id).toBe('start')
      expect(status.xoviStop?.id).toBe('stock')
    })

    it('does not warn while xovi is active', () => {
      const status = deriveSystemStatus(facts({ installedPackages: installed('xovi-extensions'), xoviActive: true }))

      expect(status.rows.mods.severity).toBeNull()
    })

    it('leaves a configured launcher to start xovi rather than warning', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('xovi-extensions'),
        xoviActive: false,
        currentLauncher: 'appload',
        activeLauncher: 'appload',
      }))

      expect(status.rows.mods.severity).toBeNull()
    })
  })

  describe('launcher', () => {
    it('offers start when no launcher is active and stop when one is', () => {
      const stopped = deriveSystemStatus(facts({ installedPackages: installed('launcherctl') }))
      const running = deriveSystemStatus(facts({ installedPackages: installed('launcherctl'), activeLauncher: 'appload' }))

      expect(stopped.launcherAction?.id).toBe('start-launcher')
      expect(running.launcherAction?.id).toBe('stop-launcher')
    })

    it('counts "none" as no launcher selected', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('launcherctl'),
        activeLauncher: 'none',
        currentLauncher: 'none',
      }))

      expect(status.launcherActive).toBe(false)
      expect(status.launcherConfigured).toBe(false)
      expect(status.rows.launcher.severity).toBeNull()
    })

    it('warns when a configured launcher is not running but the interface is', () => {
      const status = deriveSystemStatus(facts({ installedPackages: installed('launcherctl'), currentLauncher: 'appload' }))

      expect(status.rows.launcher).toEqual({ shown: true, severity: 'warning' })
    })

    it('is destructive when a configured launcher is not running and neither is the interface', () => {
      const status = deriveSystemStatus(facts({
        installedPackages: installed('launcherctl'),
        currentLauncher: 'appload',
        xochitlRunning: false,
      }))

      expect(status.rows.launcher).toEqual({ shown: true, severity: 'destructive' })
    })

    it('is hidden when launcherctl is not installed', () => {
      const status = deriveSystemStatus(facts({ currentLauncher: 'appload', xochitlRunning: false }))

      expect(status.rows.launcher).toEqual({ shown: false, severity: null })
    })
  })

  describe('interface', () => {
    it('is destructive when the interface is down and no launcher stands in for it', () => {
      const status = deriveSystemStatus(facts({ systemTasks: [task('restart-xochitl')], xochitlRunning: false }))

      expect(status.rows.ui).toEqual({ shown: true, severity: 'destructive' })
    })

    it('does not blame the interface when a launcher is configured in its place', () => {
      const status = deriveSystemStatus(facts({
        systemTasks: [task('restart-xochitl')],
        xochitlRunning: false,
        currentLauncher: 'appload',
      }))

      expect(status.rows.ui.severity).toBeNull()
    })

    it('is hidden without the restart task', () => {
      expect(deriveSystemStatus(facts({ xochitlRunning: false })).rows.ui).toEqual({ shown: false, severity: null })
    })
  })

  describe('automatic updates', () => {
    const tasks = [task('enable-updates'), task('disable-updates')]

    it('warns when the update service is on', () => {
      const status = deriveSystemStatus(facts({ systemTasks: tasks, updateService: { enabled: true, running: true } }))

      expect(status.rows.autoUpdates).toEqual({ shown: true, severity: 'warning' })
    })

    it('reads a half-applied service as on', () => {
      expect(deriveSystemStatus(facts({ systemTasks: tasks, updateService: { enabled: true, running: false } })).autoUpdatesActive).toBe(true)
      expect(deriveSystemStatus(facts({ systemTasks: tasks, updateService: { enabled: false, running: true } })).autoUpdatesActive).toBe(true)
    })

    it('does not warn when the service is off', () => {
      expect(deriveSystemStatus(facts({ systemTasks: tasks })).rows.autoUpdates).toEqual({ shown: true, severity: null })
    })

    it('is hidden without the tasks that toggle it', () => {
      const status = deriveSystemStatus(facts({ updateService: { enabled: true, running: true } }))

      expect(status.rows.autoUpdates).toEqual({ shown: false, severity: null })
    })
  })

  describe('packages after an OS change', () => {
    it('warns only when the OS changed under vellum', () => {
      expect(deriveSystemStatus(facts({ osMismatchDetected: true })).rows.packages).toEqual({ shown: true, severity: 'warning' })
      expect(deriveSystemStatus(facts({ osMismatchDetected: false })).rows.packages).toEqual({ shown: false, severity: null })
    })
  })

  describe('re-enable', () => {
    it('warns when vellum is installed and re-enabling is needed', () => {
      const status = deriveSystemStatus(facts({ vellumInstalled: true, reenableStatus: ReenableStatus.ReenableNeeded }))

      expect(status.rows.reenable).toEqual({ shown: true, severity: 'warning' })
    })

    it('does not warn for any other status', () => {
      for (const reenableStatus of [ReenableStatus.$zero, ReenableStatus.ReenableOK, ReenableStatus.ReenableUnneeded]) {
        expect(deriveSystemStatus(facts({ vellumInstalled: true, reenableStatus })).rows.reenable.severity).toBeNull()
      }
    })

    it('is hidden while it is unknown whether vellum is installed', () => {
      const status = deriveSystemStatus(facts({ vellumInstalled: null, reenableStatus: ReenableStatus.ReenableNeeded }))

      expect(status.rows.reenable).toEqual({ shown: false, severity: null })
    })
  })

  describe('timezone', () => {
    it('warns when the device timezone differs from the saved one', () => {
      const status = deriveSystemStatus(facts({
        timezone: { deviceTimezone: 'UTC', savedTimezone: 'America/Denver', needsUpdate: true },
      }))

      expect(status.rows.timezone).toEqual({ shown: true, severity: 'warning' })
    })

    it('does not warn when the timezone is unknown', () => {
      expect(deriveSystemStatus(facts({ timezone: null })).rows.timezone.severity).toBeNull()
    })
  })

  describe('guide', () => {
    const guide = { installed: true, needsUpdate: false, skipped: false, unseen: false, docId: 'doc' }

    it('is hidden until the guide status arrives, and when the user skipped it', () => {
      expect(deriveSystemStatus(facts({ guide: null })).rows.guide.shown).toBe(false)
      expect(deriveSystemStatus(facts({ guide: { ...guide, skipped: true, needsUpdate: true } })).rows.guide).toEqual({ shown: false, severity: null })
    })

    it('is info when the guide on the tablet is out of date', () => {
      expect(deriveSystemStatus(facts({ guide: { ...guide, needsUpdate: true } })).rows.guide).toEqual({ shown: true, severity: 'info' })
      expect(deriveSystemStatus(facts({ guide })).rows.guide).toEqual({ shown: true, severity: null })
    })
  })

  it('feeds the tab dot only from shown rows', () => {
    const status = deriveSystemStatus(facts({
      installedPackages: installed('xovi-extensions'),
      xoviActive: true,
      hashtab: staleHashtab,
      updateService: { enabled: true, running: true },
      reenableStatus: ReenableStatus.ReenableNeeded,
      osChannel: { activeVersion: '3.22.0', latestPublic: '3.22.0' },
    }))

    expect(systemAlertLevel(Object.values(status.rows), { guideNews: false, shellRunning: false })).toBeNull()
  })
})
