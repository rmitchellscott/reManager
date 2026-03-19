import * as React from "react"
import * as DialogPrimitive from "@radix-ui/react-dialog"
import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  closable?: boolean
  className?: string
}

export function Dialog({ open, onOpenChange, children, closable = true, className }: DialogProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={closable ? onOpenChange : undefined}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 bg-black/50 backdrop-blur-[1px] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
          onClick={(e) => {
            if (closable) {
              e.stopPropagation()
              onOpenChange(false)
            }
          }}
        />
        <DialogPrimitive.Content
          className={cn("fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 w-full px-4 flex justify-center pointer-events-none", className)}
          onInteractOutside={(e) => { if (!closable) e.preventDefault() }}
          onEscapeKeyDown={(e) => { if (!closable) e.preventDefault() }}
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          <div className="pointer-events-auto">{children}</div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

export function DialogContent({
  className,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "bg-card border rounded-lg shadow-lg p-6",
        "max-w-lg",
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export function DialogHeader({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex flex-col space-y-1.5 mb-4", className)}
      {...props}
    />
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
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex justify-end gap-2 mt-6", className)}
      {...props}
    />
  )
}
