import { useCallback, useRef, type ForwardedRef } from 'react'

const DIALOG = '[role="dialog"], [role="alertdialog"]'
const TEXT_FIELD = 'input:not([type="hidden"]):not(:disabled), textarea:not(:disabled)'

// A Radix menu hands focus to its trigger after its exit animation, overriding the opened dialog's field autoFocus, so the field is restored on the menu's close.
export function useFocusDialogOpenedFromMenu<T extends HTMLElement>(forwardedRef: ForwardedRef<T>) {
  const dialogsOpenBeforeMenu = useRef<Set<Element> | null>(null)

  const contentRef = useCallback((node: T | null) => {
    if (node && !dialogsOpenBeforeMenu.current) {
      dialogsOpenBeforeMenu.current = new Set(document.querySelectorAll(DIALOG))
    }
    if (typeof forwardedRef === 'function') forwardedRef(node)
    else if (forwardedRef) forwardedRef.current = node
  }, [forwardedRef])

  const onCloseAutoFocus = useCallback((e: Event) => {
    const dialogsOpenBefore = dialogsOpenBeforeMenu.current ?? new Set()
    dialogsOpenBeforeMenu.current = null
    const openedDialog = [...document.querySelectorAll(DIALOG)].find(dialog => !dialogsOpenBefore.has(dialog))
    if (!openedDialog) return
    e.preventDefault()
    openedDialog.querySelector<HTMLElement>(TEXT_FIELD)?.focus()
  }, [])

  return { contentRef, onCloseAutoFocus }
}
