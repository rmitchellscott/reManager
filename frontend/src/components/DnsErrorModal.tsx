import { Warning } from '@phosphor-icons/react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
interface DnsErrorModalProps {
  open: boolean
  onClose: () => void
  onEnableProxyMode: () => void
}

export function DnsErrorModal({ open, onClose, onEnableProxyMode }: DnsErrorModalProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent size="md">
        <DialogHeader severity="warning" icon={Warning}>
          <DialogTitle>DNS error</DialogTitle>
          <DialogDescription>
            Your reMarkable could not resolve DNS to download packages.
          </DialogDescription>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          reManager can download them on your computer and transfer them across instead. You can turn this off later under Settings › Connection.
        </p>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Dismiss
          </Button>
          <Button onClick={onEnableProxyMode}>
            Download through reManager
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
