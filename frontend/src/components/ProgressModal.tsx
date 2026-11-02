import { useEffect, useState } from 'react'
import { CheckCircle, WarningCircle, Wrench } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { EstimatedProgress } from '@/components/EstimatedProgress'

const STOP_GRACE_MS = 10_000

interface ProgressModalProps {
  title: string
  progressText: string
  terminalOutput: string
  isComplete: boolean
  onClose: () => void
  canStop?: boolean
  onStop?: () => void
  stopWarning?: string
  stopping?: boolean
  onAbandon?: () => void
  terminalTheme?: 'dark' | 'light'
  failed?: boolean
  showOutput?: boolean
  icon?: Icon
  expectedSeconds?: number
  runKey?: string
  onRetry?: () => void
  retryDisabled?: boolean
  runningSeverity?: 'info' | 'neutral'
  onOutputOpenChange?: (open: boolean) => void
}

export function ProgressModal({
  title,
  progressText,
  terminalOutput,
  isComplete,
  onClose,
  canStop,
  onStop,
  stopWarning,
  stopping = false,
  onAbandon,
  terminalTheme,
  failed = false,
  showOutput = false,
  icon,
  expectedSeconds,
  runKey,
  onRetry,
  retryDisabled = false,
  runningSeverity = 'neutral',
  onOutputOpenChange,
}: ProgressModalProps) {
  const state = failed
    ? { severity: 'destructive' as const, icon: WarningCircle }
    : isComplete
      ? { severity: 'success' as const, icon: CheckCircle }
      : { severity: runningSeverity, icon: icon ?? Wrench }

  const barTint = runningSeverity === 'info' ? 'bg-[var(--info)]' : 'bg-primary'

  const [confirmingStop, setConfirmingStop] = useState(false)
  const [stopOverdue, setStopOverdue] = useState(false)
  useEffect(() => {
    setStopOverdue(false)
    if (!stopping) return
    const timer = window.setTimeout(() => setStopOverdue(true), STOP_GRACE_MS)
    return () => window.clearTimeout(timer)
  }, [stopping])
  useEffect(() => {
    if (!canStop) setConfirmingStop(false)
  }, [canStop])

  const requestStop = () => {
    if (stopWarning) setConfirmingStop(true)
    else onStop?.()
  }

  return (
    <>
      <DialogHeader severity={state.severity} icon={state.icon}>
        <DialogTitle>{title}</DialogTitle>
        {progressText && <DialogDescription>{progressText}</DialogDescription>}
      </DialogHeader>

      {isComplete || failed
        ? <Progress value={100} className="h-1 bg-muted" indicatorClassName={failed ? 'bg-destructive' : barTint} aria-label={title} />
        : expectedSeconds
          ? <EstimatedProgress expectedSeconds={expectedSeconds} runKey={runKey} label={title} indicatorClassName={barTint} />
          : <Progress indeterminate className="h-1 bg-muted" indicatorClassName={barTint} aria-label={title} />}

      <OutputDisclosure output={terminalOutput} theme={terminalTheme} failed={failed} alwaysOpen={showOutput} awaitingOutput onOpenChange={onOutputOpenChange} />

      {confirmingStop && <p className="text-sm text-muted-foreground">{stopWarning}</p>}

      <DialogFooter>
        {isComplete ? (
          failed && onRetry ? (
            <>
              <Button variant="outline" onClick={onClose}>Close</Button>
              <Button onClick={onRetry} disabled={retryDisabled}>Try again</Button>
            </>
          ) : (
            <Button onClick={onClose}>Close</Button>
          )
        ) : stopping ? (
          stopOverdue && onAbandon
            ? <Button onClick={onAbandon}>Close</Button>
            : <Button disabled>Stopping…</Button>
        ) : confirmingStop ? (
          <>
            <Button variant="outline" onClick={() => setConfirmingStop(false)}>Keep running</Button>
            <Button variant="destructive" onClick={() => { setConfirmingStop(false); onStop?.() }}>Stop</Button>
          </>
        ) : canStop ? (
          <Button variant="outline" onClick={requestStop}>Stop</Button>
        ) : (
          <Button disabled>Close</Button>
        )}
      </DialogFooter>
    </>
  )
}
