import { useState, useEffect, useRef, useCallback } from 'react'
import { debugLog } from '@/lib/utils'
import { handleError, getUserFriendlyMessage } from '@/lib/errorMessages'

interface UseConnectionEventsParams {
  onConnectionRestored: (deviceType: string) => Promise<void>
  onConnectionLost: () => void
  onUnsavedConnectionFailed: () => void
  deviceRef: React.RefObject<string>
}

export function useConnectionEvents({
  onConnectionRestored,
  onConnectionLost,
  onUnsavedConnectionFailed,
  deviceRef,
}: UseConnectionEventsParams) {
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'lost' | 'reconnecting' | 'failed'>('connected')
  const [reconnectAttempt, setReconnectAttempt] = useState(0)
  const [reconnectMaxAttempts, setReconnectMaxAttempts] = useState(0)
  const [connectionError, setConnectionError] = useState<string | null>(null)
  const [showFilesystemRestoreError, setShowFilesystemRestoreError] = useState(false)
  const [isRetryingFilesystemRestore, setIsRetryingFilesystemRestore] = useState(false)
  const [writeableRootBusy, setWriteableRootBusy] = useState(false)

  const onConnectionRestoredRef = useRef(onConnectionRestored)
  onConnectionRestoredRef.current = onConnectionRestored

  const onConnectionLostRef = useRef(onConnectionLost)
  onConnectionLostRef.current = onConnectionLost

  const onUnsavedConnectionFailedRef = useRef(onUnsavedConnectionFailed)
  onUnsavedConnectionFailedRef.current = onUnsavedConnectionFailed

  useEffect(() => {
    if (typeof window.runtime === 'undefined') return

    const unsubscribeConnectionLost = window.runtime.EventsOn('connection:lost', (...args: unknown[]) => {
      const data = args[0] as { reason: string; code?: string; deviceId: string }
      debugLog('Received connection:lost:', data)
      onConnectionLostRef.current()
      setConnectionStatus('lost')
      setConnectionError(data.code ? getUserFriendlyMessage(data) : handleError(data.reason, 'Connection'))
    })

    const unsubscribeConnectionReconnecting = window.runtime.EventsOn('connection:reconnecting', (...args: unknown[]) => {
      const data = args[0] as { attempt: number; maxAttempts: number; deviceId: string }
      debugLog('Received connection:reconnecting:', data)
      setConnectionStatus('reconnecting')
      setReconnectAttempt(data.attempt)
      setReconnectMaxAttempts(data.maxAttempts)
    })

    const unsubscribeConnectionRestored = window.runtime.EventsOn('connection:restored', async (...args: unknown[]) => {
      const data = args[0] as { deviceId: string; device: string }
      debugLog('Received connection:restored:', data)
      onConnectionLostRef.current()
      setConnectionStatus('connected')

      const deviceType = data.device || deviceRef.current || ''
      try {
        await onConnectionRestoredRef.current(deviceType)
      } catch (err) {
        debugLog('Failed to refresh device info on reconnect:', err)
      }
    })

    const unsubscribeConnectionFailed = window.runtime.EventsOn('connection:failed', (...args: unknown[]) => {
      const data = args[0] as { reason: string; code?: string; deviceId: string }
      debugLog('Received connection:failed:', data)
      setConnectionStatus('failed')
      setConnectionError(data.code ? getUserFriendlyMessage(data) : handleError(data.reason, 'Connection'))
      if (!data.deviceId) {
        onUnsavedConnectionFailedRef.current()
      }
    })

    const unsubscribeFilesystemRestoreError = window.runtime.EventsOn('filesystem:restore-error', (...args: unknown[]) => {
      const data = args[0] as { message: string }
      debugLog('Received filesystem:restore-error:', data)
      setShowFilesystemRestoreError(true)
    })

    const unsubscribeWriteableRootBusy = window.runtime.EventsOn('writeable-root:busy', (...args: unknown[]) => {
      setWriteableRootBusy(args[0] as boolean)
    })

    return () => {
      unsubscribeConnectionLost()
      unsubscribeConnectionReconnecting()
      unsubscribeConnectionRestored()
      unsubscribeConnectionFailed()
      unsubscribeFilesystemRestoreError()
      unsubscribeWriteableRootBusy()
    }
  }, [])

  const handleRetryRestore = useCallback(async () => {
    setIsRetryingFilesystemRestore(true)
    try {
      await window.go.main.App.RetryRestoreFilesystem()
      setShowFilesystemRestoreError(false)
    } catch (err) {
      debugLog('Filesystem restore retry failed:', err)
    }
    setIsRetryingFilesystemRestore(false)
  }, [])

  const handleRebootDevice = useCallback(async () => {
    await window.go.main.App.RebootDevice()
    setShowFilesystemRestoreError(false)
  }, [])

  const handleDismissRestoreError = useCallback(() => {
    setShowFilesystemRestoreError(false)
  }, [])

  return {
    connectionStatus,
    setConnectionStatus,
    reconnectAttempt,
    setReconnectAttempt,
    reconnectMaxAttempts,
    connectionError,
    setConnectionError,
    showFilesystemRestoreError,
    isRetryingFilesystemRestore,
    writeableRootBusy,
    setWriteableRootBusy,
    handleRetryRestore,
    handleRebootDevice,
    handleDismissRestoreError,
  }
}
