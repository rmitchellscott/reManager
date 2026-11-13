import { ReactNode } from 'react'

// Sticky, not fixed, since fixed would cover the floating tab bar; the band below is a box-shadow rather than reserved padding, since a positioned box there would extend scrollHeight.
export function StickyActionBar({ children, status, panel }: { children: ReactNode; status?: ReactNode; panel?: ReactNode }) {
  return (
    <div
      className="sticky z-20 bottom-0
                 -ml-gutter-left -mr-gutter-right pl-gutter-left pr-gutter-right py-3 flex flex-col gap-3 edge-rule-t bg-background
                 sm:flex-row sm:flex-wrap sm:items-center"
      style={{ boxShadow: '0 100px 0 100px var(--background)', clipPath: 'inset(0 0 -200px 0)' }}
    >
      {status && <div className="sm:flex-1 sm:min-w-0">{status}</div>}
      {panel && <div className="sm:order-last sm:basis-full">{panel}</div>}
      <div className="flex gap-2 sm:ml-auto sm:shrink-0">{children}</div>
    </div>
  )
}
