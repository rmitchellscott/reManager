import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const linkClass = 'text-xs font-medium bg-transparent border-none cursor-pointer px-1 py-1 touch:hit-target disabled:opacity-50 disabled:cursor-default'

export function BulkStageLine({ staged, total, idleLabel, onAddAll, onClear, disabled, className, children }: {
  staged: number
  total: number
  idleLabel: string
  onAddAll: () => void
  onClear: () => void
  disabled: boolean
  className?: string
  children?: ReactNode
}) {
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <p className="flex-1 min-w-0 truncate text-xs text-muted-foreground tabular-nums">
        {staged > 0 ? <><span className="font-semibold text-foreground">{staged}</span> of {total} in plan</> : idleLabel}
      </p>
      <div className="flex items-center gap-1.5 shrink-0">
        {staged > 0 && (
          <button className={cn(linkClass, 'text-muted-foreground hover:text-foreground')} onClick={onClear} disabled={disabled}>
            Clear
          </button>
        )}
        {staged > 0 && staged < total && <span aria-hidden className="text-xs text-muted-foreground">·</span>}
        {staged < total && (
          <button className={cn(linkClass, 'text-foreground hover:underline')} onClick={onAddAll} disabled={disabled}>
            Add all
          </button>
        )}
      </div>
      {children}
    </div>
  )
}
