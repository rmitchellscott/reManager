import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'

export interface FilesystemRestoreErrorDialogProps {
  open: boolean
  onRetry: () => void
  onReboot: () => void
  onDismiss: () => void
  isRetrying: boolean
}

export function FilesystemRestoreErrorDialog({
  open,
  onRetry,
  onReboot,
  onDismiss,
  isRetrying,
}: FilesystemRestoreErrorDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) onDismiss() }}>
      <DialogContent>
        <DialogHeader>
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-6 w-6 text-amber-500" />
            <DialogTitle>Unable to Re-enable Read-Only Filesystem</DialogTitle>
          </div>
          <DialogDescription>
            Your changes were saved, but the read-only filesystem could not be re-enabled.
            A reboot will re-enable it without affecting your changes.
            This can be done at any time.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter className="flex-col sm:flex-row gap-2">
          <Button variant="outline" onClick={onDismiss}>
            Dismiss
          </Button>
          <Button variant="outline" onClick={onRetry} disabled={isRetrying}>
            {isRetrying ? 'Retrying...' : 'Retry'}
          </Button>
          <Button variant="destructive" onClick={onReboot}>
            Reboot Device
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
