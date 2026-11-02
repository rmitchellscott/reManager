import { CheckCircle } from '@phosphor-icons/react'
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
        <DialogHeader severity="success" icon={CheckCircle}>
          <DialogTitle>Vellum uninstalled</DialogTitle>
          <DialogDescription>
            The Vellum package manager has been removed from your reMarkable.
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
