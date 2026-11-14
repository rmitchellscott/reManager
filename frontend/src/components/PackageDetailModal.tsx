import { ReactNode } from 'react'
import { Dialog, DialogCloseButton, DialogContent } from '@/components/ui/dialog'

interface PackageDetailModalProps {
  open: boolean
  onClose: () => void
  onBack?: () => void
  children: ReactNode
}

export function PackageDetailModal({ open, onClose, onBack, children }: PackageDetailModalProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose() }} className="max-sm:w-screen max-sm:left-1/2">
      <DialogContent
        size="xl"
        onSwipeDismiss={onBack ? undefined : onClose}
        className="max-sm:w-screen max-sm:h-[100dvh] max-sm:max-h-none max-sm:rounded-none max-sm:border-0 max-sm:pt-[calc(var(--safe-area-top)+1rem)] max-sm:pb-[var(--safe-area-bottom)] max-sm:pl-[max(1.5rem,var(--safe-area-left))] max-sm:pr-[max(1.5rem,var(--safe-area-right))] sm:h-[82dvh]"
      >
        {children}
        <DialogCloseButton onClick={onClose} className="max-sm:top-[calc(var(--safe-area-top)+1rem)] max-sm:right-[max(1rem,var(--safe-area-right))]" />
      </DialogContent>
    </Dialog>
  )
}
