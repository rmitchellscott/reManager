import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { AlertTriangle } from 'lucide-react'

interface DnsErrorModalProps {
  open: boolean
  onClose: () => void
  onEnableProxyMode: () => void
}

export function DnsErrorModal({ open, onClose, onEnableProxyMode }: DnsErrorModalProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <AlertTriangle className="h-6 w-6 text-yellow-500" />
            <DialogTitle>DNS Error Detected</DialogTitle>
          </div>
          <DialogDescription className="pt-2">
            Your reMarkable couldn't resolve DNS to download packages. Enabling Proxy Mode allows reManager to download packages on your computer and transfer them to the reMarkable, bypassing DNS issues. You can disable this later in Settings.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="flex gap-2 sm:gap-0">
          <Button variant="outline" onClick={onClose}>
            Dismiss
          </Button>
          <Button onClick={onEnableProxyMode}>
            Enable Proxy Mode
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
