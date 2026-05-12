import { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { lightTheme, darkTheme } from '@/lib/terminalThemes'
import '@xterm/xterm/css/xterm.css'

interface TerminalProps {
  output: string
  theme?: 'dark' | 'light'
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
