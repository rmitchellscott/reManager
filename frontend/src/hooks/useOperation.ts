import { useCallback, useEffect, useState } from 'react'
import { onEvent } from '@/lib/events'
import { OperationsService } from '@bindings/reManager'
import { InstallStatus, type BackupProgress, type DialogRequest, type InstallProgress, type OperationResult, type OSInstallProgress } from '@/lib/types'
import { newOperationId, runOperation } from '@/lib/operations'

export type OperationKind =
  | 'command'
  | 'system-task'
  | 'timezone'
  | 'reenable'
  | 'restart-ui'
  | 'reboot'
  | 'switch-partition'
  | 'config'
  | 'sleep-screen'
  | 'guide'
  | 'import'
  | 'vellum'
  | 'backup'
  | 'os-install'

export interface CurrentOperation {
  id: string
  kind: OperationKind
  target: string
  title: string
  status: 'running' | 'stopping' | 'succeeded' | 'failed'
  output: string
  expectedSeconds: number
  showOutput: boolean
  canStop: boolean
  background: boolean
  reportsOwnFailure: boolean
}

export interface HookRun {
  operationId: string
  title: string
  expectedSeconds: number
  status: 'running' | 'stopping' | 'succeeded' | 'failed'
  output: string
}

export interface PendingPrompt {
  operationId: string
  request: DialogRequest
}

export interface OperationSpec {
  kind: OperationKind
  title: string
  target?: string
  expectedSeconds?: number
  showOutput?: boolean
  canStop?: boolean
  background?: boolean
  reportsOwnFailure?: boolean
  onProgress?: (progress: InstallProgress) => void
  onOutput?: (text: string) => void
  onBackupProgress?: (progress: BackupProgress) => void
  onOSInstallProgress?: (progress: OSInstallProgress) => void
  run: (id: string) => Promise<unknown>
}

export function useOperation() {
  const [operation, setOperation] = useState<CurrentOperation | null>(null)
  const [hookRun, setHookRun] = useState<HookRun | null>(null)
  const [prompt, setPrompt] = useState<PendingPrompt | null>(null)

  useEffect(() => {
    const offHook = onEvent('op:hook', hook => {
      if (!hook.finished) {
        setHookRun({ operationId: hook.id, title: hook.title, expectedSeconds: hook.expectedSeconds, status: 'running', output: '' })
        return
      }
      setHookRun(prev => prev?.operationId === hook.id
        ? { ...prev, status: hook.error ? 'failed' : 'succeeded' }
        : prev)
    })
    const offOutput = onEvent('op:output', ({ id, text, hook }) => {
      if (!hook) return
      setHookRun(prev => prev?.operationId === id ? { ...prev, output: prev.output + text } : prev)
    })
    const offPrompt = onEvent('op:prompt', ({ id, request }) => {
      setPrompt({ operationId: id, request })
    })
    return () => {
      offHook()
      offOutput()
      offPrompt()
    }
  }, [])

  useEffect(() => {
    if (prompt && hookRun?.status === 'succeeded') setHookRun(null)
  }, [prompt, hookRun])

  const start = useCallback(async (spec: OperationSpec): Promise<OperationResult> => {
    const id = newOperationId()
    setHookRun(null)
    setOperation({
      id,
      kind: spec.kind,
      target: spec.target ?? '',
      title: spec.title,
      status: 'running',
      output: '',
      expectedSeconds: spec.expectedSeconds ?? 0,
      showOutput: !!spec.showOutput,
      canStop: !!spec.canStop,
      background: !!spec.background,
      reportsOwnFailure: !!spec.reportsOwnFailure,
    })

    const append = (text: string) => {
      setOperation(prev => prev?.id === id ? { ...prev, output: prev.output + text } : prev)
    }
    const result = await runOperation(spec.run, {
      onOutput: (text, hook) => {
        if (hook) return
        spec.onOutput?.(text)
        append(text)
      },
      onProgress: progress => {
        spec.onProgress?.(progress)
        if (!progress.message) return
        append(progress.status === InstallStatus.InstallError ? `Error: ${progress.message}\n` : `${progress.message}\n`)
      },
      onBackupProgress: spec.onBackupProgress,
      onOSInstallProgress: spec.onOSInstallProgress,
    }, id)

    setOperation(prev => {
      if (prev?.id !== id) return prev
      if (result.canceled || (result.ok && prev.background) || (!result.ok && prev.reportsOwnFailure)) return null
      return { ...prev, status: result.ok ? 'succeeded' : 'failed', canStop: false }
    })
    return result
  }, [])

  const stop = () => {
    if (!operation || operation.status !== 'running') return
    setOperation({ ...operation, status: 'stopping' })
    void OperationsService.CancelOperation(operation.id)
  }

  const stopHook = () => {
    if (!hookRun || hookRun.status !== 'running') return
    setHookRun({ ...hookRun, status: 'stopping' })
    void OperationsService.CancelOperation(hookRun.operationId)
  }

  const abandon = () => {
    setOperation(null)
    setHookRun(null)
  }

  const respond = (response: string) => {
    if (!prompt) return
    setPrompt(null)
    void OperationsService.RespondToOperation(prompt.operationId, response)
  }

  const close = () => {
    setOperation(prev => prev && (prev.status === 'running' || prev.status === 'stopping') ? prev : null)
    setHookRun(prev => prev && (prev.status === 'running' || prev.status === 'stopping') ? prev : null)
  }

  const reset = () => {
    setOperation(null)
    setHookRun(null)
    setPrompt(null)
  }

  const restorePendingPrompt = async () => {
    const pending = await OperationsService.PendingOperationPrompt().catch(() => null)
    if (pending) setPrompt({ operationId: pending.id, request: pending.request })
  }

  const busy = operation?.status === 'running' || operation?.status === 'stopping'

  return { operation, hookRun, prompt, busy, start, stop, stopHook, abandon, respond, close, reset, restorePendingPrompt }
}
