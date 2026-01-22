import { AlertTriangle, X, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertAction } from '@/components/ui/alert'

interface NotificationBannerProps {
  message: string
  actionLabel: string
  onAction: () => void
  onDismiss?: () => void
  loading?: boolean
  loadingLabel?: string
}

export function NotificationBanner({
  message,
  actionLabel,
  onAction,
  onDismiss,
  loading = false,
  loadingLabel = 'Running...'
}: NotificationBannerProps) {
  return (
    <Alert className="border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-400">
      <AlertTriangle className="h-4 w-4" />
      <AlertDescription>{message}</AlertDescription>
      <AlertAction>
        <Button size="xs" onClick={onAction} disabled={loading}>
          {loading ? (
            <>
              <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
              {loadingLabel}
            </>
          ) : (
            actionLabel
          )}
        </Button>
        {onDismiss && (
          <Button variant="ghost" size="xs" onClick={onDismiss}>
            <X className="h-3 w-3" />
          </Button>
        )}
      </AlertAction>
    </Alert>
  )
}
