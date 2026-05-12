import { useState, useCallback } from 'react'

interface UseCopyToClipboardReturn {
  isCopied: boolean
  copyToClipboard: (text: string) => Promise<void>
}

export function useCopyToClipboard(timeout = 2000): UseCopyToClipboardReturn {
  const [isCopied, setIsCopied] = useState(false)

  const copyToClipboard = useCallback(async (text: string) => {
    if (!navigator?.clipboard) {
      console.error('Clipboard API not available')
      return
    }

    try {
      await navigator.clipboard.writeText(text)
      setIsCopied(true)
      setTimeout(() => setIsCopied(false), timeout)
    } catch (err) {
      console.error('Failed to copy to clipboard:', err)
    }
  }, [timeout])

  return { isCopied, copyToClipboard }
}
