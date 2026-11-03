import { useEffect, useState } from 'react'
import { Progress } from '@/components/ui/progress'

export interface RetryPhase {
  kind: 'dialing' | 'waiting'
  // A deadline, not a remaining count: a decrementing counter drifts and a backgrounded window never catches up.
  endsAt: number
  durationMs: number
  elapsedMs: number
  totalMs: number
}

interface RetryCountdownProps {
  attempt: number
  maxAttempts: number
  phase: RetryPhase | null
}

export function RetryCountdown({ attempt, maxAttempts, phase }: RetryCountdownProps) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!phase) return
    const tick = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(tick)
  }, [phase])

  const remaining = phase ? Math.max(0, phase.endsAt - now) : 0
  const seconds = Math.ceil(remaining / 1000)
  const pct = phase
    ? Math.min(100, ((phase.elapsedMs + (phase.durationMs - remaining)) / phase.totalMs) * 100)
    : 0

  return (
    <div>
      <Progress
        value={pct}
        className="h-1 bg-muted"
        indicatorClassName="bg-[var(--warning)] rounded-full transition-[width] duration-300 ease-linear"
      />
      <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
        <span className="tabular-nums">Attempt {attempt} of {maxAttempts}</span>
        <span className="tabular-nums">
          {phase?.kind === 'waiting' && seconds > 1 ? `Next try in ${seconds}s` : 'Trying now…'}
        </span>
      </div>
    </div>
  )
}
