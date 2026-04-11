import { AlertTriangle, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Alert, AlertTitle, AlertDescription, AlertAction } from '@/components/ui/alert'

interface ConsolidatedWarningBannerProps {
  warnings: {
    hashtabMismatch?: { hashtabVersion: string; firmwareVersion: string }
    timezoneMismatch?: { deviceTimezone: string; savedTimezone: string }
    osUpgrade?: { prevVersion: string; newVersion: string }
    autoUpdatesEnabled?: boolean
    reenableNeeded?: boolean
  }
  onGoToMaintenance: () => void
  onDismiss: () => void
}

export function ConsolidatedWarningBanner({
  warnings,
  onGoToMaintenance,
  onDismiss,
}: ConsolidatedWarningBannerProps) {
  const hasWarnings = warnings.osUpgrade || warnings.hashtabMismatch || warnings.timezoneMismatch || warnings.autoUpdatesEnabled || warnings.reenableNeeded

  if (!hasWarnings) return null

  return (
    <Alert className="border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-400">
      <AlertTriangle className="h-4 w-4" />
      <AlertTitle className="text-amber-900 dark:text-amber-50">Action Required</AlertTitle>
      <AlertDescription>
        <ul className="space-y-1 list-disc pl-4">
          {warnings.osUpgrade && (
            <li>OS change detected ({warnings.osUpgrade.prevVersion} → {warnings.osUpgrade.newVersion}). Run reenable to restore mods.</li>
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
        </ul>
      </AlertDescription>
      <AlertAction className="top-1/2 -translate-y-1/2">
        <Button variant="link" size="sm" className="h-auto p-0 pr-10" onClick={onGoToMaintenance}>
          Go to Maintenance →
        </Button>
      </AlertAction>
      <Button variant="ghost" size="xs" className="absolute right-2 top-2" onClick={onDismiss}>
        <X className="h-3 w-3" />
      </Button>
    </Alert>
  )
}
