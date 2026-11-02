import { onEvent } from '@/lib/events'
import type { BackupProgress, InstallProgress, OperationHook, OperationResult, OSInstallProgress } from '@/lib/types'

export function newOperationId(): string {
  return typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

export function operationFailure(result: OperationResult): { code: string; message: string } | string {
  const message = result.error ?? ''
  return result.code ? { code: result.code, message } : message
}

export interface OperationHandlers {
  onOutput?: (text: string, hook: boolean) => void
  onProgress?: (progress: InstallProgress) => void
  onBackupProgress?: (progress: BackupProgress) => void
  onOSInstallProgress?: (progress: OSInstallProgress) => void
  onHook?: (hook: OperationHook) => void
}

export function runOperation(
  start: (id: string) => Promise<unknown>,
  handlers: OperationHandlers = {},
  id: string = newOperationId(),
): Promise<OperationResult> {
  return new Promise(resolve => {
    const offs = [
      onEvent('op:output', output => {
        if (output.id === id) handlers.onOutput?.(output.text, output.hook)
      }),
      onEvent('op:progress', progress => {
        if (progress.id !== id) return
        if (progress.backup) handlers.onBackupProgress?.(progress.backup)
        else if (progress.osInstall) handlers.onOSInstallProgress?.(progress.osInstall)
        else handlers.onProgress?.(progress.progress)
      }),
      onEvent('op:hook', hook => {
        if (hook.id === id) handlers.onHook?.(hook)
      }),
      onEvent('op:done', result => {
        if (result.id !== id) return
        offs.forEach(off => off())
        resolve(result)
      }),
    ]
    start(id).catch(err => {
      offs.forEach(off => off())
      resolve({ id, ok: false, canceled: false, error: err instanceof Error ? err.message : String(err) })
    })
  })
}
