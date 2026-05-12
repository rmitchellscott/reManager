import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'

const STATUS_CONFIG: Record<string, { label: string; tooltip: string }> = {
  deprecated: {
    label: 'Deprecated',
    tooltip: 'This package will be removed in the future',
  },
  unmaintained: {
    label: 'Unmaintained',
    tooltip: 'This package is no longer receiving updates and may not be available on newer OS versions',
  },
}

export function StatusBadge({ status, showTooltip = true }: { status: string; showTooltip?: boolean }) {
  const config = STATUS_CONFIG[status]
  if (!config) return null

  const badge = <Badge variant="default" className="text-xs">{config.label}</Badge>

  if (!showTooltip) return badge

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{badge}</span>
      </TooltipTrigger>
      <TooltipContent>{config.tooltip}</TooltipContent>
    </Tooltip>
  )
}
