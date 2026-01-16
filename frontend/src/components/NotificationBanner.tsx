import { AlertTriangle, X, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'

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
    <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-4 flex items-center justify-between gap-4">
      <div className="flex items-center gap-3">
        <AlertTriangle className="h-5 w-5 text-yellow-500 flex-shrink-0" />
        <span className="text-sm">{message}</span>
      </div>
      <div className="flex items-center gap-2 flex-shrink-0">
        <Button size="sm" onClick={onAction} disabled={loading}>
          {loading ? (
            <>
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              {loadingLabel}
            </>
          ) : (
            actionLabel
          )}
        </Button>
        {onDismiss && (
          <Button variant="ghost" size="sm" onClick={onDismiss}>
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  )
}
