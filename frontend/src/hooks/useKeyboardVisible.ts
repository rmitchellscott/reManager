import { useEffect, useState } from 'react'

export function useKeyboardVisible() {
  const [keyboardVisible, setKeyboardVisible] = useState(false)

  useEffect(() => {
    const isEditable = (element: Element | null) =>
      element instanceof HTMLElement &&
      element.isConnected &&
      (element.tagName === 'INPUT' ||
        element.tagName === 'TEXTAREA' ||
        element.isContentEditable)

    const sync = () => setKeyboardVisible(isEditable(document.activeElement))

    document.addEventListener('focusin', sync)
    document.addEventListener('focusout', sync)
    return () => {
      document.removeEventListener('focusin', sync)
      document.removeEventListener('focusout', sync)
    }
  }, [])

  return keyboardVisible
}
