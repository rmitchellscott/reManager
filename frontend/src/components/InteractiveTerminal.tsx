import { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal as XTerm, ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Copy, CopyCheck } from 'lucide-react'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import '@xterm/xterm/css/xterm.css'

interface InteractiveTerminalProps {
  isConnected: boolean
  visible: boolean
  onRunningChange?: (running: boolean) => void
  theme?: 'dark' | 'light'
}

const lightTheme: ITheme = {
  background: '#fafafa',
  foreground: '#1a1a1a',
  cursor: '#1a1a1a',
  selectionBackground: '#b4d5fe',
  black: '#1a1a1a',
  red: '#c41a16',
  green: '#007400',
  yellow: '#826b00',
  blue: '#0451a5',
  magenta: '#a626a4',
  cyan: '#0997b3',
  white: '#767676',
  brightBlack: '#5c5c5c',
  brightRed: '#d93526',
  brightGreen: '#3a7d2c',
  brightYellow: '#b58900',
  brightBlue: '#2563eb',
  brightMagenta: '#8b5cf6',
  brightCyan: '#0891b2',
  brightWhite: '#1a1a1a',
}

const darkTheme: ITheme = {
  background: '#1a1a1a',
  foreground: '#fafafa',
  cursor: '#fafafa',
  selectionBackground: '#3a3a5a',
  black: '#3a3a3a',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#fbbf24',
  blue: '#60a5fa',
  magenta: '#a78bfa',
  cyan: '#22d3ee',
  white: '#e5e5e5',
  brightBlack: '#666666',
  brightRed: '#fca5a5',
  brightGreen: '#86efac',
  brightYellow: '#fde047',
  brightBlue: '#93c5fd',
  brightMagenta: '#c4b5fd',
  brightCyan: '#67e8f9',
  brightWhite: '#ffffff',
}

export function InteractiveTerminal({ isConnected, visible, onRunningChange, theme }: InteractiveTerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)
  const isDark = theme === 'dark'
  const [shellRunning, setShellRunning] = useState(false)
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

    requestAnimationFrame(() => {
      try {
        fitAddon.fit()
      } catch {
        // Ignore fit errors
      }
    })

    xtermRef.current = term
    fitAddonRef.current = fitAddon

    term.onData((data) => {
      window.go.main.App.WriteToShell(data).catch((err) => {
        console.error('Failed to write to shell:', err)
      })
    })

    term.onResize(({ rows, cols }) => {
      window.go.main.App.ResizeShell(rows, cols).catch((err) => {
        console.error('Failed to resize shell:', err)
      })
    })

    const handleResize = () => {
      try {
        fitAddon.fit()
      } catch {
        // Ignore fit errors
      }
    }
    window.addEventListener('resize', handleResize)

    const unsubscribeOutput = window.runtime.EventsOn('shell:output', (...args: unknown[]) => {
      const data = args[0] as string
      term.write(data)
    })

    const unsubscribeStopped = window.runtime.EventsOn('shell:stopped', () => {
      setShellRunning(false)
      term.writeln('\r\n[Shell session ended]')
    })

    cleanupRef.current = () => {
      window.removeEventListener('resize', handleResize)
      unsubscribeOutput()
      unsubscribeStopped()
      term.dispose()
      xtermRef.current = null
      fitAddonRef.current = null
    }
  }, [isDark])

  const startShell = useCallback(async () => {
    if (!isConnected) return

    setShellRunning(true)

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

      await window.go.main.App.StartShell(rows, cols)
      term.focus()
    } catch (err) {
      console.error('Failed to start shell:', err)
      term.writeln(`\r\nFailed to start shell: ${err}`)
      setShellRunning(false)
    }
  }, [isConnected, initTerminal])

  const stopShell = useCallback(async () => {
    try {
      await window.go.main.App.StopShell()
    } catch (err) {
      console.error('Failed to stop shell:', err)
    }
  }, [])

  useEffect(() => {
    if (!isConnected && shellRunning) {
      setShellRunning(false)
      xtermRef.current?.writeln('\r\n[Connection lost]')
    }
  }, [isConnected, shellRunning])

  useEffect(() => {
    onRunningChange?.(shellRunning)
  }, [shellRunning, onRunningChange])

  useEffect(() => {
    if (visible && shellRunning && fitAddonRef.current) {
      requestAnimationFrame(() => {
        try {
          fitAddonRef.current?.fit()
        } catch {
          // Ignore fit errors
        }
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
        window.go.main.App.StopShell().catch(() => {})
      }
      cleanupRef.current?.()
    }
  }, [shellRunning])

  return (
    <div style={{ display: visible ? 'block' : 'none' }}>
      <div className="flex gap-2 mb-2">
        {!shellRunning ? (
          <Button variant="outline" className="w-full" onClick={startShell} disabled={!isConnected}>
            Start Terminal
          </Button>
        ) : (
          <Button variant="outline" className="w-full md:w-1/2" onClick={stopShell}>
            Stop Terminal
          </Button>
        )}
      </div>
      {shellRunning && (
        <div className="relative">
          <div
            ref={terminalRef}
            className="h-[calc(100vh-350px)] min-h-[200px] rounded-md overflow-hidden"
            style={{ backgroundColor: isDark ? '#1a1a1a' : '#fafafa' }}
          />
          <div className="absolute top-2 right-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={copyTerminalBuffer}
                  className="h-8 w-8 bg-background/95 backdrop-blur-sm hover:bg-background"
                >
                  {isCopied ? (
                    <CopyCheck className="h-4 w-4" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {isCopied ? 'Copied!' : 'Copy output'}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      )}
    </div>
  )
}
