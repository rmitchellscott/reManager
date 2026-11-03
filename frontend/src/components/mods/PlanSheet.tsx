import type { ReactNode } from 'react'
import { X } from '@phosphor-icons/react'
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { useMediaQuery, NARROW_VIEWPORT } from '@/hooks/useMediaQuery'

export function PlanSheet({ open, onOpenChange, title, children }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  children: ReactNode
}) {
  const narrow = useMediaQuery(NARROW_VIEWPORT)
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={narrow ? 'bottom' : 'right'}
        hideClose
        className={narrow
          ? 'flex h-[calc(100dvh-var(--safe-area-top)-8px)] flex-col gap-0 rounded-t-2xl p-0 max-md:pb-0 touch-auto'
          : 'flex w-full flex-col gap-0 p-0 sm:max-w-lg'}
      >
        <div className={narrow ? 'shrink-0 pt-2 touch-none' : 'shrink-0 pt-3 border-b'}>
          {narrow && <div aria-hidden className="mx-auto h-1.5 w-9 rounded-full bg-border" />}
          <div className="flex items-center gap-2 py-2 pl-gutter-left pr-[max(8px,var(--safe-area-right))]">
            <SheetTitle className="flex flex-1 items-center text-base">{title}</SheetTitle>
            <SheetDescription className="sr-only">Changes to your mods, waiting to be applied</SheetDescription>
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              aria-label="Close"
              className="grid h-9 w-9 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground touch:hit-target"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
        <div
          className="flex min-h-0 flex-1 flex-col pl-gutter-left pr-gutter-right pt-2 pb-[var(--safe-area-bottom)]"
          onPointerDown={(e) => e.stopPropagation()}
        >
          {children}
        </div>
      </SheetContent>
    </Sheet>
  )
}
