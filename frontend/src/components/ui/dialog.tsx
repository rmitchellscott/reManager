import * as React from "react"
import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  closable?: boolean
  className?: string
}

export function Dialog({ open, onOpenChange, children, closable = true, className }: DialogProps) {
  React.useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden'
      return () => {
        document.body.style.overflow = ''
      }
    }
  }, [open])

  if (!open) return null

  return (
    <div className={cn("fixed inset-0 z-50 flex items-center justify-center", className)}>
      <div
        className="fixed inset-0 z-40 bg-black/50 backdrop-blur-[1px]"
        onClick={() => closable && onOpenChange(false)}
      />
      <div className="relative z-50 w-full px-4 flex justify-center pointer-events-none">
        <div className="pointer-events-auto">{children}</div>
      </div>
    </div>
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
    <h2
      className={cn("text-lg font-semibold leading-none tracking-tight", className)}
      {...props}
    />
  )
}

export function DialogDescription({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
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
