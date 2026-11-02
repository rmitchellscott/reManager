import { CaretUpDown, Check } from '@phosphor-icons/react'
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { TakeoverPage } from '@/components/TakeoverPage'
import { useTouchLayout } from '@/hooks/useMediaQuery'
import { IANA_TIMEZONES } from '@/lib/timezones'
import { panelControl } from '@/components/Panel'

interface TakeoverChrome {
  device: string
  deviceMachine?: string
  firmware?: string
  onSettings: () => void
  onDisconnect: () => void
}

interface TimezoneComboboxProps {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  takeover?: TakeoverChrome
}

function TimezoneList({ value, onPick, className, listClassName }: {
  value: string
  onPick: (value: string) => void
  className?: string
  listClassName?: string
}) {
  return (
    <Command className={className}>
      <CommandInput placeholder="Search timezone…" />
      <CommandList className={listClassName} data-touch-scroll>
        <CommandEmpty>No timezone found.</CommandEmpty>
        <CommandGroup>
          {IANA_TIMEZONES.map((tz) => (
            <CommandItem
              key={tz}
              value={tz}
              onSelect={(currentValue) => onPick(currentValue === value ? "" : currentValue)}
            >
              <Check
                className={cn(
                  "mr-2 h-4 w-4",
                  value === tz ? "opacity-100" : "opacity-0"
                )}
              />
              {tz}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  )
}

export function TimezoneCombobox({ value, onChange, disabled, takeover }: TimezoneComboboxProps) {
  const [open, setOpen] = useState(false)
  const fullScreen = useTouchLayout() && !!takeover

  const pick = (next: string) => {
    onChange(next)
    setOpen(false)
  }

  const trigger = (
    <Button
      variant="outline"
      role="combobox"
      aria-expanded={open}
      className={`w-full justify-between ${panelControl}`}
      disabled={disabled}
      onClick={fullScreen ? () => setOpen(true) : undefined}
    >
      <span className="truncate">{value || "Select timezone…"}</span>
      <CaretUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
    </Button>
  )

  if (fullScreen) {
    return (
      <>
        {trigger}
        {open && createPortal(
          <TakeoverPage
            title="Timezone"
            narrowTitle="Timezone"
            device={takeover.device}
            deviceMachine={takeover.deviceMachine}
            firmware={takeover.firmware}
            onSettings={takeover.onSettings}
            onDisconnect={takeover.onDisconnect}
            onClose={() => setOpen(false)}
            fill
          >
            <TimezoneList
              value={value}
              onPick={pick}
              className="min-h-0 flex-1 bg-background"
              listClassName="max-h-none flex-1"
            />
          </TakeoverPage>,
          document.body,
        )}
      </>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        {trigger}
      </PopoverTrigger>
      <PopoverContent className="w-[300px] p-0">
        <TimezoneList value={value} onPick={pick} />
      </PopoverContent>
    </Popover>
  )
}
