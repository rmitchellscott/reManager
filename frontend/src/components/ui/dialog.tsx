import * as React from "react"
import { cn } from "@/lib/utils"

interface DialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  closable?: boolean
  className?: string
  priority?: boolean
}

export function Dialog({ open, onOpenChange, children, closable = true, className, priority = false }: DialogProps) {
  React.useEffect(() => {
    if (open) {
      document.body.style.overflow = 'hidden'
      return () => {
        document.body.style.overflow = ''
      }
    }
  }, [open])

  if (!open) return null

  const backdropZ = priority ? 'z-[65]' : 'z-[55]'
  const contentZ = priority ? 'z-[70]' : 'z-[60]'

  return (
    <div className={cn(`fixed inset-0 ${contentZ} flex items-center justify-center`, className)}>
      <div
        className={`fixed inset-0 ${backdropZ} bg-black/50 backdrop-blur-[1px]`}
        onClick={() => closable && onOpenChange(false)}
      />
      <div className={`relative ${contentZ} w-full px-4 flex justify-center pointer-events-none`}>
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
