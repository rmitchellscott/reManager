import { CheckCircle, ClockCounterClockwise, DownloadSimple, FloppyDisk, Warning, WarningCircle } from '@phosphor-icons/react'
import { useState, useEffect, useCallback } from 'react'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { formatBytes, formatDate } from '@/lib/format'
import Editor, { loader } from '@monaco-editor/react'
import 'monaco-editor/esm/vs/editor/edcore.main.js'
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js'
import 'monaco-editor/esm/vs/basic-languages/ini/ini.contribution.js'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker.js?worker'
import ini from 'ini'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'
import { ConfigService } from '@bindings/reManager'
import { useAppContext } from '@/contexts/AppContext'
import { operationFailure } from '@/lib/operations'
import type { BackupInfo, OperationResult } from '@/lib/types'

self.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}
loader.config({ monaco })

interface ConfigEditorProps {
  isConnected: boolean
  theme?: 'dark' | 'light'
  onDirtyChange: (dirty: boolean) => void
}

// Disposing the Monaco editor within 50ms of a keystroke cancels its queued word-highlight run, whose rejection it never catches, so closing would otherwise log an unhandled CancellationError.
window.addEventListener('unhandledrejection', (event) => {
  if (event.reason instanceof Error && event.reason.name === 'Canceled') event.preventDefault()
})

export function ConfigEditor({ isConnected, theme, onDirtyChange }: ConfigEditorProps) {
  const { commandRunning, startOperation } = useAppContext()
  const [content, setContent] = useState("")
  const [originalContent, setOriginalContent] = useState("")
  const [loadCount, setLoadCount] = useState(0)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [backups, setBackups] = useState<BackupInfo[]>([])
  const [showSuccess, setShowSuccess] = useState(false)
  const [showError, setShowError] = useState(false)
  const [errorTitle, setErrorTitle] = useState("")
  const [errorMessage, setErrorMessage] = useState("")
  const [errorDetails, setErrorDetails] = useState("")
  const [backupsOpen, setBackupsOpen] = useState(false)
  const [showSaveConfirm, setShowSaveConfirm] = useState(false)
  const [restoreOverEdits, setRestoreOverEdits] = useState<string | null>(null)
  const isDirty = content !== originalContent
  const isDark = theme === 'dark'

  const validateContent = useCallback((text: string): boolean => {
    if (!text || text.trim() === "") {
      setValidationError(null)
      return true
    }

    try {
      ini.parse(text)
      setValidationError(null)
      return true
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Invalid INI syntax'
      setValidationError(message)
      return false
    }
  }, [])

  const loadConfig = useCallback(async () => {
    if (!isConnected) return

    try {
      const fileContent = await ConfigService.ReadConfigFile()
      setContent(fileContent)
      setOriginalContent(fileContent)
      setLoadCount(count => count + 1)
      validateContent(fileContent)
    } catch (err) {
      setErrorTitle('Could not load xochitl.conf')
      setErrorMessage(handleError(err, ''))
      setErrorDetails(errorDetail(err))
      setShowError(true)
    }
  }, [isConnected, validateContent])

  const requestSave = useCallback(() => {
    if (!validateContent(content)) {
      setErrorTitle('Could not save xochitl.conf')
      setErrorMessage('Cannot save: invalid INI syntax.')
      setErrorDetails('')
      setShowError(true)
      return
    }
    setShowSaveConfirm(true)
  }, [content, validateContent])

  const saveConfig = useCallback(async () => {
    setShowSaveConfirm(false)
    if (!validateContent(content)) {
      setErrorTitle('Could not save xochitl.conf')
      setErrorMessage('Cannot save: invalid INI syntax.')
      setErrorDetails('')
      setShowError(true)
      return
    }

    const result = await startOperation({
      kind: 'config',
      target: 'save',
      title: 'Save xochitl.conf',
      background: true,
      reportsOwnFailure: true,
      run: id => ConfigService.WriteConfigFile(id, content),
    })
    if (!result.ok) {
      showOperationError('Could not save xochitl.conf', result)
      return
    }
    setOriginalContent(content)
    setShowSuccess(true)
    setTimeout(() => setShowSuccess(false), 3000)
  }, [content, validateContent, startOperation])

  const showOperationError = (title: string, result: OperationResult) => {
    const failure = operationFailure(result)
    setErrorTitle(title)
    setErrorMessage(handleError(failure, ''))
    setErrorDetails(errorDetail(failure))
    setShowError(true)
  }

  const createBackup = useCallback(async () => {
    if (!isConnected) return

    const result = await startOperation({
      kind: 'config',
      target: 'backup',
      title: 'Back up xochitl.conf',
      background: true,
      reportsOwnFailure: true,
      run: id => ConfigService.BackupConfigFile(id),
    })
    if (!result.ok) {
      showOperationError('Could not create backup', result)
      return
    }
    await loadBackups()
    setShowSuccess(true)
    setTimeout(() => setShowSuccess(false), 3000)
  }, [isConnected, startOperation])

  const restoreBackup = useCallback(async (backupName: string) => {
    setBackupsOpen(false)
    setRestoreOverEdits(null)
    const result = await startOperation({
      kind: 'config',
      target: 'restore',
      title: 'Restore xochitl.conf',
      background: true,
      reportsOwnFailure: true,
      run: id => ConfigService.RestoreConfigBackup(id, backupName),
    })
    if (!result.ok) {
      showOperationError('Could not restore backup', result)
      return
    }
    await loadConfig()
    setShowSuccess(true)
    setTimeout(() => setShowSuccess(false), 3000)
  }, [loadConfig, startOperation])

  const loadBackups = useCallback(async () => {
    if (!isConnected) return

    try {
      const backupList = await ConfigService.ListConfigBackups()
      setBackups(backupList || [])
    } catch (err) {
      console.error('Failed to load backups:', err)
    }
  }, [isConnected])

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      if (isDirty && !commandRunning && isConnected) {
        requestSave()
      }
    }
  }, [isDirty, commandRunning, isConnected, requestSave])

  useEffect(() => {
    onDirtyChange(isDirty)
  }, [isDirty, onDirtyChange])

  useEffect(() => {
    if (content) {
      validateContent(content)
    }
  }, [content, validateContent])

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  useEffect(() => {
    if (isConnected) {
      loadBackups().catch(err => {
        console.error('Failed to load backups on mount:', err)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isConnected])

  useEffect(() => {
    if (isConnected && !isDirty) {
      loadConfig()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isConnected])


  return (
    <div className="flex min-h-0 flex-1 flex-col gap-[16px]">
      <div className="flex flex-wrap items-center gap-[8px]">
        <Button
          onClick={requestSave}
          disabled={!isConnected || !isDirty || commandRunning || validationError !== null}
          size="sm"
        >
          <FloppyDisk className="h-4 w-4 mr-2" />
          Save{isDirty && " *"}
        </Button>
        <Button
          onClick={createBackup}
          disabled={!isConnected || commandRunning}
          size="sm"
          variant="outline"
        >
          <DownloadSimple className="h-4 w-4 mr-2" />
          Backup
        </Button>

        <Popover open={backupsOpen} onOpenChange={setBackupsOpen}>
          <PopoverTrigger asChild>
            <Button
              disabled={!isConnected || commandRunning || !backups || backups.length === 0}
              size="sm"
              variant="outline"
            >
              <ClockCounterClockwise className="h-4 w-4 mr-2" />
              Restore ({backups?.length ?? 0})
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[400px] p-0" align="start">
            <Command>
              <CommandList>
                <CommandEmpty>No backups yet.</CommandEmpty>
                <CommandGroup heading="Available backups">
                  {backups?.map((backup) => (
                    <CommandItem
                      key={backup.name}
                      onSelect={() => {
                        if (!isDirty) {
                          void restoreBackup(backup.name)
                          return
                        }
                        setBackupsOpen(false)
                        setRestoreOverEdits(backup.name)
                      }}
                      className="flex flex-col items-start py-3"
                    >
                      <div className="font-medium">{backup.name}</div>
                      <div className="text-xs text-muted-foreground">
                        {formatDate(backup.timestamp)} • {formatBytes(backup.size)}
                      </div>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {showSuccess && (
          <div className="flex items-center gap-2 text-sm text-[var(--success-text)]">
            <CheckCircle className="h-4 w-4" />
            Done
          </div>
        )}
      </div>

      {validationError && (
        <Alert variant="destructive">
          <WarningCircle className="h-4 w-4" />
          <AlertTitle>Invalid INI syntax</AlertTitle>
          <AlertDescription className="text-muted-foreground">{validationError}</AlertDescription>
        </Alert>
      )}

      <div className="min-h-0 flex-1 overflow-hidden rounded-[var(--radius)] border">
        {/* key={loadCount} remounts a fresh editor per load so Monaco's undo history starts at the loaded file rather than stepping back past it. */}
        <Editor
          key={loadCount}
          height="100%"
          defaultLanguage="ini"
          value={content}
          onChange={(value) => setContent(value || '')}
          theme={isDark ? 'vs-dark' : 'light'}
          options={{
            readOnly: !isConnected,
            minimap: { enabled: false },
            fontSize: 13,
            fontFamily: '"Fira Code", "Cascadia Code", "Consolas", monospace',
            lineNumbers: 'on',
            scrollBeyondLastLine: false,
            automaticLayout: true,
            padding: { top: 12, bottom: 12 },
            fixedOverflowWidgets: true,
          }}
        />
      </div>

      {isDirty && (
        <div className="text-xs text-muted-foreground">
          Press Ctrl+S (or Cmd+S) to save
        </div>
      )}

      <Dialog open={showSaveConfirm} onOpenChange={setShowSaveConfirm}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Save xochitl.conf?</DialogTitle>
            <DialogDescription>
              This is a system configuration file.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 text-sm text-muted-foreground">
            <ul className="ml-5 list-disc space-y-1">
              <li>Incorrect values can stop the reMarkable UI from starting.</li>
              <li>Changes apply after the reMarkable UI restarts or your reMarkable reboots.</li>
              {(backups?.length ?? 0) === 0 && <li>There is no backup yet. Cancel and use Backup first.</li>}
            </ul>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSaveConfirm(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={saveConfig} disabled={!isConnected}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={restoreOverEdits !== null} onOpenChange={(open) => !open && setRestoreOverEdits(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Restore {restoreOverEdits}?</DialogTitle>
            <DialogDescription>
              Your unsaved edits to xochitl.conf will be replaced.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestoreOverEdits(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => restoreOverEdits && restoreBackup(restoreOverEdits)}
              disabled={!isConnected || commandRunning}
            >
              Restore
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showError} onOpenChange={setShowError}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={WarningCircle}>
            <DialogTitle>{errorTitle}</DialogTitle>
            <DialogDescription>{errorMessage}</DialogDescription>
          </DialogHeader>
          <OutputDisclosure output={errorDetails} />
          <DialogFooter>
            <Button onClick={() => setShowError(false)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
