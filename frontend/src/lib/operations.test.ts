import { afterEach, describe, expect, it, vi } from 'vitest'
import { operationFailure, runOperation } from '@/lib/operations'
import { emit, listenerCount, resetEvents } from '@/test/wailsEvents'
import {
  BackupPhase,
  ErrorCode,
  InstallStatus,
  OSInstallStage,
  type InstallProgress,
  type OperationResult,
} from '@/lib/types'

vi.mock('@wailsio/runtime', async () => ({ Events: (await import('@/test/wailsEvents')).fakeEvents }))

afterEach(resetEvents)

const done = (id: string, overrides: Partial<OperationResult> = {}): OperationResult =>
  ({ id, ok: true, canceled: false, ...overrides })

const installProgress: InstallProgress = { component: 'xovi', index: 1, total: 2, status: InstallStatus.InstallInstalling, message: 'Installing xovi' }

async function settled(promise: Promise<unknown>): Promise<boolean> {
  let isSettled = false
  void promise.then(() => { isSettled = true })
  await new Promise(resolve => setTimeout(resolve, 0))
  return isSettled
}

describe('runOperation', () => {
  it('passes its id to the backend and resolves with the op:done for that id', async () => {
    const start = vi.fn(async () => {})
    const run = runOperation(start, {}, 'op-1')

    expect(start).toHaveBeenCalledWith('op-1')
    emit('op:done', done('op-1', { value: 'answer' }))

    await expect(run).resolves.toEqual(done('op-1', { value: 'answer' }))
  })

  it('ignores an op:done for another id', async () => {
    const run = runOperation(async () => {}, {}, 'op-1')

    emit('op:done', done('earlier-run'))
    expect(await settled(run)).toBe(false)

    emit('op:done', done('op-1', { ok: false, error: 'failed' }))
    await expect(run).resolves.toMatchObject({ id: 'op-1', ok: false, error: 'failed' })
  })

  it('listens before the backend is called, so an event sent during the call is not missed', async () => {
    const onOutput = vi.fn()
    const run = runOperation(async id => {
      emit('op:output', { id, text: 'first line\n', hook: false })
      emit('op:done', done(id))
    }, { onOutput }, 'op-1')

    await expect(run).resolves.toEqual(done('op-1'))
    expect(onOutput).toHaveBeenCalledWith('first line\n', false)
  })

  it('stops listening once the run is done', async () => {
    const run = runOperation(async () => {}, {}, 'op-1')
    expect(listenerCount()).toBeGreaterThan(0)

    emit('op:done', done('op-1'))
    await run

    expect(listenerCount()).toBe(0)
  })

  it('routes output, progress and hooks for its own id only', async () => {
    const handlers = {
      onOutput: vi.fn(),
      onProgress: vi.fn(),
      onBackupProgress: vi.fn(),
      onOSInstallProgress: vi.fn(),
      onHook: vi.fn(),
    }
    const run = runOperation(async () => {}, handlers, 'op-1')
    const backup = { phase: BackupPhase.PhaseDownloading, filePath: '', filesDone: 1, filesTotal: 4, bytesDone: 10, bytesTotal: 40, percentage: 25, message: '' }
    const osInstall = { stage: OSInstallStage.OSInstallUploading, percent: 50, message: '' }
    const hook = { id: 'op-1', title: 'Post-install', expectedSeconds: 5, finished: false, error: '' }

    for (const id of ['other', 'op-1']) {
      emit('op:output', { id, text: 'hook says hi\n', hook: true })
      emit('op:progress', { id, progress: installProgress })
      emit('op:progress', { id, progress: installProgress, backup })
      emit('op:progress', { id, progress: installProgress, osInstall })
      emit('op:hook', { ...hook, id })
    }
    emit('op:done', done('op-1'))
    await run

    expect(handlers.onOutput).toHaveBeenCalledExactlyOnceWith('hook says hi\n', true)
    expect(handlers.onProgress).toHaveBeenCalledExactlyOnceWith(installProgress)
    expect(handlers.onBackupProgress).toHaveBeenCalledExactlyOnceWith(backup)
    expect(handlers.onOSInstallProgress).toHaveBeenCalledExactlyOnceWith(osInstall)
    expect(handlers.onHook).toHaveBeenCalledExactlyOnceWith(hook)
  })

  it('resolves as a failure, and stops listening, when the backend call rejects', async () => {
    const run = runOperation(async () => { throw new Error('not connected') }, {}, 'op-1')

    await expect(run).resolves.toEqual({ id: 'op-1', ok: false, canceled: false, error: 'not connected' })
    expect(listenerCount()).toBe(0)
  })

  it('picks a fresh id for each run when none is given', () => {
    const start = vi.fn<(id: string) => Promise<void>>(async () => {})
    void runOperation(start)
    void runOperation(start)

    const [[first], [second]] = start.mock.calls
    expect(first).toBeTruthy()
    expect(first).not.toBe(second)
  })
})

describe('operationFailure', () => {
  it('rebuilds a coded error from the result', () => {
    expect(operationFailure(done('op-1', { ok: false, code: ErrorCode.ErrDiskFull, error: 'no space left on device' })))
      .toEqual({ code: ErrorCode.ErrDiskFull, message: 'no space left on device' })
  })

  it('keeps the code when the result carries no message', () => {
    expect(operationFailure(done('op-1', { ok: false, code: ErrorCode.ErrTimeout }))).toEqual({ code: ErrorCode.ErrTimeout, message: '' })
  })

  it('is the plain message when the result has no code', () => {
    expect(operationFailure(done('op-1', { ok: false, error: 'exit status 1' }))).toBe('exit status 1')
    expect(operationFailure(done('op-1', { ok: false }))).toBe('')
  })
})
