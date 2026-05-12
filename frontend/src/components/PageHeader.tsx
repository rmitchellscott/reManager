import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Settings, Unplug, LifeBuoy } from 'lucide-react'

interface PageHeaderProps {
  device: string
  deviceMachine?: string
  firmware?: string
  onSettings: () => void
  onDisconnect: () => void
  onSupport?: () => void
}

export function PageHeader({ device, deviceMachine, firmware, onSettings, onDisconnect, onSupport }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h1 className="text-3xl font-bold text-foreground">reManager</h1>
        <p className="text-muted-foreground">Manage packages on your reMarkable</p>
      </div>
      <div className="flex items-center gap-2 text-sm">
        {device && firmware && (
          <span className="text-muted-foreground">
            {deviceMachine || device} ({firmware})
          </span>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" onClick={onSettings}>
              <Settings className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Settings</TooltipContent>
        </Tooltip>
        {onSupport && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="sm" onClick={onSupport}>
                <LifeBuoy className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Support</TooltipContent>
          </Tooltip>
        )}
        {device && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="sm" onClick={onDisconnect}>
                <Unplug className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Disconnect</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  )
}
