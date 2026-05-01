import { BookOpen, X, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertAction } from '@/components/ui/alert'

interface UserGuideOfferProps {
  type: 'install' | 'update'
  installing: boolean
  onInstall: () => void
  onDismiss: () => void
  onDismissPermanently: () => void
}

export function UserGuideOffer({
  type,
  installing,
  onInstall,
  onDismiss,
  onDismissPermanently,
}: UserGuideOfferProps) {
  return (
    <Alert className="border-blue-200 bg-blue-50 dark:border-blue-900 dark:bg-blue-950 [&>svg]:text-blue-600 dark:[&>svg]:text-blue-400">
      <BookOpen className="h-4 w-4" />
      <AlertDescription>
        {type === 'install'
          ? 'A user guide for reManager is available. Install it on your reMarkable?'
          : 'An updated user guide is available for your reMarkable.'}
      </AlertDescription>
      <AlertAction className="top-1/2 -translate-y-1/2">
        <Button variant="outline" size="xs" onClick={onDismissPermanently} disabled={installing}>
          Don't ask again
        </Button>
        <Button size="xs" onClick={onInstall} disabled={installing}>
          {installing ? (
            <>
              <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
              Installing...
            </>
          ) : (
            type === 'install' ? 'Install' : 'Update'
          )}
        </Button>
        <Button variant="ghost" size="xs" onClick={onDismiss} disabled={installing}>
          <X className="h-3 w-3" />
        </Button>
      </AlertAction>
    </Alert>
  )
}
