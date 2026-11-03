import { Warning } from '@phosphor-icons/react'
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
  isConnected: boolean
}

export function FilesystemRestoreErrorDialog({
  open,
  onRetry,
  onReboot,
  onDismiss,
  isRetrying,
  isConnected,
}: FilesystemRestoreErrorDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) onDismiss() }}>
      <DialogContent>
        <DialogHeader severity="warning" icon={Warning}>
          <DialogTitle>Could not re-enable the read-only filesystem</DialogTitle>
          <DialogDescription>
            Your changes were saved.
          </DialogDescription>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          Rebooting re-enables it without affecting your changes. You can reboot at any time.
        </p>

        <DialogFooter separateExit>
          <Button variant="outline" onClick={onDismiss}>
            Dismiss
          </Button>
          <Button variant="outline" onClick={onRetry} disabled={isRetrying || !isConnected}>
            {isRetrying ? 'Retrying…' : 'Try again'}
          </Button>
          <Button variant="destructive" onClick={onReboot} disabled={!isConnected}>
            Reboot reMarkable
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
