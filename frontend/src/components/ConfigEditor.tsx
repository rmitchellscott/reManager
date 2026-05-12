import { useState, useEffect, useCallback } from 'react'
import { handleError } from '@/lib/errorMessages'
import { formatBytes, formatDate } from '@/lib/format'
import Editor from '@monaco-editor/react'
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
import { Save, FolderDown, History, CheckCircle2, AlertCircle, AlertTriangle } from 'lucide-react'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'

interface ConfigEditorProps {
  isConnected: boolean
  theme?: 'dark' | 'light'
}

interface BackupInfo {
  name: string
  timestamp: number
  size: number
}

export function ConfigEditor({ isConnected, theme }: ConfigEditorProps) {
  const [content, setContent] = useState("")
  const [originalContent, setOriginalContent] = useState("")
  const [isSaving, setIsSaving] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [backups, setBackups] = useState<BackupInfo[]>([])
  const [isDirty, setIsDirty] = useState(false)
  const [showSuccess, setShowSuccess] = useState(false)
  const [showError, setShowError] = useState(false)
  const [errorMessage, setErrorMessage] = useState("")
  const [backupsOpen, setBackupsOpen] = useState(false)
  const isDark = theme === 'dark'
  const [editorHeight, setEditorHeight] = useState(500)

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
      const fileContent = await window.go.main.App.ReadConfigFile()
      setContent(fileContent)
      setOriginalContent(fileContent)
      setIsDirty(false)
      validateContent(fileContent)
    } catch (err) {
      setErrorMessage(handleError(err, 'Load config'))
      setShowError(true)
    }
  }, [isConnected, validateContent])

  const saveConfig = useCallback(async () => {
    if (!validateContent(content)) {
      setErrorMessage('Cannot save: Invalid INI syntax')
      setShowError(true)
      return
    }

    setIsSaving(true)
    try {
      await window.go.main.App.WriteConfigFile(content)
      setOriginalContent(content)
      setIsDirty(false)
      setShowSuccess(true)
      setTimeout(() => setShowSuccess(false), 3000)
    } catch (err) {
      setErrorMessage(handleError(err, 'Save config'))
      setShowError(true)
    } finally {
      setIsSaving(false)
    }
  }, [content, validateContent])

  const createBackup = useCallback(async () => {
    if (!isConnected) return

    try {
      await window.go.main.App.BackupConfigFile()
      await loadBackups()
      setShowSuccess(true)
      setTimeout(() => setShowSuccess(false), 3000)
    } catch (err) {
      setErrorMessage(handleError(err, 'Create backup'))
      setShowError(true)
    }
  }, [isConnected])

  const restoreBackup = useCallback(async (backupName: string) => {
    setBackupsOpen(false)
    try {
      await window.go.main.App.RestoreConfigBackup(backupName)
      await loadConfig()
      setShowSuccess(true)
      setTimeout(() => setShowSuccess(false), 3000)
    } catch (err) {
      setErrorMessage(handleError(err, 'Restore backup'))
      setShowError(true)
    }
  }, [loadConfig])

  const loadBackups = useCallback(async () => {
    if (!isConnected) return

    try {
      const backupList = await window.go.main.App.ListConfigBackups()
      setBackups(backupList)
    } catch (err) {
      console.error('Failed to load backups:', err)
    }
  }, [isConnected])

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      if (isDirty && !isSaving && isConnected) {
        saveConfig()
      }
    }
  }, [isDirty, isSaving, isConnected, saveConfig])

  useEffect(() => {
    setIsDirty(content !== originalContent)
  }, [content, originalContent])

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

  // Auto-load config when component mounts or connection is established
  useEffect(() => {
    if (isConnected) {
      loadConfig()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isConnected])


  useEffect(() => {
    const calculateHeight = () => {
      const baseOffset = 280
      const alertHeight = content ? 100 : 0
      const validationErrorHeight = validationError ? 80 : 0
      const dirtyHintHeight = isDirty ? 30 : 0
      const totalOffset = baseOffset + alertHeight + validationErrorHeight + dirtyHintHeight
      const availableHeight = window.innerHeight - totalOffset
      setEditorHeight(Math.max(300, availableHeight))
    }

    calculateHeight()
    window.addEventListener('resize', calculateHeight)
    return () => window.removeEventListener('resize', calculateHeight)
  }, [content, validationError, isDirty])

  return (
    <div className="space-y-4">
      {/* Warning banner */}
      {content && (
        <Alert className="border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950 [&>svg]:text-amber-600 dark:[&>svg]:text-amber-400">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle className="text-amber-900 dark:text-amber-50">System Configuration File</AlertTitle>
          <AlertDescription className="text-muted-foreground">
            <p>Incorrect changes may cause issues. Create a backup before making changes.</p>
            <p>Restart xochitl or reboot to apply changes.</p>
          </AlertDescription>
        </Alert>
      )}

      {/* Toolbar */}
      <div className="flex gap-2 flex-wrap items-center">
        <Button
          onClick={saveConfig}
          disabled={!isConnected || !isDirty || isSaving || validationError !== null}
          size="sm"
        >
          <Save className="h-4 w-4 mr-2" />
          Save{isDirty && " *"}
        </Button>
        <Button
          onClick={createBackup}
          disabled={!isConnected}
          size="sm"
          variant="outline"
        >
          <FolderDown className="h-4 w-4 mr-2" />
          Backup
        </Button>

        <Popover open={backupsOpen} onOpenChange={setBackupsOpen}>
          <PopoverTrigger asChild>
            <Button
              disabled={!isConnected || !backups || backups.length === 0}
              size="sm"
              variant="outline"
            >
              <History className="h-4 w-4 mr-2" />
              Restore ({backups?.length ?? 0})
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[400px] p-0" align="start">
            <Command>
              <CommandList>
                <CommandEmpty>No backups found</CommandEmpty>
                <CommandGroup heading="Available Backups">
                  {backups?.map((backup) => (
                    <CommandItem
                      key={backup.name}
                      onSelect={() => restoreBackup(backup.name)}
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
          <div className="flex items-center gap-2 text-sm text-green-600 dark:text-green-400">
            <CheckCircle2 className="h-4 w-4" />
            Success
          </div>
        )}
      </div>

      {/* Validation error display */}
      {validationError && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Invalid INI syntax</AlertTitle>
          <AlertDescription className="text-muted-foreground">{validationError}</AlertDescription>
        </Alert>
      )}

      {/* Editor */}
      <div className="border rounded-md">
        <Editor
          height={`${editorHeight}px`}
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

      {/* Keyboard shortcut hint */}
      {isDirty && (
        <div className="text-xs text-muted-foreground">
          Press Ctrl+S (or Cmd+S) to save
        </div>
      )}

      {/* Error dialog */}
      <Dialog open={showError} onOpenChange={setShowError}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-red-500" />
              Error
            </DialogTitle>
            <DialogDescription>{errorMessage}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setShowError(false)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
