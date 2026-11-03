import { useState, useEffect, useRef, useCallback } from 'react'
import { debugLog } from '@/lib/utils'
import { errorDetail, handleError } from '@/lib/errorMessages'
import type { RetryPhase } from '@/components/RetryCountdown'
import { OSService } from '@bindings/reManager'
import { onEvent } from '@/lib/events'
import type { ErrorCode } from '@/lib/types'

export type ConnectionStatus = 'connected' | 'lost' | 'reconnecting' | 'restoring' | 'failed'

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
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('connected')
  const [reconnectAttempt, setReconnectAttempt] = useState(0)
  const [reconnectMaxAttempts, setReconnectMaxAttempts] = useState(0)
  const [retryPhase, setRetryPhase] = useState<RetryPhase | null>(null)
  const [connectionErrorCode, setConnectionErrorCode] = useState<ErrorCode | null>(null)
  const [connectionError, setConnectionError] = useState<string | null>(null)
  const [connectionErrorDetails, setConnectionErrorDetails] = useState('')
  const reportConnectionFailure = useCallback((failure: unknown) => {
    if (failure === null) {
      setConnectionError(null)
      setConnectionErrorDetails('')
      return
    }
    setConnectionError(handleError(failure, 'Could not connect to your reMarkable.'))
    setConnectionErrorDetails(errorDetail(failure))
  }, [])
  const [showFilesystemRestoreError, setShowFilesystemRestoreError] = useState(false)
  const [isRetryingFilesystemRestore, setIsRetryingFilesystemRestore] = useState(false)
  const [writableRootBusy, setWritableRootBusy] = useState(false)

  const onConnectionRestoredRef = useRef(onConnectionRestored)
  onConnectionRestoredRef.current = onConnectionRestored

  const onConnectionLostRef = useRef(onConnectionLost)
  onConnectionLostRef.current = onConnectionLost

  const onUnsavedConnectionFailedRef = useRef(onUnsavedConnectionFailed)
  onUnsavedConnectionFailedRef.current = onUnsavedConnectionFailed

  const connectionStatusRef = useRef(connectionStatus)
  connectionStatusRef.current = connectionStatus
  const restoreSession = useCallback(async (deviceType: string) => {
    setConnectionStatus('restoring')
    connectionStatusRef.current = 'restoring'
    try {
      await onConnectionRestoredRef.current(deviceType)
      if (connectionStatusRef.current !== 'restoring') return
      setConnectionStatus('connected')
    } catch (err) {
      debugLog('Failed to reload from the reMarkable after reconnecting:', err)
      if (connectionStatusRef.current !== 'restoring') return
      setConnectionStatus('failed')
      setConnectionErrorCode(null)
      reportConnectionFailure(err)
    }
  }, [reportConnectionFailure])

  useEffect(() => {

    const unsubscribeConnectionLost = onEvent('connection:lost', data => {
      debugLog('Received connection:lost:', data)
      onConnectionLostRef.current()
      setConnectionStatus('lost')
      reportConnectionFailure(data.code ? data : data.reason)
    })

    const unsubscribeConnectionReconnecting = onEvent('connection:reconnecting', data => {
      debugLog('Received connection:reconnecting:', data)
      setConnectionStatus('reconnecting')
      setReconnectAttempt(data.attempt)
      setReconnectMaxAttempts(data.maxAttempts)
      setRetryPhase({ kind: 'dialing', endsAt: Date.now() + data.durationMs, durationMs: data.durationMs, elapsedMs: data.elapsedMs, totalMs: data.totalMs })
    })

    const unsubscribeRetryScheduled = onEvent('connection:retry-scheduled', data => {
      debugLog('Received connection:retry-scheduled:', data)
      setReconnectAttempt(data.attempt)
      setReconnectMaxAttempts(data.maxAttempts)
      setRetryPhase({ kind: 'waiting', endsAt: Date.now() + data.durationMs, durationMs: data.durationMs, elapsedMs: data.elapsedMs, totalMs: data.totalMs })
    })

    const unsubscribeConnectionRestored = onEvent('connection:restored', async data => {
      debugLog('Received connection:restored:', data)
      await restoreSession(data.device || deviceRef.current || '')
    })

    const unsubscribeConnectionFailed = onEvent('connection:failed', data => {
      debugLog('Received connection:failed:', data)
      setConnectionStatus('failed')
      setConnectionErrorCode(data.code ?? null)
      reportConnectionFailure(data.code ? data : data.reason)
      if (!data.deviceId) {
        onUnsavedConnectionFailedRef.current()
      }
    })

    const unsubscribeFilesystemRestoreError = onEvent('filesystem:restore-error', data => {
      debugLog('Received filesystem:restore-error:', data)
      setShowFilesystemRestoreError(true)
    })

    const unsubscribeWritableRootBusy = onEvent('writable-root:busy', setWritableRootBusy)

    return () => {
      unsubscribeConnectionLost()
      unsubscribeConnectionReconnecting()
      unsubscribeRetryScheduled()
      unsubscribeConnectionRestored()
      unsubscribeConnectionFailed()
      unsubscribeFilesystemRestoreError()
      unsubscribeWritableRootBusy()
    }
  }, [])

  const handleRetryRestore = useCallback(async () => {
    setIsRetryingFilesystemRestore(true)
    try {
      await OSService.RetryRestoreFilesystem()
      setShowFilesystemRestoreError(false)
    } catch (err) {
      debugLog('Filesystem restore retry failed:', err)
    }
    setIsRetryingFilesystemRestore(false)
  }, [])

  const handleDismissRestoreError = useCallback(() => {
    setShowFilesystemRestoreError(false)
  }, [])

  return {
    connectionStatus,
    setConnectionStatus,
    restoreSession,
    reconnectAttempt,
    setReconnectAttempt,
    reconnectMaxAttempts,
    retryPhase,
    connectionErrorCode,
    connectionError,
    connectionErrorDetails,
    reportConnectionFailure,
    showFilesystemRestoreError,
    isRetryingFilesystemRestore,
    writableRootBusy,
    setWritableRootBusy,
    handleRetryRestore,
    handleDismissRestoreError,
  }
}
