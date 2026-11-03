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

interface VellumInstallSuccessDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  reinstalled?: boolean
}

export function VellumInstallSuccessDialog({
  open,
  onOpenChange,
  reinstalled,
}: VellumInstallSuccessDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader severity="success" icon={CheckCircle}>
          <DialogTitle>{reinstalled ? 'Vellum repaired' : 'Vellum installed'}</DialogTitle>
          <DialogDescription>
            {reinstalled
              ? 'The latest version of Vellum is installed. Your installed mods were kept.'
              : 'The Vellum package manager has been installed on your reMarkable. You can now install and manage mods.'}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>
            {reinstalled ? 'Done' : 'Get started'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
