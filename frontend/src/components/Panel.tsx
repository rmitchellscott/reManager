import { Check } from '@phosphor-icons/react'
import { cloneElement, createContext, isValidElement, useContext } from 'react'
import { cn } from '@/lib/utils'

export const SEVERITY_TEXT = {
  info: 'text-[var(--info-text)]',
  warning: 'text-[var(--warning-text)]',
  destructive: 'text-destructive',
} as const

export type PanelSeverity = keyof typeof SEVERITY_TEXT

export const panelControl = 'min-h-9 h-auto py-1.5 px-3 text-sm touch:hit-target'

export const panelAction = `${panelControl} min-w-20 whitespace-normal`

export function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="mb-2.5 text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground">
      {children}
    </h3>
  )
}

const PanelContext = createContext({ controlColumn: false, divided: true })

export function PanelGroup({ controlColumn = false, divided = true, className, children }: {
  controlColumn?: boolean
  divided?: boolean
  className?: string
  children?: React.ReactNode
}) {
  return (
    <PanelContext.Provider value={{ controlColumn, divided }}>
      <div className={cn(divided ? 'divide-y' : 'space-y-5', className)}>{children}</div>
    </PanelContext.Provider>
  )
}

export function StepMarker({ position, done }: { position: number; done: boolean }) {
  return done ? (
    <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground">
      <Check className="h-3 w-3" weight="bold" />
      <span className="sr-only">Step {position}, done</span>
    </span>
  ) : (
    <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full border text-[11px] font-semibold tabular-nums">
      {position}
      <span className="sr-only">, to do</span>
    </span>
  )
}

export function PanelRow({ label, detail, severity, htmlFor, marker, expansion, children }: {
  label: string
  detail?: React.ReactNode
  severity?: PanelSeverity | null
  htmlFor?: string
  marker?: React.ReactNode
  expansion?: React.ReactNode
  children?: React.ReactNode
}) {
  const { controlColumn, divided } = useContext(PanelContext)
  const labelClass = 'text-sm font-medium'
  const detailId = htmlFor && detail ? `${htmlFor}-detail` : undefined

  // Radix roots that render no element of their own drop the prop, so a select has to set it on its trigger instead.
  const control = detailId && isValidElement(children)
    ? cloneElement(children as React.ReactElement<{ 'aria-describedby'?: string }>, { 'aria-describedby': detailId })
    : children

  return (
    <div className={cn(divided && 'py-3 first:pt-0 last:pb-0')}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          {marker}
          <div className="min-w-0">
            {htmlFor
              ? <label htmlFor={htmlFor} className={labelClass}>{label}</label>
              : <p className={labelClass}>{label}</p>}
            {detail && (
              <p id={detailId} className={cn('text-xs leading-5 break-words', severity ? SEVERITY_TEXT[severity] : 'text-muted-foreground')}>
                {detail}
              </p>
            )}
          </div>
        </div>
        {control && (
          <div className={cn('flex shrink-0 items-center', controlColumn && 'sm:w-40 sm:justify-end')}>
            {control}
          </div>
        )}
      </div>
      {expansion && <div className="mt-3">{expansion}</div>}
    </div>
  )
}
