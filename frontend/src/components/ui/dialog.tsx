import { X } from '@phosphor-icons/react'
import * as React from "react"
import * as DialogPrimitive from "@radix-ui/react-dialog"
import { useBackHandler } from '@/hooks/useBackHandler'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  closable?: boolean
  className?: string
}

export function Dialog({ open, onOpenChange, children, closable = true, className }: DialogProps) {
  // Registered even when not closable, so Back cannot dismiss what is underneath.
  useBackHandler(() => { if (closable) onOpenChange(false) }, open)
  const contentRef = React.useRef<HTMLDivElement>(null)

  return (
    <DialogPrimitive.Root open={open} onOpenChange={closable ? onOpenChange : undefined}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-[70] bg-black/50 backdrop-blur-[1px] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
          onClick={(e) => {
            if (closable) {
              e.stopPropagation()
              onOpenChange(false)
            }
          }}
        />
        <DialogPrimitive.Content
          ref={contentRef}
          role={closable ? 'dialog' : 'alertdialog'}
          className={cn(
            // pointer-events-none so the band beside a narrow panel still belongs
            // to the overlay, which is what closes the dialog.
            "fixed left-[calc(50%+(var(--safe-area-left)-var(--safe-area-right))/2)] top-1/2 z-[70] -translate-x-1/2 -translate-y-1/2 pointer-events-none outline-none",
            "flex w-[calc(100vw-2rem-var(--safe-area-left)-var(--safe-area-right))] justify-center",
            className,
          )}
          onInteractOutside={(e) => { if (!closable) e.preventDefault() }}
          onEscapeKeyDown={(e) => { if (!closable) e.preventDefault() }}
          // Focus lands on the dialog, not its first control, so a phone's keyboard stays down.
          onOpenAutoFocus={(e) => {
            e.preventDefault()
            contentRef.current?.focus({ preventScroll: true })
          }}
        >
          {children}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

const CONTENT_SIZE = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  default: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-3xl',
  '2xl': 'max-w-4xl',
  '3xl': 'max-w-5xl',
} as const

export function DialogContent({
  className,
  children,
  size = 'default',
  onSwipeDismiss,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & { size?: keyof typeof CONTENT_SIZE; onSwipeDismiss?: () => void }) {
  const swipeable = useMediaQuery('(max-width: 639px)')
  const { ref: swipeRef, swipeHandlers, touchAction } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: () => onSwipeDismiss?.(),
    enabled: !!onSwipeDismiss && swipeable,
  })

  return (
    <div
      ref={swipeRef}
      {...swipeHandlers}
      className={cn(
        touchAction,
        "relative bg-card border rounded-lg shadow-lg p-6 pointer-events-auto",
        "flex flex-col gap-5",
        // dvh, not vh: mobile browser chrome is excluded from dvh, so a tall dialog does not run under the address bar.
        "w-full max-h-[calc(100dvh-2rem-2*max(var(--safe-area-top),var(--safe-area-bottom)))]",
        CONTENT_SIZE[size],
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export function DialogCloseButton({
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      aria-label="Close"
      className={cn(
        "absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
        className,
      )}
      {...props}
    >
      {/* On the glyph, not the button: hit-target sets position: relative, which would override absolute on touch. */}
      <span className="block touch:hit-target">
        <X className="h-4 w-4" />
      </span>
    </button>
  )
}

export function DialogBody({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex-1 min-h-0 overflow-y-auto overscroll-contain -mx-6 px-6", className)}
      {...props}
    />
  )
}

const SEVERITY_BADGE = {
  destructive: 'bg-destructive/10 text-destructive ring-destructive/20',
  warning: 'bg-amber-500/10 text-amber-600 dark:text-amber-400 ring-amber-500/20',
  info: 'bg-blue-500/10 text-blue-600 dark:text-blue-400 ring-blue-500/20',
  success: 'bg-[var(--success)]/10 text-[var(--success)] ring-[var(--success)]/20',
  neutral: 'bg-muted text-muted-foreground ring-border',
} as const

export function DialogHeader({
  className,
  severity,
  icon: Icon,
  iconClassName,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & {
  severity?: keyof typeof SEVERITY_BADGE
  icon?: React.ComponentType<{ className?: string }>
  iconClassName?: string
}) {
  if (severity && Icon) {
    return (
      <div className={cn("flex items-center gap-4", className)} {...props}>
        <div className={cn(
          'flex h-9 w-9 shrink-0 items-center justify-center rounded-md ring-1 ring-inset',
          SEVERITY_BADGE[severity],
        )}>
          <Icon className={cn("h-5 w-5", iconClassName)} />
        </div>
        <div className="min-w-0 flex flex-col space-y-1.5">{children}</div>
      </div>
    )
  }
  return (
    <div className={cn("flex flex-col space-y-1.5", className)} {...props}>{children}</div>
  )
}

export function DialogTitle({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <DialogPrimitive.Title asChild>
      <h2
        className={cn("text-lg font-semibold leading-none tracking-tight", className)}
        {...props}
      />
    </DialogPrimitive.Title>
  )
}

export function DialogDescription({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <DialogPrimitive.Description asChild>
      <div
        className={cn("text-sm text-muted-foreground", className)}
        {...props}
      />
    </DialogPrimitive.Description>
  )
}

export function DialogFooter({
  className,
  separateExit,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & {
  separateExit?: boolean
}) {
  return (
    <div
      className={cn(
        // col-reverse puts the primary action on top while DOM order stays cancel-then-confirm for tab order.
        "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
        "[&>button]:w-full sm:[&>button]:w-auto",
        separateExit && "sm:[&>button:first-child]:mr-auto max-sm:[&>button:first-child]:border-transparent max-sm:[&>button:first-child]:bg-transparent",
        className,
      )}
      {...props}
    />
  )
}
