import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Banner } from '@/components/ui/banner'

export interface ConsolidatedWarningBannerProps {
  warnings: {
    hashtabMismatch?: { hashtabVersion: string; firmwareVersion: string }
    timezoneMismatch?: { deviceTimezone: string; savedTimezone: string }
    osUpgrade?: { prevVersion: string; newVersion: string }
    autoUpdatesEnabled?: boolean
    reenableNeeded?: boolean
    xoviNotRunning?: boolean
    hashtabMissing?: boolean
  }
  onGoToMaintenance: () => void
  onDismiss: () => void
}

export function hasActiveWarnings(warnings: ConsolidatedWarningBannerProps['warnings']): boolean {
  return !!(warnings.osUpgrade || warnings.hashtabMismatch || warnings.timezoneMismatch
    || warnings.autoUpdatesEnabled || warnings.reenableNeeded || warnings.xoviNotRunning
    || warnings.hashtabMissing)
}

export function ConsolidatedWarningBanner({
  warnings,
  onGoToMaintenance,
  onDismiss,
}: ConsolidatedWarningBannerProps) {
  if (!hasActiveWarnings(warnings)) return null

  return (
    <Banner
      severity="warning"
      icon={AlertTriangle}
      title="Action Required"
      onDismiss={onDismiss}
      actions={
        <Button size="sm" onClick={onGoToMaintenance}>
          Go to Maintenance
        </Button>
      }
    >
      <ul className="space-y-1 list-disc pl-4">
        {warnings.osUpgrade && (
          <li>OS change detected ({warnings.osUpgrade.prevVersion} → {warnings.osUpgrade.newVersion}). Run reenable to restore mods.</li>
        )}
        {warnings.hashtabMissing && (
          <li>Hashtable has not been built. Build it from the Maintenance tab to enable UI mods.</li>
        )}
        {warnings.hashtabMismatch && (
          <li>Hashtable built for OS {warnings.hashtabMismatch.hashtabVersion}, but device is running {warnings.hashtabMismatch.firmwareVersion}</li>
        )}
        {warnings.timezoneMismatch && (
          <li>Device timezone ({warnings.timezoneMismatch.deviceTimezone}) differs from your preference ({warnings.timezoneMismatch.savedTimezone})</li>
        )}
        {warnings.autoUpdatesEnabled && (
          <li>Auto-updates are enabled and may interfere with mods</li>
        )}
        {warnings.reenableNeeded && (
          <li>Reenable is needed to restore packages that modify the system partition</li>
        )}
        {warnings.xoviNotRunning && (
          <li>Mods are installed but not running. Start UI with Mods from the Maintenance tab.</li>
        )}
      </ul>
    </Banner>
  )
}
