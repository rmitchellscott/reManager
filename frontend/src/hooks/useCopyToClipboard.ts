import { useState, useCallback } from 'react'

interface UseCopyToClipboardReturn {
  isCopied: boolean
  copyToClipboard: (text: string) => Promise<void>
  error: Error | null
}

export function useCopyToClipboard(timeout = 2000): UseCopyToClipboardReturn {
  const [isCopied, setIsCopied] = useState(false)
  const [error, setError] = useState<Error | null>(null)

  const copyToClipboard = useCallback(async (text: string) => {
    if (!navigator?.clipboard) {
      const err = new Error('Clipboard API not available')
      setError(err)
      console.error('Clipboard API not available')
      return
    }

    try {
      await navigator.clipboard.writeText(text)
      setIsCopied(true)
      setError(null)
      setTimeout(() => setIsCopied(false), timeout)
    } catch (err) {
      const error = err as Error
      setError(error)
      console.error('Failed to copy to clipboard:', error)
    }
  }, [timeout])

  return { isCopied, copyToClipboard, error }
}
