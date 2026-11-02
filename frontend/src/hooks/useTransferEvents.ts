import { useState, useEffect } from 'react'
import type { FileBrowserError, TransferFinished, TransferSnapshot } from '@/lib/types'
import { FilesService } from '@bindings/reManager'
import { onEvent } from '@/lib/events'

export function useTransferEvents() {
  const [transfers, setTransfers] = useState<TransferSnapshot | null>(null)
  const [lastFinished, setLastFinished] = useState<TransferFinished | null>(null)
  const [transferError, setTransferError] = useState<FileBrowserError | null>(null)

  useEffect(() => {

    FilesService.GetTransfers().then(setTransfers).catch(() => {})

    const unsubscribeSnapshot = onEvent('transfers:snapshot', setTransfers)
    const unsubscribeFinished = onEvent('transfers:finished', setLastFinished)
    const unsubscribeError = onEvent('filebrowser:error', setTransferError)

    return () => {
      unsubscribeSnapshot()
      unsubscribeFinished()
      unsubscribeError()
    }
  }, [])

  return {
    transfers,
    lastFinished,
    transferError,
    setTransferError,
  }
}
