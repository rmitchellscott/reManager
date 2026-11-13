import { useEffect, useState } from 'react'
import { CaretDown, ClipboardText } from '@phosphor-icons/react'
import { ShellService, SystemService } from '@bindings/reManager'
const KEY_SEQUENCES: Record<string, string> = {
  '⎋': '\x1b',
  '⇥': '\t',
  '◀': '\x1b[D',
  '▲': '\x1b[A',
  '▼': '\x1b[B',
  '▶': '\x1b[C',
  '|': '|',
  '/': '/',
  '~': '~',
  '-': '-',
}

const KEYS_ON_IPAD_KEYBOARD = new Set(['⇥'])

const softKeyboardIsIPad = SystemService.GetPlatformCapabilities()
  .then((capabilities) => capabilities.iPad)
  .catch(() => false)

const KEY_NAMES: Record<string, string> = {
  '⎋': 'Escape',
  '⇥': 'Tab',
  '◀': 'Left',
  '▲': 'Up',
  '▼': 'Down',
  '▶': 'Right',
}

interface TerminalKeyBarProps {
  ctrlArmed: boolean
  onToggleCtrl: () => void
}

const KEY_CLASS =
  'inline-flex h-[34px] min-w-[44px] shrink-0 items-center justify-center rounded-md border bg-background px-[12px] py-[6px] text-[14px] leading-[20px] font-medium'

export function TerminalKeyBar({ ctrlArmed, onToggleCtrl }: TerminalKeyBarProps) {
  const [isIPad, setIsIPad] = useState(false)
  useEffect(() => {
    softKeyboardIsIPad.then(setIsIPad)
  }, [])

  const keys = Object.entries(KEY_SEQUENCES).filter(
    ([label]) => !(isIPad && KEYS_ON_IPAD_KEYBOARD.has(label)),
  )

  const send = (sequence: string) => {
    ShellService.WriteToShell(sequence).catch((err) => {
      console.error('Failed to write to shell:', err)
    })
  }

  const paste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text) send(text)
    } catch (err) {
      console.error('Failed to read clipboard:', err)
    }
  }

  const dismissKeyboard = () => {
    const active = document.activeElement
    if (active instanceof HTMLElement) active.blur()
  }

  // A side inset in portrait is the iPhone Duo's closed display, where it
  // reserves a column for the camera and status bar at the top. The row sits on
  // the keyboard, well below them, so it takes the full width. In landscape the
  // inset is a notch that can reach this row, so it stays inset there.
  return (
    <div
      data-touch-scroll
      className="hidden shrink-0 touch-pan-x items-center gap-[4px] overflow-x-auto border-t bg-background px-[8px] py-[6px] touch:flex compact:portrait:-mr-[max(0px,calc(var(--safe-area-right)-12px))]"
    >
      <button
        type="button"
        onMouseDown={(e) => e.preventDefault()}
        onClick={onToggleCtrl}
        aria-pressed={ctrlArmed}
        aria-label="Control"
        className={`${KEY_CLASS} ${ctrlArmed ? 'bg-foreground text-background' : ''}`}
      >
        ^
      </button>
      {keys.map(([label, sequence]) => (
        <button
          key={label}
          type="button"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => send(sequence)}
          aria-label={KEY_NAMES[label]}
          className={KEY_CLASS}
        >
          {label}
        </button>
      ))}
      {!isIPad && (
        <>
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={paste}
            aria-label="Paste"
            className={KEY_CLASS}
          >
            <ClipboardText className="h-[16px] w-[16px]" aria-hidden />
          </button>
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={dismissKeyboard}
            aria-label="Hide keyboard"
            className={`${KEY_CLASS} ml-auto`}
          >
            <CaretDown className="h-[16px] w-[16px]" aria-hidden />
          </button>
        </>
      )}
    </div>
  )
}
