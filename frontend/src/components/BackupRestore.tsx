import { useEffect, useState, useCallback, useRef } from 'react'
import { handleError } from '@/lib/errorMessages'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { AlertTriangle, Loader2, CheckCircle2 } from 'lucide-react'

interface BackupRestoreDialogProps {
  mode: 'backup' | 'restore' | null
  onClose: () => void
}

interface ProgressData {
  phase: string
  filePath?: string
  filesDone?: number
  filesTotal?: number
  bytesDone?: number
  bytesTotal?: number
  percentage: number
  message: string
}

interface ErrorData {
  message: string
}

interface CompleteData {
  message: string
  path?: string
}

interface FinalStats {
  files?: number
  bytes?: number
  duration?: number
  throughput?: number
  path?: string
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
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

export function BackupRestoreDialog({ mode, onClose }: BackupRestoreDialogProps) {
  const [stage, setStage] = useState<'confirm' | 'progress'>('confirm')
  const [progress, setProgress] = useState<ProgressData | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)
  const [startTime, setStartTime] = useState<number | null>(null)
  const [throughput, setThroughput] = useState<number>(0)
  const [elapsedTime, setElapsedTime] = useState<number>(0)
  const [finalStats, setFinalStats] = useState<FinalStats>({})
  const [deviceMismatch, setDeviceMismatch] = useState<{ backup: string; current: string } | null>(null)
  const samplesRef = useRef<Array<{ time: number; bytes: number }>>([])
  const lastThroughputUpdateRef = useRef<number>(0)
  const progressRef = useRef<ProgressData | null>(null)

  const WINDOW_SIZE_MS = 10000
  const THROUGHPUT_UPDATE_INTERVAL_MS = 1000

  const getPhaseMessage = (phase: string): string => {
    switch (phase) {
      case 'scanning':
        return 'Scanning reMarkable files...'
      case 'downloading':
        return 'Downloading from reMarkable tablet...'
      case 'archiving':
        return 'Creating archive...'
      case 'extracting':
        return 'Extracting backup...'
      case 'uploading':
        return 'Uploading to reMarkable tablet...'
      case 'complete':
        return 'Complete'
      default:
        return phase
    }
  }

  const handleProgress = useCallback((...args: unknown[]) => {
    const p = args[0] as ProgressData
    const now = Date.now()

    if (startTime === null) {
      setStartTime(now)
    }

    setElapsedTime(startTime ? now - startTime : 0)

    if (p.bytesDone !== undefined) {
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
    }

    progressRef.current = p
    setProgress(p)
  }, [startTime])

  const handleComplete = useCallback((...args: unknown[]) => {
    const data = args[0] as CompleteData
    const duration = startTime ? Date.now() - startTime : 0
    const currentProgress = progressRef.current

    setFinalStats({
      files: currentProgress?.filesDone || currentProgress?.filesTotal,
      bytes: currentProgress?.bytesDone,
      duration,
      throughput: duration > 0 && currentProgress?.bytesDone
        ? (currentProgress.bytesDone / (duration / 1000))
        : undefined,
      path: data.path,
    })

    setProgress(null)
    setSuccess(data.message)
  }, [startTime])

  const handleBackupError = useCallback((...args: unknown[]) => {
    const data = args[0] as ErrorData & { code?: string }
    setProgress(null)
    setError(data.code ? handleError(data, 'Backup') : handleError(data.message, 'Backup'))
  }, [])

  useEffect(() => {
    const unsubBackupProgress = window.runtime.EventsOn('backup:progress', handleProgress)
    const unsubBackupComplete = window.runtime.EventsOn('backup:complete', handleComplete)
    const unsubBackupError = window.runtime.EventsOn('backup:error', handleBackupError)
    const unsubRestoreProgress = window.runtime.EventsOn('restore:progress', handleProgress)
    const unsubRestoreComplete = window.runtime.EventsOn('restore:complete', handleComplete)
    const unsubRestoreError = window.runtime.EventsOn('restore:error', handleBackupError)
    const unsubDeviceMismatch = window.runtime.EventsOn('restore:device-mismatch', (...args: unknown[]) => {
      const data = args[0] as { backupDevice: string; currentDevice: string }
      setDeviceMismatch({ backup: data.backupDevice, current: data.currentDevice })
    })

    return () => {
      unsubBackupProgress()
      unsubBackupComplete()
      unsubBackupError()
      unsubRestoreProgress()
      unsubRestoreComplete()
      unsubRestoreError()
      unsubDeviceMismatch()
    }
  }, [handleProgress, handleComplete, handleBackupError])

  // Prompt for file first for both backup and restore
  useEffect(() => {
    if (mode === 'backup') {
      setError(null)
      setSuccess(null)
      setProgress(null)
      setStartTime(null)
      setThroughput(0)
      setElapsedTime(0)
      setFinalStats({})
      samplesRef.current = []
      lastThroughputUpdateRef.current = 0
      progressRef.current = null
      const selectFile = async () => {
        try {
          const filePath = await window.go.main.App.SelectBackupFile()
          if (filePath) {
            setStage('progress')
            window.go.main.App.CreateDeviceBackup(filePath)
          } else {
            onClose()
          }
        } catch (err) {
          onClose()
        }
      }
      selectFile()
    } else if (mode === 'restore') {
      setError(null)
      setSuccess(null)
      setProgress(null)
      setStartTime(null)
      setThroughput(0)
      setElapsedTime(0)
      setFinalStats({})
      setDeviceMismatch(null)
      samplesRef.current = []
      lastThroughputUpdateRef.current = 0
      progressRef.current = null
      setSelectedFile(null)
      const selectFile = async () => {
        try {
          const filePath = await window.go.main.App.SelectRestoreFile()
          if (filePath) {
            setSelectedFile(filePath)
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
  }, [mode, onClose])

  // Update elapsed time every second while operation is in progress
  useEffect(() => {
    if (startTime && progress !== null) {
      const interval = setInterval(() => {
        setElapsedTime(Date.now() - startTime)
      }, 1000)
      return () => clearInterval(interval)
    }
  }, [startTime, progress])

  const handleRestoreConfirm = () => {
    if (selectedFile) {
      setStartTime(null)
      setThroughput(0)
      setElapsedTime(0)
      setFinalStats({})
      samplesRef.current = []
      lastThroughputUpdateRef.current = 0
      progressRef.current = null
      setStage('progress')
      window.go.main.App.RestoreDeviceBackup(selectedFile)
    }
  }

  const handleCancel = () => {
    window.go.main.App.CancelBackup()
    setProgress(null)
    onClose()
  }

  const handleClose = () => {
    if (progress === null) {
      onClose()
    }
  }

  const isOperating = progress !== null

  if (!mode) return null

  return (
    <Dialog open={mode !== null} onOpenChange={(open) => !open && handleClose()}>
      <DialogContent className="w-[500px] max-w-[90vw]">
        <DialogHeader>
          <DialogTitle>
            {success
              ? (mode === 'backup' ? 'Backup Created' : 'Restore Complete')
              : (mode === 'backup' ? 'Creating Backup' : 'Restore Backup')
            }
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {mode === 'restore' && stage === 'confirm' && (
            <div className="space-y-3">
              {selectedFile && (
                <div className="text-sm">
                  <div className="font-medium mb-1">Selected backup:</div>
                  <div className="text-muted-foreground truncate">{selectedFile}</div>
                </div>
              )}
              {deviceMismatch && (
                <Alert className="border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-400">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertTitle className="text-amber-900 dark:text-amber-50">Different Device</AlertTitle>
                  <AlertDescription className="text-muted-foreground">
                    This backup was created on "{deviceMismatch.backup || 'Unknown'}" but you're restoring to "{deviceMismatch.current || 'Unknown'}".
                  </AlertDescription>
                </Alert>
              )}
              <Alert className="border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-400">
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle className="text-amber-900 dark:text-amber-50">Warning</AlertTitle>
                <AlertDescription className="text-muted-foreground">
                  This will overwrite all documents and configurations on your device.
                  Device reboot recommended after restore completes.
                </AlertDescription>
              </Alert>
            </div>
          )}

          {stage === 'progress' && (
            <div className="space-y-3">
              <Progress value={success ? 100 : Math.min(99, progress?.percentage || 0)} className="h-2" />

              {!success && !error && progress && (
                <>
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span className="font-medium">{getPhaseMessage(progress.phase)}</span>
                    </div>
                    <span className="text-muted-foreground">{Math.min(99, Math.round(progress.percentage))}%</span>
                  </div>

                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm text-muted-foreground">
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
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2 font-medium text-green-600 dark:text-green-400">
                      <CheckCircle2 className="h-4 w-4" />
                      <span>Complete</span>
                    </div>
                    <span className="text-muted-foreground">100%</span>
                  </div>

                  <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm text-muted-foreground">
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
                      <div>Avg speed: {formatThroughput(finalStats.throughput)}</div>
                    )}
                  </div>

                  {mode === 'backup' && finalStats.path && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          onClick={() => window.go.main.App.RevealInFileManager(finalStats.path!)}
                          className="text-sm text-primary underline-offset-4 hover:underline text-left truncate w-full"
                        >
                          Saved: {finalStats.path.split(/[/\\]/).pop()}
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>Click to open folder</TooltipContent>
                    </Tooltip>
                  )}
                </div>
              )}

              {error && (
                <Alert variant="destructive">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertDescription className="ml-2">{error}</AlertDescription>
                </Alert>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          {stage === 'confirm' && mode === 'restore' ? (
            <>
              <Button onClick={onClose} variant="outline" className="flex-1">
                Cancel
              </Button>
              <Button onClick={handleRestoreConfirm} variant="destructive" className="flex-1">
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
