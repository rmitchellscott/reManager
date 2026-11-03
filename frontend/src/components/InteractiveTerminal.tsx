import { CheckCircle, Copy, Stop } from '@phosphor-icons/react'
import { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { TerminalKeyBar } from '@/components/TerminalKeyBar'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { useSoftKeyboard } from '@/hooks/useSoftKeyboard'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import { lightTheme, darkTheme } from '@/lib/terminalThemes'
import { errorDetail, handleError } from '@/lib/errorMessages'
import '@xterm/xterm/css/xterm.css'
import { ShellService } from '@bindings/reManager'
import { onEvent } from '@/lib/events'

interface InteractiveTerminalProps {
  isConnected: boolean
  visible: boolean
  onRunningChange?: (running: boolean) => void
  theme?: 'dark' | 'light'
}

const terminalControl = 'h-[36px] px-[12px] text-[14px] compact:h-[40px] touch:hit-target'

interface AndroidTerminalInputBridge {
  setTerminalFocused(focused: boolean): void
}

declare global {
  interface Window {
    androidTerminalInput?: AndroidTerminalInputBridge
  }
}

const reportTerminalFocused = () => window.androidTerminalInput?.setTerminalFocused(true)
const reportTerminalBlurred = () => window.androidTerminalInput?.setTerminalFocused(false)

const blurActiveElement = () => {
  const active = document.activeElement
  if (active instanceof HTMLElement) active.blur()
}

const floatingControl = 'h-[32px] w-[32px] border bg-background hover:bg-accent compact:h-[40px] compact:w-[40px] touch:hit-target'

export function InteractiveTerminal({ isConnected, visible, onRunningChange, theme }: InteractiveTerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)
  const isDark = theme === 'dark'
  const [shellRunning, setShellRunning] = useState(false)
  const [hasStarted, setHasStarted] = useState(false)
  const [startError, setStartError] = useState<{ message: string; detail: string } | null>(null)
  const { visible: keyboardVisible } = useSoftKeyboard()
  const ctrlArmedRef = useRef(false)
  const [ctrlArmed, setCtrlArmed] = useState(false)
  const { isCopied, copyToClipboard } = useCopyToClipboard()

  const copyTerminalBuffer = useCallback(() => {
    const term = xtermRef.current
    if (!term) return
    const buffer = term.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buffer.length; i++) {
      const line = buffer.getLine(i)
      if (line) lines.push(line.translateToString(true))
    }
    const text = lines.join('\n').trimEnd()
    if (text) copyToClipboard(text)
  }, [copyToClipboard])

  const initTerminal = useCallback(() => {
    if (!terminalRef.current || xtermRef.current) return

    const term = new XTerm({
      theme: isDark ? darkTheme : lightTheme,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      cursorBlink: true,
      disableStdin: false,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)

    // xterm feeds keystrokes through a hidden textarea, which iOS treats as prose: predictions, autocapitalize and autocorrect, none of which survives a shell.
    const helper = terminalRef.current.querySelector('.xterm-helper-textarea')
    if (helper) {
      helper.setAttribute('autocorrect', 'off')
      helper.setAttribute('autocapitalize', 'none')
      helper.setAttribute('autocomplete', 'off')
      helper.setAttribute('spellcheck', 'false')
      // Samsung Keyboard ignores all four, so on Android the native WebView switches the keyboard to character input while this has focus.
      helper.addEventListener('focus', reportTerminalFocused)
      helper.addEventListener('blur', reportTerminalBlurred)
    }

    requestAnimationFrame(() => {
      try {
        fitAddon.fit()
      } catch {}
    })

    xtermRef.current = term
    fitAddonRef.current = fitAddon

    term.onData((data) => {
      let payload = data
      if (ctrlArmedRef.current && /^[a-zA-Z]$/.test(data)) {
        payload = String.fromCharCode(data.toUpperCase().charCodeAt(0) - 64)
        ctrlArmedRef.current = false
        setCtrlArmed(false)
      }
      ShellService.WriteToShell(payload).catch((err) => {
        console.error('Failed to write to shell:', err)
      })
    })

    term.onResize(({ rows, cols }) => {
      ShellService.ResizeShell(rows, cols).catch((err) => {
        console.error('Failed to resize shell:', err)
      })
    })

    const handleResize = () => {
      try {
        fitAddon.fit()
      } catch {}
    }
    window.addEventListener('resize', handleResize)

    const observer = new ResizeObserver(handleResize)
    if (terminalRef.current) observer.observe(terminalRef.current)

    const unsubscribeOutput = onEvent('shell:output', data => term.write(data))

    const unsubscribeStopped = onEvent('shell:stopped', () => {
      // Blur first: removing xterm's focused textarea from the document strands anything keyed off an on-screen keyboard.
      blurActiveElement()
      setShellRunning(false)
    })

    cleanupRef.current = () => {
      window.removeEventListener('resize', handleResize)
      observer.disconnect()
      unsubscribeOutput()
      unsubscribeStopped()
      helper?.removeEventListener('focus', reportTerminalFocused)
      helper?.removeEventListener('blur', reportTerminalBlurred)
      // Taking the focused textarea out of the document is not guaranteed to fire blur.
      reportTerminalBlurred()
      term.dispose()
      xtermRef.current = null
      fitAddonRef.current = null
    }
  }, [isDark])

  const startShell = useCallback(async () => {
    if (!isConnected) return

    setShellRunning(true)
    setHasStarted(true)
    setStartError(null)

    // The box renders on this state change and xterm measures it on attach; without this wait the shell opened at 0x0 against an unsized container.
    await new Promise(resolve => setTimeout(resolve, 50))

    initTerminal()

    const fitAddon = fitAddonRef.current
    const term = xtermRef.current

    if (!fitAddon || !term) {
      setShellRunning(false)
      return
    }

    try {
      const dims = fitAddon.proposeDimensions()
      const rows = dims?.rows || 24
      const cols = dims?.cols || 80

      await ShellService.StartShell(rows, cols)
      term.focus()
    } catch (err) {
      setStartError({ message: handleError(err, 'Could not start the shell.'), detail: errorDetail(err) })
      setShellRunning(false)
    }
  }, [isConnected, initTerminal])

  const stopShell = useCallback(async () => {
    blurActiveElement()
    try {
      await ShellService.StopShell()
    } catch (err) {
      console.error('Failed to stop shell:', err)
    }
  }, [])

  useEffect(() => {
    if (!isConnected && shellRunning) {
      setShellRunning(false)
    }
  }, [isConnected, shellRunning])

  useEffect(() => {
    onRunningChange?.(shellRunning)
  }, [shellRunning, onRunningChange])

  const autoStartedRef = useRef(false)
  useEffect(() => {
    if (!visible) {
      autoStartedRef.current = false
      return
    }
    if (shellRunning) {
      autoStartedRef.current = true
      return
    }
    if (!isConnected || autoStartedRef.current) return
    autoStartedRef.current = true
    startShell()
  }, [visible, isConnected, shellRunning, startShell])

  useEffect(() => {
    if (visible && shellRunning && fitAddonRef.current) {
      requestAnimationFrame(() => {
        try {
          fitAddonRef.current?.fit()
        } catch {}
      })
    }
  }, [visible, shellRunning])

  useEffect(() => {
    if (xtermRef.current) {
      xtermRef.current.options.theme = isDark ? darkTheme : lightTheme
    }
  }, [isDark])

  useEffect(() => {
    return () => {
      if (shellRunning) {
        ShellService.StopShell().catch(() => {})
      }
      cleanupRef.current?.()
    }
  }, [shellRunning])

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-[8px]" style={{ display: visible ? 'flex' : 'none' }}>
      {shellRunning ? (
        <div className="flex min-h-0 flex-1 flex-col compact:-mx-[12px] compact:-mb-[8px]">
          <div className="relative flex min-h-0 flex-1 flex-col">
            <div
              ref={terminalRef}
              className="min-h-[120px] flex-1 overflow-hidden rounded-[var(--radius)] p-3 compact:rounded-none"
              style={{ backgroundColor: isDark ? '#1a1a1a' : '#fafafa' }}
            />
            <div className="absolute right-2 top-2 flex items-center gap-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon" onClick={copyTerminalBuffer} className={floatingControl} aria-label="Copy output">
                    {isCopied ? <CheckCircle className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{isCopied ? 'Copied' : 'Copy output'}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon" onClick={stopShell} className={floatingControl} aria-label="Stop shell">
                    <Stop className="h-4 w-4" weight="fill" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Stop shell</TooltipContent>
              </Tooltip>
            </div>
          </div>
          {isConnected && visible && keyboardVisible && (
            <TerminalKeyBar
              ctrlArmed={ctrlArmed}
              onToggleCtrl={() => {
                const next = !ctrlArmedRef.current
                ctrlArmedRef.current = next
                setCtrlArmed(next)
              }}
            />
          )}
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3">
          <p className="text-sm text-muted-foreground">
            {startError ? startError.message : hasStarted ? 'Session ended.' : 'No shell running.'}
          </p>
          {startError && <OutputDisclosure output={startError.detail} className="w-full max-w-md" />}
          <Button variant="outline" className={terminalControl} onClick={startShell} disabled={!isConnected}>
            {startError ? 'Try again' : hasStarted ? 'Start again' : 'Start shell'}
          </Button>
        </div>
      )}
    </div>
  )
}
