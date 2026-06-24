import { AlertTriangle, Copy, CopyCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

export interface ShellRcNoisyDialogProps {
  open: boolean
  onRetry: () => void
  onClose: () => void
  isRetrying: boolean
}

const guard = `case $- in
  *i*) ;;
    *) return ;;
esac`

export function ShellRcNoisyDialog({ open, onRetry, onClose, isRetrying }: ShellRcNoisyDialogProps) {
  const { isCopied, copyToClipboard } = useCopyToClipboard()

  return (
    <Dialog open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose() }}>
      <DialogContent>
        <DialogHeader>
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-6 w-6 text-amber-500" />
            <DialogTitle>Your reMarkable's shell is printing on SSH</DialogTitle>
          </div>
          <DialogDescription>
            reManager talks to your reMarkable over non-interactive SSH, which must stay silent.
            Your reMarkable sources <code className="text-xs">~/.bashrc</code> for every session,
            so anything it prints corrupts file transfers and device detection.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            Add this to the very top of <code className="text-xs">~/.bashrc</code>, before anything that prints:
          </p>
          <div className="relative">
            <pre className="rounded-md bg-muted p-3 pr-12 text-xs overflow-x-auto"><code>{guard}</code></pre>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => copyToClipboard(guard)}
              className="absolute top-2 right-2 h-7 w-7"
            >
              {isCopied ? <CopyCheck className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            </Button>
          </div>
          <p className="text-sm text-muted-foreground">
            If the output comes from <code className="text-xs">~/.profile</code> or{' '}
            <code className="text-xs">/etc/profile.d/*.sh</code>, apply the same guard there.
          </p>
        </div>

        <DialogFooter className="flex-col sm:flex-row gap-2">
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
          <Button onClick={onRetry} disabled={isRetrying}>
            {isRetrying ? 'Retrying...' : 'Try again'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
