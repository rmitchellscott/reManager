import { type LucideIcon, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

type BannerSeverity = 'destructive' | 'warning' | 'info'

interface BannerProps {
  severity: BannerSeverity
  icon: LucideIcon
  title: string
  onDismiss?: () => void
  children: React.ReactNode
  actions?: React.ReactNode
  className?: string
}

const severityStyles: Record<BannerSeverity, { container: string; icon: string; title: string }> = {
  destructive: {
    container: 'border-destructive/50 dark:border-destructive',
    icon: 'text-destructive',
    title: 'text-destructive',
  },
  warning: {
    container: 'border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950',
    icon: 'text-amber-600 dark:text-amber-400',
    title: 'text-amber-900 dark:text-amber-50',
  },
  info: {
    container: 'border-blue-200 bg-blue-50 dark:border-blue-900 dark:bg-blue-950',
    icon: 'text-blue-600 dark:text-blue-400',
    title: 'text-blue-900 dark:text-blue-50',
  },
}

export function Banner({ severity, icon: Icon, title, onDismiss, children, actions, className }: BannerProps) {
  const styles = severityStyles[severity]

  return (
    <div
      role="alert"
      className={cn(
        'rounded-lg border px-4 py-3 text-sm',
        styles.container,
        className,
      )}
    >
      {onDismiss ? (
        <div className="flex flex-col gap-0.5 h-full">
          <div className="flex gap-3 items-start">
            <Icon className={cn('h-4 w-4 shrink-0 mt-0.5', styles.icon)} />
            <p className={cn('flex-1 min-w-0 font-medium tracking-tight', styles.title)}>{title}</p>
            <Button variant="ghost" size="xs" className="shrink-0 -mt-1 -mr-2" onClick={onDismiss}>
              <X className="h-3 w-3" />
            </Button>
          </div>
          <div className="flex flex-wrap gap-x-4 gap-y-2 flex-1 ml-7">
            <div className="flex-auto self-start min-w-0">{children}</div>
            {actions && (
              <div className="flex items-center gap-2 ml-auto shrink-0 self-end">{actions}</div>
            )}
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-x-4 gap-y-2 h-full">
          <div className="flex gap-3 items-start flex-auto self-start">
            <Icon className={cn('h-4 w-4 shrink-0 mt-0.5', styles.icon)} />
            <div className="flex-1 min-w-0">
              <p className={cn('font-medium tracking-tight', styles.title)}>{title}</p>
              <div className="mt-0.5">{children}</div>
            </div>
          </div>
          {actions && (
            <div className="flex items-center gap-2 ml-auto shrink-0 self-center">{actions}</div>
          )}
        </div>
      )}
    </div>
  )
}
