import { CaretDown, CaretRight, X } from '@phosphor-icons/react'
import { useMemo, useState } from 'react'
import { TransferKind, TransferState, type TransferSnapshot } from '@/lib/types'
import { formatBytes, formatBytesPair, formatDuration } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { FilesService } from '@bindings/reManager'

export function TransferPanel({ transfers }: { transfers: TransferSnapshot | null }) {
  const [expanded, setExpanded] = useState(false)

  const active = useMemo(
    () => (transfers?.transfers ?? []).filter(t => t.state === TransferState.TransferQueued || t.state === TransferState.TransferStarting || t.state === TransferState.TransferRunning),
    [transfers]
  )

  const finished = useMemo(
    () => (transfers?.transfers ?? []).filter(t => t.state === TransferState.TransferFailed || t.state === TransferState.TransferCanceled),
    [transfers]
  )

  if (active.length === 0 && finished.length === 0) return null

  return (
    <div className="rounded-[var(--radius)] border bg-card p-3 space-y-2">
      {active.length > 0 && transfers && (
        <>
          <div className="flex justify-between items-center text-sm gap-2">
            {active.length === 1 ? (
              <span className="truncate">
                {`${active[0].kind === TransferKind.TransferUpload ? 'Uploading' : 'Downloading'}: ${active[0].name}`}
              </span>
            ) : (
              <button
                className="flex items-center gap-1 min-w-0 hover:underline"
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? <CaretDown className="h-4 w-4 shrink-0" /> : <CaretRight className="h-4 w-4 shrink-0" />}
                <span className="truncate">{`${active.length} transfers`}</span>
              </button>
            )}
            <div className="flex items-center gap-2 shrink-0">
              <span className="tabular-nums text-right min-w-[3.5ch]">
                {active.every(t => t.state !== TransferState.TransferRunning)
                  ? 'Waiting…'
                  : `${transfers.percentage.toFixed(0)}%`}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 w-8 p-0 touch:hit-target"
                onClick={() => FilesService.CancelAllTransfers()}
                aria-label="Cancel all transfers"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <Progress value={transfers.percentage} />
          <div className="text-xs text-muted-foreground tabular-nums">
            {formatBytesPair(transfers.bytesDone, transfers.bytesTotal)}
            {active.length === 1 && active[0].isDir &&
              ` · ${active[0].filesDone} / ${active[0].filesTotal} files`}
            {transfers.bytesPerSecond > 0 && ` · ${formatBytes(transfers.bytesPerSecond)}/s`}
            {transfers.secondsRemaining > 0 && ` · ${formatDuration(transfers.secondsRemaining)} left`}
          </div>
        </>
      )}

      {expanded && active.length > 1 && active.map(transfer => (
        <div key={transfer.id} className="pl-5 space-y-1">
          <div className="flex justify-between items-center text-xs gap-2">
            <span className="truncate">{transfer.name}</span>
            <div className="flex items-center gap-2 shrink-0">
              <span className="tabular-nums text-right min-w-[3.5ch]">
                {transfer.state === TransferState.TransferQueued ? 'Queued' : transfer.state === TransferState.TransferStarting ? 'Waiting…' : `${transfer.percentage.toFixed(0)}%`}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 w-8 p-0 touch:hit-target"
                onClick={() => FilesService.CancelTransfer(transfer.id)}
                aria-label={`Cancel ${transfer.name}`}
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          </div>
          <Progress value={transfer.percentage} className="h-1" />
          <div className="text-xs text-muted-foreground tabular-nums">
            {formatBytesPair(transfer.bytesDone, transfer.bytesTotal)}
            {transfer.isDir && ` · ${transfer.filesDone} / ${transfer.filesTotal} files`}
          </div>
        </div>
      ))}

      {finished.length > 0 && (
        <div className="space-y-1 pt-1">
          {finished.map(transfer => (
            <div key={transfer.id} className="text-xs text-destructive">
              {transfer.name}: {transfer.state === TransferState.TransferCanceled ? 'canceled' : transfer.error || 'failed'}
            </div>
          ))}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => FilesService.ClearCompletedTransfers()}
          >
            Dismiss
          </Button>
        </div>
      )}
    </div>
  )
}
