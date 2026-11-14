import { useEffect, useRef } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { lightTheme, darkTheme } from '@/lib/terminalThemes'
import '@xterm/xterm/css/xterm.css'

interface TerminalProps {
  output: string
  theme?: 'dark' | 'light'
  onContentHeightChange?: (height: number) => void
}

// xterm scrolls its own viewport a whole row at a time with no touch momentum, so instead the container scrolls natively over a spacer and the terminal stays pinned in view.
// It stays one screen tall because every row is a DOM element: sized to thousands of lines, each write froze the page.
export function Terminal({ output, theme, onContentHeightChange }: TerminalProps) {
  const outerRef = useRef<HTMLDivElement>(null)
  const scrollerRef = useRef<HTMLDivElement>(null)
  const hostRef = useRef<HTMLDivElement>(null)
  const spacerRef = useRef<HTMLDivElement>(null)
  const thumbRef = useRef<HTMLDivElement>(null)
  const thumbHideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const lastOutputRef = useRef<string>('')
  // Measured only when the scroll moves, not before each write: the box's height changes as xterm is sized to it, and a mid-change check reads a followed terminal as scrolled up.
  const followingOutputRef = useRef(true)
  const followScrollTopRef = useRef<number | null>(null)
  const isDark = theme === 'dark'
  const onContentHeightChangeRef = useRef(onContentHeightChange)
  useEffect(() => {
    onContentHeightChangeRef.current = onContentHeightChange
  })

  const rowHeight = () => {
    const term = xtermRef.current
    const screen = term?.element?.querySelector<HTMLElement>('.xterm-screen')
    return term && screen ? screen.offsetHeight / term.rows : 0
  }

  const scrollToEndIfFollowing = () => {
    const scroller = scrollerRef.current
    if (!followingOutputRef.current || !scroller) return
    scroller.scrollTop = scroller.scrollHeight
    followScrollTopRef.current = scroller.scrollTop
  }

  // iOS draws its scroll indicator beneath the pinned terminal's layer, so it vanishes except at the very end; touch devices get this one instead.
  const revealScrollThumb = () => {
    const scroller = scrollerRef.current
    const thumb = thumbRef.current
    if (!scroller || !thumb) return
    const { scrollTop, scrollHeight, clientHeight } = scroller
    if (scrollHeight <= clientHeight + 1) return
    const thumbHeight = Math.max(24, (clientHeight * clientHeight) / scrollHeight)
    const thumbTop = (scrollTop / (scrollHeight - clientHeight)) * (clientHeight - thumbHeight)
    thumb.style.height = `${thumbHeight}px`
    thumb.style.transform = `translateY(${thumbTop}px)`
    thumb.style.opacity = '1'
    if (thumbHideTimerRef.current) clearTimeout(thumbHideTimerRef.current)
    thumbHideTimerRef.current = setTimeout(() => {
      thumb.style.opacity = '0'
    }, 800)
  }

  const handleScroll = () => {
    const scroller = scrollerRef.current
    if (!scroller) return
    followingOutputRef.current = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 2
    showRowAtScrollPosition()
    const scrolledByFollowingOutput = scroller.scrollTop === followScrollTopRef.current
    followScrollTopRef.current = null
    if (!scrolledByFollowingOutput) revealScrollThumb()
  }

  const shiftByRowRemainder = () => {
    const term = xtermRef.current
    const scroller = scrollerRef.current
    const height = rowHeight()
    if (!term?.element || !scroller || !(height > 0)) return
    const offset = scroller.scrollTop - term.buffer.active.viewportY * height
    term.element.style.transform = `translateY(${-offset}px)`
  }

  const showRowAtScrollPosition = () => {
    const term = xtermRef.current
    const scroller = scrollerRef.current
    const height = rowHeight()
    if (!term || !scroller || !(height > 0)) return
    const buffer = term.buffer.active
    const topRow = Math.min(Math.floor(scroller.scrollTop / height), buffer.baseY)
    if (topRow !== buffer.viewportY) term.scrollToLine(topRow)
    shiftByRowRemainder()
  }

  const sizeSpacerToOutput = () => {
    const term = xtermRef.current
    const outer = outerRef.current
    const scroller = scrollerRef.current
    const host = hostRef.current
    const spacer = spacerRef.current
    const height = rowHeight()
    if (!term || !outer || !scroller || !host || !spacer || !(height > 0)) return
    const buffer = term.buffer.active
    const outputRows = buffer.baseY + buffer.cursorY + (buffer.cursorX > 0 ? 1 : 0)
    spacer.style.height = `${Math.max(0, outputRows * height - host.offsetHeight)}px`
    const verticalPadding = outer.offsetHeight - scroller.clientHeight
    onContentHeightChangeRef.current?.(Math.ceil(outputRows * height + verticalPadding))
  }

  const fitToContainer = () => {
    const term = xtermRef.current
    const scroller = scrollerRef.current
    const host = hostRef.current
    if (!term || !scroller || !host) return
    host.style.height = `${scroller.clientHeight}px`
    const proposed = fitAddonRef.current?.proposeDimensions()
    if (!proposed || isNaN(proposed.cols) || isNaN(proposed.rows)) return
    // Two rows past what fits: a scroll position part way through a row shows a sliver above the screenful and one below.
    const rows = proposed.rows + 2
    if (proposed.cols !== term.cols || rows !== term.rows) term.resize(proposed.cols, rows)
    sizeSpacerToOutput()
    scrollToEndIfFollowing()
    showRowAtScrollPosition()
  }

  useEffect(() => {
    const scroller = scrollerRef.current
    const host = hostRef.current
    if (!scroller || !host || xtermRef.current) return

    const term = new XTerm({
      theme: isDark ? darkTheme : lightTheme,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      cursorBlink: false,
      disableStdin: true,
      scrollback: 50000,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(host)

    xtermRef.current = term
    fitAddonRef.current = fitAddon

    // The container does the scrolling, so xterm's own viewport must not take the wheel or a drag, or show a second scrollbar.
    const xtermViewport = term.element?.querySelector<HTMLElement>('.xterm-viewport')
    if (xtermViewport) xtermViewport.style.overflowY = 'hidden'
    const keepFromXterm = (ev: Event) => ev.stopPropagation()
    const captureOnly = { capture: true, passive: true }
    scroller.addEventListener('wheel', keepFromXterm, captureOnly)
    scroller.addEventListener('touchstart', keepFromXterm, captureOnly)
    scroller.addEventListener('touchmove', keepFromXterm, captureOnly)

    // Nothing is typed here; a tap focusing xterm's hidden textarea otherwise brings up the iOS keyboard.
    term.textarea?.setAttribute('inputmode', 'none')

    // A cross-row scroll asks xterm for the new row, which it draws a frame later; shifting only once it has drawn keeps them in step.
    const renderSubscription = term.onRender(shiftByRowRemainder)

    requestAnimationFrame(fitToContainer)

    // The container, not the window: a dialog widening to show output resizes it without the window changing.
    const resizeObserver = new ResizeObserver(fitToContainer)
    resizeObserver.observe(scroller)

    return () => {
      resizeObserver.disconnect()
      renderSubscription.dispose()
      scroller.removeEventListener('wheel', keepFromXterm, captureOnly)
      scroller.removeEventListener('touchstart', keepFromXterm, captureOnly)
      scroller.removeEventListener('touchmove', keepFromXterm, captureOnly)
      if (thumbHideTimerRef.current) clearTimeout(thumbHideTimerRef.current)
      term.dispose()
    }
  }, [])

  useEffect(() => {
    if (xtermRef.current) {
      xtermRef.current.options.theme = isDark ? darkTheme : lightTheme
    }
  }, [isDark])

  useEffect(() => {
    const term = xtermRef.current
    if (!term) return

    const restarted = !output.startsWith(lastOutputRef.current)
    const newContent = restarted ? output : output.slice(lastOutputRef.current.length)
    lastOutputRef.current = output
    if (!restarted && !newContent) return

    if (restarted) term.reset()
    term.write(newContent.replace(/\n/g, '\r\n'), () => {
      sizeSpacerToOutput()
      scrollToEndIfFollowing()
      showRowAtScrollPosition()
    })
  }, [output])

  return (
    <div
      ref={outerRef}
      className="relative w-full h-full rounded-md overflow-hidden p-3"
      // The scrollbar takes its color from color-scheme, not the background, and the terminal's theme can differ from the app's.
      style={{ backgroundColor: isDark ? '#1a1a1a' : '#fafafa', colorScheme: isDark ? 'dark' : 'light' }}
    >
      <div
        ref={scrollerRef}
        data-touch-scroll
        onScroll={handleScroll}
        className="h-full overflow-x-hidden overflow-y-auto overscroll-contain touch:[scrollbar-width:none] touch:[&::-webkit-scrollbar]:hidden"
      >
        <div ref={hostRef} className="sticky top-0 overflow-hidden" />
        <div ref={spacerRef} />
      </div>
      <div
        ref={thumbRef}
        className="pointer-events-none absolute top-3 right-[2px] hidden w-[3px] rounded-full opacity-0 transition-opacity duration-300 touch:block"
        style={{ backgroundColor: isDark ? 'rgba(255, 255, 255, 0.45)' : 'rgba(0, 0, 0, 0.4)' }}
      />
    </div>
  )
}
