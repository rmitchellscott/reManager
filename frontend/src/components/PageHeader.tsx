import { Eject, Gear } from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
interface PageHeaderProps {
  device: string
  deviceMachine?: string
  firmware?: string
  onSettings: () => void
  onDisconnect: () => void
  nav?: ReactNode
  narrowTitle?: string
  subtitle?: string
  exitLabel?: string
}

const iconButton = 'max-md:h-[40px] max-md:w-[40px] max-md:p-0 md:h-[36px] md:px-[12px]'
const iconGlyph = 'h-[16px] w-[16px]'

export const pageTitle = 'text-[24px] font-bold leading-[32px] md:text-[30px] md:leading-[36px]'

export function PageHeader({ device, deviceMachine, firmware, onSettings, onDisconnect, nav, narrowTitle, subtitle, exitLabel }: PageHeaderProps) {
  const connected = Boolean(device && firmware)
  const subtitleText = subtitle || (connected ? deviceMachine || device : '')
  const exitText = exitLabel || 'Disconnect'

  return (
    <div>
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className={cn(pageTitle, 'text-foreground')}>
            <span className="hidden md:inline">reManager</span>
            <span className="md:hidden">
              {narrowTitle || (connected ? deviceMachine || device : 'reManager')}
            </span>
          </h1>
          {subtitleText && (
            <p className="hidden truncate text-[14px] leading-[20px] text-muted-foreground md:block">
              {subtitleText}
            </p>
          )}
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-1 text-sm md:gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="sm" className={iconButton} onClick={() => onSettings()} aria-label="Settings">
                <Gear className={iconGlyph} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Settings</TooltipContent>
          </Tooltip>
          {(device || exitLabel) && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="sm" className={iconButton} onClick={onDisconnect} aria-label={exitText}>
                  <Eject className={iconGlyph} />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{exitText}</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>

      <div className="flex items-center justify-between gap-3 md:mt-3 md:grid md:grid-cols-[1fr_auto_1fr]">
        <div className="hidden md:block" />
        <div className="md:justify-self-center">{nav}</div>
      </div>
    </div>
  )
}
