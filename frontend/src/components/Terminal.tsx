import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm, ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

interface TerminalProps {
  output: string
}

const lightTheme: ITheme = {
  background: '#fafafa',
  foreground: '#1a1a1a',
  cursor: '#1a1a1a',
  black: '#1a1a1a',
  red: '#d93526',
  green: '#3a7d2c',
  yellow: '#b58900',
  blue: '#2563eb',
  magenta: '#8b5cf6',
  cyan: '#0891b2',
  white: '#e5e5e5',
}

const darkTheme: ITheme = {
  background: '#1a1a1a',
  foreground: '#fafafa',
  cursor: '#fafafa',
  black: '#3a3a3a',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#fbbf24',
  blue: '#60a5fa',
  magenta: '#a78bfa',
  cyan: '#22d3ee',
  white: '#e5e5e5',
}

function getSystemTheme(): ITheme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? darkTheme : lightTheme
}

export function Terminal({ output }: TerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const lastOutputRef = useRef<string>('')
  const [isDark, setIsDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return

    const term = new XTerm({
      theme: getSystemTheme(),
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

    const handleThemeChange = (e: MediaQueryListEvent) => {
      term.options.theme = e.matches ? darkTheme : lightTheme
      setIsDark(e.matches)
    }
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQuery.addEventListener('change', handleThemeChange)

    return () => {
      window.removeEventListener('resize', handleResize)
      mediaQuery.removeEventListener('change', handleThemeChange)
      term.dispose()
    }
  }, [])

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
