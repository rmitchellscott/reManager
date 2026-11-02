// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { OperationsService } from '@bindings/reManager'
import { useOperation, type OperationSpec } from '@/hooks/useOperation'
import { emit, resetEvents } from '@/test/wailsEvents'
import { InstallStatus, type DialogRequest, type OperationResult } from '@/lib/types'

vi.mock('@wailsio/runtime', async () => ({ Events: (await import('@/test/wailsEvents')).fakeEvents }))
vi.mock('@bindings/reManager', () => ({
  OperationsService: {
    CancelOperation: vi.fn(async () => {}),
    RespondToOperation: vi.fn(async () => {}),
    PendingOperationPrompt: vi.fn(async () => null),
  },
}))

afterEach(() => {
  resetEvents()
  vi.clearAllMocks()
})

const request: DialogRequest = {
  title: 'Restart the interface?',
  subtitle: '',
  message: '',
  note: '',
  steps: null,
  confirmText: 'Restart',
  cancelText: 'Later',
  inProgressMessage: '',
  infoOnly: false,
  installFlow: false,
  success: false,
  primaryAction: '',
  actions: null,
  choices: false,
}

function setup() {
  const hook = renderHook(() => useOperation())
  const startRun = (spec: Partial<OperationSpec> = {}) => {
    let id = ''
    let result!: Promise<OperationResult>
    act(() => {
      result = hook.result.current.start({
        kind: 'command',
        title: 'Rebuild hashtable',
        target: 'qt-resource-rebuilder:rebuild_hashtable',
        run: async runId => { id = runId },
        ...spec,
      })
    })
    return { id, result }
  }
  const send: typeof emit = (name, data) => act(() => emit(name, data))
  const finish = async (id: string, overrides: Partial<OperationResult> = {}) => {
    await act(async () => emit('op:done', { id, ok: true, canceled: false, ...overrides }))
  }
  return { hook, startRun, send, finish }
}

describe('useOperation', () => {
  it('starts a run as the current operation and hands the run its id', () => {
    const { hook, startRun } = setup()
    const { id } = startRun({ canStop: true })

    expect(id).toBeTruthy()
    expect(hook.result.current.operation).toMatchObject({
      id,
      kind: 'command',
      target: 'qt-resource-rebuilder:rebuild_hashtable',
      status: 'running',
      output: '',
      canStop: true,
    })
    expect(hook.result.current.busy).toBe(true)
  })

  it('builds the log from output and progress lines', () => {
    const { hook, startRun, send } = setup()
    const onOutput = vi.fn()
    const onProgress = vi.fn()
    const { id } = startRun({ onOutput, onProgress })

    send('op:output', { id, text: 'rebuilding\n', hook: false })
    send('op:progress', { id, progress: { component: 'a', index: 1, total: 2, status: InstallStatus.InstallCompleted, message: 'Removed a' } })
    send('op:progress', { id, progress: { component: 'b', index: 2, total: 2, status: InstallStatus.InstallError, message: 'b is in use' } })
    send('op:progress', { id, progress: { component: 'c', index: 2, total: 2, status: InstallStatus.InstallInstalling, message: '' } })

    expect(hook.result.current.operation?.output).toBe('rebuilding\nRemoved a\nError: b is in use\n')
    expect(onOutput).toHaveBeenCalledWith('rebuilding\n')
    expect(onProgress).toHaveBeenCalledTimes(3)
  })

  it('settles on its op:done and returns the result', async () => {
    const { hook, startRun, finish } = setup()
    const { id, result } = startRun({ canStop: true })

    await finish(id, { value: 'ok' })

    await expect(result).resolves.toMatchObject({ id, ok: true, value: 'ok' })
    expect(hook.result.current.operation).toMatchObject({ status: 'succeeded', canStop: false })
    expect(hook.result.current.busy).toBe(false)
  })

  it('keeps a failed run up to show why', async () => {
    const { hook, startRun, finish } = setup()
    const { id } = startRun()

    await finish(id, { ok: false, error: 'exit status 1' })

    expect(hook.result.current.operation?.status).toBe('failed')
  })

  it('closes a background run that worked and a failed run whose caller reports it', async () => {
    const { hook, startRun, finish } = setup()

    const background = startRun({ background: true })
    await finish(background.id)
    expect(hook.result.current.operation).toBeNull()

    const ownFailure = startRun({ reportsOwnFailure: true })
    await finish(ownFailure.id, { ok: false, error: 'nope' })
    expect(hook.result.current.operation).toBeNull()
  })

  it('stops a run through the backend and closes it once canceled', async () => {
    const { hook, startRun, finish } = setup()
    const { id } = startRun({ canStop: true })

    act(() => hook.result.current.stop())

    expect(hook.result.current.operation?.status).toBe('stopping')
    expect(hook.result.current.busy).toBe(true)
    expect(OperationsService.CancelOperation).toHaveBeenCalledWith(id)

    await finish(id, { ok: false, canceled: true })
    expect(hook.result.current.operation).toBeNull()
  })

  it('answers a prompt for the operation that asked', () => {
    const { hook, startRun, send } = setup()
    const { id } = startRun()

    send('op:prompt', { id, request })
    expect(hook.result.current.prompt).toEqual({ operationId: id, request })

    act(() => hook.result.current.respond('confirm'))

    expect(hook.result.current.prompt).toBeNull()
    expect(OperationsService.RespondToOperation).toHaveBeenCalledWith(id, 'confirm')
  })

  it('keeps output from an earlier run that is still going out of the current one', () => {
    const { hook, startRun, send } = setup()
    const earlier = startRun()
    const current = startRun()

    send('op:output', { id: earlier.id, text: 'late line\n', hook: false })

    expect(hook.result.current.operation).toMatchObject({ id: current.id, status: 'running', output: '' })
  })

  it('does not let an earlier run that settles late overwrite the current one', async () => {
    const { hook, startRun, finish } = setup()
    const earlier = startRun()
    const current = startRun()

    await finish(earlier.id, { ok: false, error: 'late failure' })

    expect(hook.result.current.operation).toMatchObject({ id: current.id, status: 'running' })
  })

  describe('hooks', () => {
    const hookEvent = (id: string, finished = false, error = '') =>
      ({ id, title: 'Post-install', expectedSeconds: 3, finished, error })

    it('follows a hook on its own surface, apart from the operation log', () => {
      const { hook, startRun, send } = setup()
      const { id } = startRun()

      send('op:hook', hookEvent(id))
      send('op:output', { id, text: 'hook output\n', hook: true })

      expect(hook.result.current.hookRun).toMatchObject({ operationId: id, status: 'running', output: 'hook output\n' })
      expect(hook.result.current.operation?.output).toBe('')
    })

    it('ignores the finish of a hook from another operation', () => {
      const { hook, startRun, send } = setup()
      const { id } = startRun()

      send('op:hook', hookEvent(id))
      send('op:hook', hookEvent('another', true, 'boom'))

      expect(hook.result.current.hookRun?.status).toBe('running')
    })

    it("stands a succeeded hook down for its follow-up prompt but keeps a failed one", () => {
      const succeeded = setup()
      const first = succeeded.startRun()
      succeeded.send('op:hook', hookEvent(first.id))
      succeeded.send('op:hook', hookEvent(first.id, true))
      succeeded.send('op:prompt', { id: first.id, request })
      expect(succeeded.hook.result.current.hookRun).toBeNull()

      const failed = setup()
      const second = failed.startRun()
      failed.send('op:hook', hookEvent(second.id))
      failed.send('op:hook', hookEvent(second.id, true, 'exit status 2'))
      failed.send('op:prompt', { id: second.id, request })
      expect(failed.hook.result.current.hookRun?.status).toBe('failed')
    })
  })
})
