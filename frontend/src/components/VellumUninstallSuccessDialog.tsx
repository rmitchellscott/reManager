import { CheckCircle2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'

interface VellumUninstallSuccessDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function VellumUninstallSuccessDialog({
  open,
  onOpenChange,
}: VellumUninstallSuccessDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-5 w-5 text-green-600 dark:text-green-400" />
            <DialogTitle>Vellum Uninstalled Successfully</DialogTitle>
          </div>
          <DialogDescription>
            The Vellum package manager has been removed from your device.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
