import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { operationFailure } from '@/lib/operations'
import { ErrorCode } from '@/lib/types'

vi.mock('@wailsio/runtime', () => ({ Events: { On: () => () => {} } }))

const FALLBACK = 'Could not do the thing.'

const coded = (code: ErrorCode, message = '') => ({ code, message, retryable: false })

const codesWithoutOwnMessage = [
  ErrorCode.$zero,
  ErrorCode.ErrInvalidInput,
  ErrorCode.ErrTargetSlotUnhealthy,
  ErrorCode.ErrSlotCheckUnavailable,
  ErrorCode.ErrUpdateInProgress,
  ErrorCode.ErrLockDirMissing,
]
const codesWithOwnMessage = Object.values(ErrorCode)
  .filter(code => code !== ErrorCode.ErrUnknown && !codesWithoutOwnMessage.includes(code))

beforeEach(() => { vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => { vi.restoreAllMocks() })

describe('handleError', () => {
  const unknownMessage = () => handleError(coded(ErrorCode.ErrUnknown), FALLBACK)

  it('gives every known code a message of its own', () => {
    const messages = codesWithOwnMessage.map(code => handleError(coded(code), FALLBACK))

    for (const message of messages) {
      expect(message).not.toBe(FALLBACK)
      expect(message).not.toBe(unknownMessage())
    }
    expect(new Set(messages).size).toBe(messages.length)
  })

  it('falls back to the generic message for a code with no message of its own', () => {
    for (const code of codesWithoutOwnMessage) {
      expect(handleError(coded(code), FALLBACK)).toBe(unknownMessage())
    }
    expect(handleError(coded('ERR_FROM_A_NEWER_BACKEND' as ErrorCode), FALLBACK)).toBe(unknownMessage())
  })

  it("prefers the backend's own message over the code's", () => {
    expect(handleError(coded(ErrorCode.ErrUpdateInProgress, 'An update is already running.'), FALLBACK))
      .toBe('An update is already running.')
  })

  it('reads a coded error from the cause of a rejected bound call', () => {
    const rejection = Object.assign(new Error('call failed'), { cause: coded(ErrorCode.ErrAuthFailed) })

    expect(handleError(rejection, FALLBACK)).toBe(handleError(coded(ErrorCode.ErrAuthFailed), FALLBACK))
  })

  it('reads the code a failed operation reports', () => {
    const failure = operationFailure({ id: 'op', ok: false, canceled: false, code: ErrorCode.ErrDiskFull })

    expect(handleError(failure, FALLBACK)).toBe(handleError(coded(ErrorCode.ErrDiskFull), FALLBACK))
  })

  it.each([
    ['ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]', ErrorCode.ErrAuthFailed],
    ['dial tcp 10.11.99.1:22: connect: connection refused', ErrorCode.ErrConnectionRefused],
    ['dial tcp 10.11.99.1:22: bind to local network: Binding socket to network 261 failed: EPERM (Operation not permitted)', ErrorCode.ErrVPNBlocksLocal],
    ['dial tcp 10.11.99.1:22: i/o timeout', ErrorCode.ErrTimeout],
    ['dial tcp 10.11.99.1:22: connect: no route to host', ErrorCode.ErrNetworkUnreachable],
    ['dial tcp: lookup remarkable.local: no such host', ErrorCode.ErrDNSFailed],
    ['ssh: this private key is passphrase protected', ErrorCode.ErrKeyPassphrase],
    ['sftp: "Failure" (SSH_FX_FAILURE)', ErrorCode.ErrSFTPFailed],
    ['write /home/root/file: no space left on device', ErrorCode.ErrDiskFull],
    ['context canceled', ErrorCode.ErrOperationCanceled],
  ])('recognizes the uncoded error "%s"', (raw, code) => {
    expect(handleError(new Error(raw), FALLBACK)).toBe(handleError(coded(code), FALLBACK))
    expect(errorDetail(new Error(raw))).toBe('')
  })

  it('gives the generic message for an unrecognized connection-level error', () => {
    expect(handleError('read tcp 10.11.99.1:22: connection reset by peer', FALLBACK)).toBe(unknownMessage())
  })

  it("uses the caller's fallback, and keeps the raw text as detail, for anything else", () => {
    const error = new Error('xochitl exited with status 3')

    expect(handleError(error, FALLBACK)).toBe(FALLBACK)
    expect(errorDetail(error)).toBe('xochitl exited with status 3')
  })
})
