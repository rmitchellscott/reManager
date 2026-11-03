import { ArrowsClockwise, Bug, Chat, Check, CircleNotch, Copy, Package } from '@phosphor-icons/react'
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { formatDate } from '@/lib/format'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { SupportService } from '@bindings/reManager'
import type { BundleRecord } from '@/lib/types'
import { operationFailure, runOperation } from '@/lib/operations'

export type SupportScreen = 'report' | 'chat' | 'bundles'

interface SupportHomeProps {
  active: boolean
  supportPageOpen: boolean
  connectedDeviceId: string
  supportUpload: boolean
  onOpen: (screen: SupportScreen) => void
}

const CHOICES: { screen: SupportScreen; label: string; detail: string; Icon: typeof Bug }[] = [
  { screen: 'report', label: 'Report a problem', detail: 'Let the developer know what went wrong', Icon: Bug },
  { screen: 'chat', label: 'Community chat', detail: 'Get help from other users', Icon: Chat },
  { screen: 'bundles', label: 'Support bundles', detail: 'Generate & manage diagnostics', Icon: Package },
]

export function SupportHome({ active, supportPageOpen, connectedDeviceId, supportUpload, onOpen }: SupportHomeProps) {
  const [bundles, setBundles] = useState<BundleRecord[]>([])
  const [copied, setCopied] = useState(false)
  const [appending, setAppending] = useState(false)
  const [appendError, setAppendError] = useState<{ message: string; detail: string } | null>(null)

  const loadBundles = useCallback(async () => {
    const records = await SupportService.GetBundleHistory()
    setBundles(records || [])
  }, [])

  useEffect(() => {
    if (active && !supportPageOpen) loadBundles()
  }, [active, supportPageOpen, loadBundles])

  const handleAppend = async (remoteId: string) => {
    setAppendError(null)
    setAppending(true)
    const result = await runOperation(id => SupportService.AppendSupportBundle(id, remoteId, connectedDeviceId))
    setAppending(false)
    if (result.ok) {
      loadBundles()
      toast.success('Bundle updated')
      return
    }
    if (result.canceled) return
    const failure = operationFailure(result)
    setAppendError({ message: handleError(failure, 'Could not update this bundle.'), detail: errorDetail(failure) })
  }

  const choices = supportUpload ? CHOICES : CHOICES.filter(c => c.screen !== 'report')
  const activeBundles = supportUpload ? bundles.filter(b => !b.expiresAt || b.expiresAt > Date.now() / 1000) : []
  const latest = (connectedDeviceId
    ? activeBundles.find(b => b.deviceId === connectedDeviceId)
    : activeBundles[0]) || activeBundles[0]

  return (
    <div className="space-y-4">
      <div className={`grid grid-cols-1 gap-3 ${choices.length === 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'}`}>
        {choices.map(({ screen, label, detail, Icon }) => (
          <button
            key={screen}
            type="button"
            onClick={() => onOpen(screen)}
            className="relative flex items-center gap-3 rounded-lg border px-4 py-3 text-left transition-colors touch:hit-target hover:border-ring hover:bg-accent md:flex-col md:gap-3 md:py-5 md:text-center"
          >
            <Icon className="h-7 w-7 shrink-0 text-muted-foreground md:h-8 md:w-8" aria-hidden />
            <span className="min-w-0">
              <span className="block text-sm font-medium">{label}</span>
              <span className="block text-xs text-muted-foreground">{detail}</span>
            </span>
            {screen === 'bundles' && activeBundles.length > 0 && (
              <Badge className="absolute right-2 top-2 flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-[0.625rem]">
                {activeBundles.length}
              </Badge>
            )}
          </button>
        ))}
      </div>

      {latest && (
        <div className="flex items-center gap-3 touch:gap-4 rounded-lg border p-3.5">
          <Package className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{latest.deviceName || 'Unknown device'}</div>
            <div className="text-xs text-muted-foreground">
              Bundle last updated {formatDate(latest.lastAppended || latest.uploadedAt)}
            </div>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8 shrink-0 touch:hit-target"
                aria-label="Copy bundle link"
                onClick={() => {
                  SupportService.CopyToClipboard(latest.url)
                  setCopied(true)
                  setTimeout(() => setCopied(false), 2000)
                }}
              >
                {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>Copy link</TooltipContent>
          </Tooltip>
          <Button
            size="sm"
            className="shrink-0 touch:hit-target"
            disabled={appending}
            onClick={() => handleAppend(latest.remoteId)}
          >
            {appending
              ? <CircleNotch className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              : <ArrowsClockwise className="mr-1.5 h-3.5 w-3.5" />}
            Update bundle
          </Button>
        </div>
      )}

      {appendError && (
        <>
          <p className="text-xs text-destructive">{appendError.message}</p>
          <OutputDisclosure output={appendError.detail} />
        </>
      )}
    </div>
  )
}
