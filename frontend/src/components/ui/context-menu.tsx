import * as React from "react"
import * as ContextMenuPrimitive from "@radix-ui/react-context-menu"
import { cn } from "@/lib/utils"
import { useFocusDialogOpenedFromMenu } from "@/hooks/useFocusDialogOpenedFromMenu"
import { useBackHandler } from "@/hooks/useBackHandler"

function ContextMenu({ onOpenChange, ...props }: React.ComponentPropsWithoutRef<typeof ContextMenuPrimitive.Root>) {
  const [open, setOpen] = React.useState(false)
  useBackHandler(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })), open)
  return (
    <ContextMenuPrimitive.Root
      {...props}
      onOpenChange={(next) => {
        setOpen(next)
        onOpenChange?.(next)
      }}
    />
  )
}

const ContextMenuTrigger = ContextMenuPrimitive.Trigger

const ContextMenuContent = React.forwardRef<
  React.ElementRef<typeof ContextMenuPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof ContextMenuPrimitive.Content>
>(({ className, ...props }, ref) => {
  const { contentRef, onCloseAutoFocus } = useFocusDialogOpenedFromMenu(ref)
  return (
    <ContextMenuPrimitive.Portal>
      <ContextMenuPrimitive.Content
        ref={contentRef}
        onCloseAutoFocus={onCloseAutoFocus}
        className={cn(
          "z-[80] max-h-[var(--radix-context-menu-content-available-height)] min-w-[8rem] overflow-y-auto overflow-x-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md",
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 origin-[--radix-context-menu-content-transform-origin]",
          className
        )}
        {...props}
      />
    </ContextMenuPrimitive.Portal>
  )
})
ContextMenuContent.displayName = ContextMenuPrimitive.Content.displayName

const ContextMenuItem = React.forwardRef<
  React.ElementRef<typeof ContextMenuPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof ContextMenuPrimitive.Item>
>(({ className, ...props }, ref) => (
  <ContextMenuPrimitive.Item
    ref={ref}
    className={cn(
      "relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm touch:min-h-[var(--hit-target)] touch:px-3 outline-none transition-colors focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&>svg]:size-4 [&>svg]:shrink-0",
      className
    )}
    {...props}
  />
))
ContextMenuItem.displayName = ContextMenuPrimitive.Item.displayName

export {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
}
