import { useEffect, useState } from 'react'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'

interface EstimatedProgressProps {
  expectedSeconds: number
  runKey?: string
  label?: string
  className?: string
  indicatorClassName?: string
}

// Holds short of 100% on purpose: reaching the end while work continues is worse than admitting the estimate was approximate.
const CEILING = 95
const ESTIMATE_MARK = 80

export function EstimatedProgress({ expectedSeconds, runKey, label, className, indicatorClassName }: EstimatedProgressProps) {
  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    setElapsed(0)
    const startedAt = Date.now()
    const tick = window.setInterval(() => setElapsed((Date.now() - startedAt) / 1000), 250)
    return () => window.clearInterval(tick)
  }, [runKey, expectedSeconds])

  const fraction = expectedSeconds > 0 ? elapsed / expectedSeconds : 0
  const value = fraction <= 1
    ? fraction * ESTIMATE_MARK
    : ESTIMATE_MARK + (CEILING - ESTIMATE_MARK) * (1 - 1 / (1 + (fraction - 1)))

  return (
    <Progress
      value={value}
      className={cn('h-1 bg-muted', className)}
      indicatorClassName={cn('transition-[width] duration-300 ease-linear', indicatorClassName)}
      aria-label={label}
    />
  )
}
