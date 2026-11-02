import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'

const STATUS_CONFIG: Record<string, { label: string; tooltip: string }> = {
  deprecated: {
    label: 'Deprecated',
    tooltip: 'This mod will be removed from the catalog.',
  },
  unmaintained: {
    label: 'Unmaintained',
    tooltip: 'This mod no longer gets upgrades and may not work on newer OS versions.',
  },
}

export function StatusBadge({ status, showTooltip = true }: { status: string; showTooltip?: boolean }) {
  const config = STATUS_CONFIG[status]
  if (!config) return null

  const badge = <Badge variant="secondary" className="text-xs">{config.label}</Badge>

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
