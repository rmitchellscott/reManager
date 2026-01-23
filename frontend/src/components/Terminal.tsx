import { useEffect, useRef } from 'react'
import { Terminal as XTerm, ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

interface TerminalProps {
  output: string
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

export function Terminal({ output, theme }: TerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const lastOutputRef = useRef<string>('')
  const isDark = theme === 'dark'

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return

    const term = new XTerm({
      theme: isDark ? darkTheme : lightTheme,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      cursorBlink: false,
      disableStdin: true,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)

    requestAnimationFrame(() => {
      try {
        fitAddon.fit()
      } catch {}
    })

    xtermRef.current = term
    fitAddonRef.current = fitAddon

    const handleResize = () => {
      try {
        fitAddon.fit()
      } catch {}
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      term.dispose()
    }
  }, [])

  useEffect(() => {
    if (xtermRef.current) {
      xtermRef.current.options.theme = isDark ? darkTheme : lightTheme
    }
  }, [isDark])

  useEffect(() => {
    if (!xtermRef.current) return

    const newContent = output.slice(lastOutputRef.current.length)
    if (newContent) {
      xtermRef.current.write(newContent.replace(/\n/g, '\r\n'))
      lastOutputRef.current = output
    }
  }, [output])

  return (
    <div
      ref={terminalRef}
      className="w-full h-full min-h-[300px] rounded-md overflow-hidden"
      style={{ backgroundColor: isDark ? '#1a1a1a' : '#fafafa' }}
    />
  )
}
