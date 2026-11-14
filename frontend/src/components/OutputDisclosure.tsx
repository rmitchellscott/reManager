import { CaretDown } from '@phosphor-icons/react'
import { useEffect, useRef, useState } from 'react'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'
import { cn } from '@/lib/utils'

interface OutputDisclosureProps {
  output: string
  theme?: 'dark' | 'light'
  failed?: boolean
  alwaysOpen?: boolean
  label?: string
  className?: string
  onOpenChange?: (open: boolean) => void
  awaitingOutput?: boolean
}

// The copy button is 32px, 8px in from the top; a one-line box is shorter and cut it off.
const COPY_BUTTON_CLEARANCE_PX = 48
const FIT_SETTLE_MS = 250

export function OutputDisclosure({
  output,
  theme,
  failed = false,
  alwaysOpen = false,
  label = 'Details',
  className,
  onOpenChange,
  awaitingOutput = false,
}: OutputDisclosureProps) {
  const [open, setOpen] = useState(alwaysOpen)
  const openedForFailure = useRef(false)

  useEffect(() => {
    if (failed && !openedForFailure.current) {
      openedForFailure.current = true
      setOpen(true)
    }
    if (!failed) openedForFailure.current = false
  }, [failed])

  const showing = (!!output || awaitingOutput) && open
  useEffect(() => {
    onOpenChange?.(showing)
    return () => onOpenChange?.(false)
  }, [showing, onOpenChange])

  // xterm sizes from the container, so the box needs a pixel height; the terminal measures it because a long line wraps to several rows on a narrow screen. The fit waits for a second line and a short settle because a command's echo arrives alone, ahead of its output (about 350ms for launcherctl status).
  const [measuredHeight, setMeasuredHeight] = useState<number | null>(null)
  const measuredHeightRef = useRef(measuredHeight)
  useEffect(() => {
    measuredHeightRef.current = measuredHeight
  })
  const [fittedHeight, setFittedHeight] = useState<number | null>(null)
  const outputShowing = showing && !!output
  const pastFirstLine = output.replace(/\n$/, '').includes('\n')
  useEffect(() => {
    if (!outputShowing) {
      setMeasuredHeight(null)
      setFittedHeight(null)
      return
    }
    if (!pastFirstLine) return
    const settle = setTimeout(() => setFittedHeight(measuredHeightRef.current), FIT_SETTLE_MS)
    return () => clearTimeout(settle)
  }, [outputShowing, pastFirstLine])
  const outgrewFit = fittedHeight !== null && measuredHeight !== null && measuredHeight > fittedHeight

  if (!output && !awaitingOutput) return null

  const contentHeight = fittedHeight ?? measuredHeight ?? 0
  const maxHeight = 'min(45vh, 360px)'

  const terminal = (
    <div
      className="rounded-[var(--radius)] overflow-hidden overscroll-y-contain"
      style={{ height: outgrewFit ? maxHeight : `min(45vh, 360px, ${Math.max(contentHeight, COPY_BUTTON_CLEARANCE_PX)}px)` }}
    >
      <TerminalWithCopy output={output} theme={theme} onContentHeightChange={setMeasuredHeight} />
    </div>
  )

  if (alwaysOpen) {
    return <div className={className}>{terminal}</div>
  }

  return (
    <div className={cn('space-y-2', className)}>
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        aria-expanded={open}
        className="flex items-center gap-1.5 text-sm font-medium text-muted-foreground hover:text-foreground touch:hit-target"
      >
        {label}
        <CaretDown className={cn('h-3.5 w-3.5 transition-transform', open && 'rotate-180')} />
      </button>
      {open && terminal}
    </div>
  )
}
