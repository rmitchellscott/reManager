import { useState, useCallback } from 'react'
import { Clipboard } from '@wailsio/runtime'

interface UseCopyToClipboardReturn {
  isCopied: boolean
  copyToClipboard: (text: string) => Promise<void>
}

export function useCopyToClipboard(timeout = 2000): UseCopyToClipboardReturn {
  const [isCopied, setIsCopied] = useState(false)

  const copyToClipboard = useCallback(async (text: string) => {
    const attempts: (() => Promise<unknown>)[] = []
    if (navigator?.clipboard) {
      attempts.push(() => navigator.clipboard.writeText(text))
    }
    attempts.push(() => Clipboard.SetText(text))

    for (const attempt of attempts) {
      try {
        await attempt()
        setIsCopied(true)
        setTimeout(() => setIsCopied(false), timeout)
        return
      } catch {
      }
    }
    console.error('Failed to copy to clipboard')
  }, [timeout])

  return { isCopied, copyToClipboard }
}
