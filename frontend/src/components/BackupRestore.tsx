import { CheckCircle, Warning } from '@phosphor-icons/react'
import { useEffect, useLayoutEffect, useState, useRef } from 'react'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { operationFailure } from '@/lib/operations'
import { useAppContext } from '@/contexts/AppContext'
import { BackupPhase, type BackupProgress, type RestoreFile } from '@/lib/types'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { formatBytes } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { FilesService, OperationsService, SystemService } from '@bindings/reManager'

const canRevealInFileManager = SystemService.GetPlatformCapabilities()
  .then((capabilities) => capabilities.revealInFileManager)
  .catch(() => false)

interface BackupRestoreDialogProps {
  mode: 'backup' | 'restore' | null
  isConnected: boolean
  onClose: () => void
}

interface FinalStats {
  files?: number
  bytes?: number
  duration?: number
  throughput?: number
  path?: string
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)

  if (hours > 0) return `${hours}h ${minutes % 60}m ${seconds % 60}s`
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`
  return `${seconds}s`
}

function formatThroughput(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`
}

function StatWithShortLabel({ label, shortLabel, value }: { label: string; shortLabel: string; value: string }) {
  const cellRef = useRef<HTMLDivElement>(null)
  const mirrorRef = useRef<HTMLSpanElement>(null)
  const [longLabelFits, setLongLabelFits] = useState(true)

  useLayoutEffect(() => {
    const cell = cellRef.current
    const mirror = mirrorRef.current
    if (!cell || !mirror) return
    const measure = () => setLongLabelFits(mirror.offsetWidth <= cell.clientWidth)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(cell)
    observer.observe(mirror)
    return () => observer.disconnect()
  }, [])

  return (
    <div ref={cellRef} className="relative min-w-0">
      <span ref={mirrorRef} aria-hidden className="invisible absolute left-0 top-0 whitespace-nowrap">
        {label}: {value}
      </span>
      {longLabelFits ? label : shortLabel}: {value}
    </div>
  )
}

export function BackupRestoreDialog({ mode, isConnected, onClose }: BackupRestoreDialogProps) {
  const { runningOperation, startOperation } = useAppContext()
  const [stage, setStage] = useState<'confirm' | 'progress'>('confirm')
  const [progress, setProgress] = useState<BackupProgress | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [errorDetails, setErrorDetails] = useState('')
  const [success, setSuccess] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<RestoreFile | null>(null)
  const [throughput, setThroughput] = useState<number>(0)
  const [elapsedTime, setElapsedTime] = useState<number>(0)
  const [finalStats, setFinalStats] = useState<FinalStats>({})
  const [revealInFileManager, setRevealInFileManager] = useState(false)
  const samplesRef = useRef<Array<{ time: number; bytes: number }>>([])
  const lastThroughputUpdateRef = useRef<number>(0)
  const progressRef = useRef<BackupProgress | null>(null)
  const startTimeRef = useRef<number | null>(null)
  const operationIdRef = useRef<string | null>(null)
  const restoreRequestedRef = useRef(false)
  const running = runningOperation?.kind === 'backup'

  const WINDOW_SIZE_MS = 10000
  const THROUGHPUT_UPDATE_INTERVAL_MS = 1000

  const getPhaseMessage = ({ phase, message }: BackupProgress): string => {
    switch (phase) {
      case BackupPhase.PhaseScanning:
        return 'Scanning files on your reMarkable…'
      case BackupPhase.PhaseDownloading:
        return 'Downloading from your reMarkable…'
      case BackupPhase.PhaseUploading:
        return 'Uploading to your reMarkable…'
      case BackupPhase.PhaseStoppingUI:
      case BackupPhase.PhaseStartingUI:
        return message
      case BackupPhase.PhaseComplete:
        return 'Complete'
      default:
        return phase
    }
  }

  const handleProgress = (p: BackupProgress) => {
    const now = Date.now()
    if (startTimeRef.current === null) {
      startTimeRef.current = now
    }
    setElapsedTime(now - startTimeRef.current)

    samplesRef.current.push({ time: now, bytes: p.bytesDone })

    const cutoff = now - WINDOW_SIZE_MS
    samplesRef.current = samplesRef.current.filter(s => s.time > cutoff)

    if (now - lastThroughputUpdateRef.current >= THROUGHPUT_UPDATE_INTERVAL_MS) {
      if (samplesRef.current.length >= 2) {
        const oldest = samplesRef.current[0]
        const newest = samplesRef.current[samplesRef.current.length - 1]
        const timeDelta = (newest.time - oldest.time) / 1000
        const bytesDelta = newest.bytes - oldest.bytes

        if (timeDelta > 0.5) {
          setThroughput(bytesDelta / timeDelta)
          lastThroughputUpdateRef.current = now
        }
      }
    }

    progressRef.current = p
    setProgress(p)
  }

  const handleComplete = (message: string, path?: string) => {
    const duration = startTimeRef.current ? Date.now() - startTimeRef.current : 0
    const currentProgress = progressRef.current

    setFinalStats({
      files: currentProgress?.filesDone || currentProgress?.filesTotal,
      bytes: currentProgress?.bytesDone,
      duration,
      throughput: duration > 0 && currentProgress?.bytesDone
        ? (currentProgress.bytesDone / (duration / 1000))
        : undefined,
      path,
    })

    setProgress(null)
    setSuccess(message)
  }

  const resetRun = () => {
    setError(null)
    setSuccess(null)
    setProgress(null)
    setThroughput(0)
    setElapsedTime(0)
    setFinalStats({})
    samplesRef.current = []
    lastThroughputUpdateRef.current = 0
    progressRef.current = null
    startTimeRef.current = null
  }

  const run = async (runMode: 'backup' | 'restore', start: (id: string) => Promise<unknown>) => {
    setStage('progress')
    let runId = ''
    let output = ''
    const result = await startOperation({
      kind: 'backup',
      target: runMode,
      title: runMode === 'restore' ? 'Restore backup' : 'Create backup',
      background: true,
      reportsOwnFailure: true,
      onBackupProgress: p => {
        if (operationIdRef.current === runId) handleProgress(p)
      },
      onOutput: text => { output += text },
      run: id => {
        runId = id
        operationIdRef.current = id
        return start(id)
      },
    })
    if (operationIdRef.current !== runId) return
    operationIdRef.current = null
    if (result.ok) {
      handleComplete(runMode === 'restore' ? 'Restore complete.' : 'Backup complete', result.value)
      return
    }
    if (result.canceled) return
    setProgress(null)
    const failure = operationFailure(result)
    setError(handleError(failure, runMode === 'restore' ? 'Could not restore the backup.' : 'Could not create the backup.'))
    setErrorDetails(output.trim() || errorDetail(failure))
  }

  useEffect(() => {
    canRevealInFileManager.then(setRevealInFileManager)
  }, [])

  useEffect(() => {
    if (mode === 'backup') {
      resetRun()
      const selectFile = async () => {
        try {
          const filePath = await FilesService.SelectBackupFile()
          if (filePath) {
            void run('backup', id => FilesService.CreateDeviceBackup(id, filePath))
          } else {
            onClose()
          }
        } catch (err) {
          onClose()
        }
      }
      selectFile()
    } else if (mode === 'restore') {
      resetRun()
      setSelectedFile(null)
      restoreRequestedRef.current = false
      const selectFile = async () => {
        try {
          const file = await FilesService.SelectRestoreFile()
          if (file.path) {
            setSelectedFile(file)
            setStage('confirm')
          } else {
            onClose()
          }
        } catch (err) {
          onClose()
        }
      }
      selectFile()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, onClose])

  useEffect(() => {
    if (!running) return
    const interval = setInterval(() => {
      if (startTimeRef.current) setElapsedTime(Date.now() - startTimeRef.current)
    }, 1000)
    return () => clearInterval(interval)
  }, [running])

  const handleRestoreConfirm = () => {
    if (selectedFile && !restoreRequestedRef.current) {
      restoreRequestedRef.current = true
      resetRun()
      const archivePath = selectedFile.path
      void run('restore', id => FilesService.RestoreDeviceBackup(id, archivePath))
    }
  }

  const handleCancel = () => {
    if (operationIdRef.current) void OperationsService.CancelOperation(operationIdRef.current)
    operationIdRef.current = null
    setProgress(null)
    onClose()
  }

  const handleClose = () => {
    if (!running) {
      onClose()
    }
  }

  const isOperating = running
  const isMeasurable = progress?.phase === BackupPhase.PhaseDownloading || progress?.phase === BackupPhase.PhaseUploading
  const isIndeterminate = !success && !error && !isMeasurable
  const restoreConfirm = mode === 'restore' && stage === 'confirm'

  if (!mode) return null

  return (
    <Dialog open={mode !== null} onOpenChange={(open) => !open && handleClose()}>
      <DialogContent>
        <DialogHeader
          severity={success ? 'success' : restoreConfirm ? 'destructive' : undefined}
          icon={success ? CheckCircle : restoreConfirm ? Warning : undefined}
        >
          <DialogTitle>
            {success
              ? (mode === 'backup' ? 'Backup created' : 'Restore complete')
              : (mode === 'backup' ? 'Creating backup' : 'Restore backup')
            }
          </DialogTitle>
          {restoreConfirm && (
            <DialogDescription>
              Documents and settings in this backup will replace the ones on your reMarkable. Documents added since the backup are kept.
            </DialogDescription>
          )}
        </DialogHeader>

        <div className="space-y-4">
          {mode === 'restore' && stage === 'confirm' && (
            <div className="space-y-3">
              {selectedFile && (
                <div className="text-sm">
                  <div className="font-medium mb-1">Selected backup:</div>
                  <div className="text-muted-foreground truncate">{selectedFile.path}</div>
                </div>
              )}
              {selectedFile?.fromAnotherDevice && (
                <Alert variant="warning">
                  <Warning className="h-4 w-4" />
                  <AlertTitle>Different reMarkable</AlertTitle>
                  <AlertDescription>
                    This backup was created on "{selectedFile.backupDevice || 'Unknown'}" but you're restoring to "{selectedFile.currentDevice || 'Unknown'}".
                  </AlertDescription>
                </Alert>
              )}
              <p className="text-sm text-muted-foreground">
                Your reMarkable will be unresponsive until the restore finishes. If you have a passcode, enter it on your reMarkable afterward.
              </p>
            </div>
          )}

          {stage === 'progress' && (
            <div className="space-y-3">
              <Progress
                indeterminate={isIndeterminate}
                value={success ? 100 : Math.min(99, progress?.percentage || 0)}
                className="h-2"
                aria-label={mode === 'backup' ? 'Creating backup' : 'Restoring backup'}
              />

              {!success && !error && progress && (
                <>
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-medium">{getPhaseMessage(progress)}</span>
                    {isMeasurable && (
                      <span className="text-muted-foreground tabular-nums">{Math.min(99, Math.round(progress.percentage))}%</span>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm text-muted-foreground tabular-nums">
                    {progress.filesDone !== undefined && progress.filesTotal !== undefined && (
                      <div>Files: {progress.filesDone.toLocaleString()} / {progress.filesTotal.toLocaleString()}</div>
                    )}
                    {progress.bytesDone !== undefined && progress.bytesDone > 0 && (
                      <div>Transferred: {formatBytes(progress.bytesDone)}</div>
                    )}
                    {throughput > 0 && (
                      <div>Speed: {formatThroughput(throughput)}</div>
                    )}
                    {elapsedTime > 0 && (
                      <div>Elapsed: {formatDuration(elapsedTime)}</div>
                    )}
                  </div>
                </>
              )}

              {success && (
                <div className="space-y-3">
                  <div className="h-5" aria-hidden />


                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm text-muted-foreground tabular-nums">
                    {finalStats.files !== undefined && finalStats.files > 0 && (
                      <div>Total files: {finalStats.files.toLocaleString()}</div>
                    )}
                    {finalStats.bytes !== undefined && finalStats.bytes > 0 && (
                      <div>Total size: {formatBytes(finalStats.bytes)}</div>
                    )}
                    {finalStats.duration !== undefined && finalStats.duration > 0 && (
                      <div>Duration: {formatDuration(finalStats.duration)}</div>
                    )}
                    {finalStats.throughput !== undefined && finalStats.throughput > 0 && (
                      <StatWithShortLabel label="Average speed" shortLabel="Avg speed" value={formatThroughput(finalStats.throughput)} />
                    )}
                  </div>

                  {mode === 'backup' && revealInFileManager && finalStats.path && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          onClick={() => FilesService.RevealInFileManager(finalStats.path!)}
                          className="text-sm text-primary underline-offset-4 hover:underline text-left truncate w-full"
                        >
                          Saved: {finalStats.path.split(/[/\\]/).pop()}
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>Open folder</TooltipContent>
                    </Tooltip>
                  )}
                </div>
              )}

              {error && (
                <Alert variant="destructive">
                  <Warning className="h-4 w-4" />
                  <AlertDescription className="ml-2">{error}</AlertDescription>
                </Alert>
              )}
              {error && <OutputDisclosure output={errorDetails} />}
            </div>
          )}
        </div>

        <DialogFooter>
          {stage === 'confirm' && mode === 'restore' ? (
            <>
              <Button onClick={onClose} variant="outline" className="flex-1">
                Cancel
              </Button>
              <Button onClick={handleRestoreConfirm} variant="destructive" className="flex-1" disabled={!isConnected}>
                Restore
              </Button>
            </>
          ) : (
            <Button
              onClick={isOperating ? handleCancel : handleClose}
              variant={isOperating ? "destructive" : "default"}
              className="w-full"
            >
              {isOperating ? "Cancel" : "Close"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
