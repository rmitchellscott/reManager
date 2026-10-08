import { X } from '@phosphor-icons/react'

import * as React from "react"
import * as SheetPrimitive from "@radix-ui/react-dialog"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"
import { useSwipeToDismiss, type SwipeDirection } from "@/hooks/useSwipeToDismiss"
import { useBackHandler } from "@/hooks/useBackHandler"

function Sheet({ open, onOpenChange, ...props }: React.ComponentPropsWithoutRef<typeof SheetPrimitive.Root>) {
  useBackHandler(() => onOpenChange?.(false), !!open)
  return <SheetPrimitive.Root open={open} onOpenChange={onOpenChange} {...props} />
}

const SheetTrigger = SheetPrimitive.Trigger

const SheetClose = SheetPrimitive.Close

const SheetPortal = SheetPrimitive.Portal

const SheetOverlay = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Overlay
    className={cn(
      "fixed inset-0 z-[70] bg-black/50 backdrop-blur-[1px] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
      className
    )}
    {...props}
    ref={ref}
  />
))
SheetOverlay.displayName = SheetPrimitive.Overlay.displayName

// The iOS webview runs full bleed, so an edge-anchored sheet must clear the status bar and home indicator itself; p-6 alone puts content under both.
const safeTop = 'max-md:pt-[calc(var(--safe-area-top)+1.5rem)]'
const safeBottom = 'max-md:pb-[calc(var(--safe-area-bottom)+1.5rem)]'

const sheetVariants = cva(
  "fixed z-[70] gap-4 bg-background p-6 shadow-lg transition ease-in-out data-[state=closed]:duration-300 data-[state=open]:duration-500 data-[state=open]:animate-in data-[state=closed]:animate-out",
  {
    variants: {
      side: {
        top: `inset-x-0 top-0 border-b data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top ${safeTop}`,
        bottom:
          `inset-x-0 bottom-0 border-t data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom ${safeBottom}`,
        left: `inset-y-0 left-0 h-full w-3/4 border-r data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left sm:max-w-sm ${safeTop} ${safeBottom}`,
        right:
          `inset-y-0 right-0 h-full w-3/4 border-l data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right sm:max-w-sm ${safeTop} ${safeBottom}`,
      },
    },
    defaultVariants: {
      side: "right",
    },
  }
)

interface SheetContentProps
  extends React.ComponentPropsWithoutRef<typeof SheetPrimitive.Content>,
    VariantProps<typeof sheetVariants> {
  hideClose?: boolean
  overlayClassName?: string
  scrimRef?: React.MutableRefObject<HTMLElement | null>
}

const dismissDirection: Record<NonNullable<SheetContentProps["side"]>, SwipeDirection> = {
  top: "up",
  bottom: "down",
  left: "left",
  right: "right",
}

const SheetContent = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Content>,
  SheetContentProps
>(({ side = "right", className, children, hideClose, overlayClassName, scrimRef, ...props }, ref) => {
  const closeRef = React.useRef<HTMLButtonElement>(null)
  const { ref: swipeRef, scrimRef: overlayRef, swipeHandlers, touchAction } = useSwipeToDismiss({
    direction: dismissDirection[side ?? "right"],
    onDismiss: () => closeRef.current?.click(),
    scrimRef,
  })

  const setRefs = React.useCallback(
    (node: HTMLDivElement | null) => {
      swipeRef.current = node
      if (typeof ref === "function") ref(node)
      else if (ref) ref.current = node
    },
    [ref, swipeRef]
  )

  return (
    <SheetPortal>
      <SheetOverlay
        ref={scrimRef ? undefined : (overlayRef as React.Ref<HTMLDivElement>)}
        className={overlayClassName}
      />
      <SheetPrimitive.Content
        ref={setRefs}
        className={cn(sheetVariants({ side }), "outline-none", touchAction, className)}
        onOpenAutoFocus={(e) => {
          e.preventDefault()
          swipeRef.current?.focus({ preventScroll: true })
        }}
        {...swipeHandlers}
        {...props}
      >
        <SheetPrimitive.Close
          ref={closeRef}
          aria-hidden={hideClose || undefined}
          tabIndex={hideClose ? -1 : undefined}
          className={
            hideClose
              ? "sr-only"
              : "absolute right-4 top-4 max-md:top-[calc(var(--safe-area-top)+1rem)] rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:pointer-events-none data-[state=open]:bg-secondary"
          }
        >
          <X className="h-4 w-4" />
          <span className="sr-only">Close</span>
        </SheetPrimitive.Close>
        {children}
      </SheetPrimitive.Content>
    </SheetPortal>
  )
})
SheetContent.displayName = SheetPrimitive.Content.displayName

const SheetHeader = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      "flex flex-col space-y-2 text-center sm:text-left",
      className
    )}
    {...props}
  />
)
SheetHeader.displayName = "SheetHeader"

const SheetFooter = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      "flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2",
      className
    )}
    {...props}
  />
)
SheetFooter.displayName = "SheetFooter"

const SheetTitle = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Title>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Title
    ref={ref}
    className={cn("text-lg font-semibold text-foreground", className)}
    {...props}
  />
))
SheetTitle.displayName = SheetPrimitive.Title.displayName

const SheetDescription = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Description>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Description
    ref={ref}
    className={cn("text-sm text-muted-foreground", className)}
    {...props}
  />
))
SheetDescription.displayName = SheetPrimitive.Description.displayName

export {
  Sheet,
  SheetPortal,
  SheetOverlay,
  SheetTrigger,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
}
