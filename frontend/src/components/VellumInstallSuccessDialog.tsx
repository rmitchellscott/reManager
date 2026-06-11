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
        <DialogHeader>
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-5 w-5 text-green-600 dark:text-green-400" />
            <DialogTitle>{reinstalled ? 'Vellum Repaired Successfully' : 'Vellum Installed Successfully'}</DialogTitle>
          </div>
          <DialogDescription>
            {reinstalled
              ? 'Vellum has been reinstalled with the latest version. Your installed mods were preserved.'
              : 'The Vellum package manager has been installed on your device. You can now install and manage mods.'}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>
            {reinstalled ? 'Done' : 'Get Started'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
