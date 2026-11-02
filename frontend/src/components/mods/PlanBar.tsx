import type { ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { PlanLevel } from '@/lib/modsStatus'

export type { PlanLevel }

export interface PlanBarModel {
  title: ReactNode
  icon?: ReactNode
  titleClassName?: string
  count?: number
  bump: number
  wash: PlanWash
  level: PlanLevel
  detail: ReactNode
  progress?: number
  everyTab: boolean
  done: boolean
  onOpen: () => void
  onDone: () => void
}

export interface PlanWash {
  key: number
  arrival: boolean
}

export const noPlanWash: PlanWash = { key: 0, arrival: false }

export function PlanBarWash({ wash }: { wash: PlanWash }) {
  if (wash.key === 0) return null
  return (
    <span
      key={wash.key}
      aria-hidden
      className={cn(
        'pointer-events-none absolute -inset-px rounded-[inherit] bg-[var(--pending-wash)] opacity-0 shadow-[inset_0_0_0_1px_oklch(from_var(--pending)_l_c_h_/_55%)]',
        wash.arrival ? 'animate-plan-wash' : 'animate-plan-wash-small',
      )}
    />
  )
}

const countBorder: Record<PlanLevel, string> = {
  pending: 'border-[oklch(from_var(--pending)_l_c_h_/_25%)]',
  warning: 'border-[oklch(from_var(--warning)_l_c_h_/_25%)]',
  destructive: 'border-transparent',
}

export function PlanCount({ count, level, bounce = false }: { count: number; level: PlanLevel; bounce?: boolean }) {
  return (
    <Badge
      variant={level}
      className={cn(
        'ml-1.5 inline-flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full border px-[5px] py-0 text-[11px] font-medium leading-none tabular-nums shadow-none',
        countBorder[level],
        bounce && 'motion-safe:animate-count-pop',
      )}
    >
      <span className="block [text-box:trim-both_cap_alphabetic]">{count}</span>
    </Badge>
  )
}

export function PlanBarContent({ model, focusable = true, className }: { model: PlanBarModel; focusable?: boolean; className?: string }) {
  return (
    <div
      onClick={() => model.onOpen()}
      className={cn('relative flex items-center gap-3 text-left cursor-pointer', className)}
    >
      <button type="button" tabIndex={focusable ? undefined : -1} className="min-w-0 flex-1 rounded-sm text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <span className="flex items-center text-sm font-semibold">
          {model.icon && <span className="mr-1.5 shrink-0">{model.icon}</span>}
          <span className={cn('truncate', model.titleClassName)}>{model.title}</span>
          {model.count !== undefined && <PlanCount key={model.bump} count={model.count} level={model.level} bounce={model.bump > 0} />}
        </span>
        <span className="block truncate text-xs leading-[18px] text-muted-foreground">{model.detail}</span>
      </button>
      {model.done ? (
        <Button size="sm" className="shrink-0" tabIndex={focusable ? undefined : -1} onClick={(e) => { e.stopPropagation(); model.onDone() }}>
          Done
        </Button>
      ) : (
        <span aria-hidden className="inline-flex h-9 shrink-0 items-center rounded-md border border-input bg-background px-3 text-sm font-medium">
          Show
        </span>
      )}
      {model.progress !== undefined && (
        <span aria-hidden className="absolute inset-x-0 bottom-0 h-[3px] bg-muted">
          <span className="block h-full bg-primary transition-[width] duration-300 ease-out" style={{ width: `${model.progress}%` }} />
        </span>
      )}
    </div>
  )
}
