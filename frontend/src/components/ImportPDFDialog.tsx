import { CaretDown, CaretUp, CircleNotch, Plus, UploadSimple, X } from '@phosphor-icons/react'
import { useState, useEffect, useCallback, useRef } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { hasTouchPointer } from '@/hooks/useMediaQuery'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { formatBytes } from '@/lib/format'
import { handleError } from '@/lib/errorMessages'
import { FilesService } from '@bindings/reManager'
import { onEvent, type EventData } from '@/lib/events'
import { useAppContext } from '@/contexts/AppContext'
import { operationFailure } from '@/lib/operations'
import { InstallStatus } from '@/lib/types'

interface StagedFile {
  id: string
  path: string
  filename: string
  editedName: string
  size: number
  pageCount: number
  pageCountOverride: number | null
  coverPage: 'first' | 'none'
  fileType: 'pdf' | 'epub' | 'rmdoc'
  status: 'staged' | 'uploading' | 'done' | 'error'
  error?: string
}

interface ImportPDFDialogProps {
  open: boolean
  isConnected: boolean
  hasLibrarian: boolean
  xoviActive: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportPDFDialog({ open, isConnected, hasLibrarian, xoviActive, onOpenChange }: ImportPDFDialogProps) {
  const { commandRunning, runningOperation, startOperation } = useAppContext()
  const canRescanLibrary = hasLibrarian && xoviActive
  const [files, setFiles] = useState<StagedFile[]>([])
  const [restartXochitl, setRestartXochitl] = useState(true)
  const importing = runningOperation?.kind === 'import'
  const [isDragging, setIsDragging] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameInputRefs = useRef<Map<string, HTMLInputElement>>(new Map())

  useEffect(() => {
    if (open) {
      setFiles([])
      setRestartXochitl(true)
      setIsDragging(false)
      setError(null)
    }
  }, [open])

  const addPaths = useCallback(async (paths: string[]) => {
    const validPaths = paths.filter((p) => {
      const lower = p.toLowerCase()
      return lower.endsWith('.pdf') || lower.endsWith('.epub') || lower.endsWith('.rmdoc')
    })
    if (validPaths.length === 0) {
      setError('Select PDF, EPUB, or rmdoc files.')
      return
    }
    setError(null)

    const newFiles: StagedFile[] = []
    for (const p of validPaths) {
      try {
        const info = await FilesService.GetImportFileInfo(p)
        const filename = p.replace(/\\/g, '/').split('/').pop() ?? p
        const isRmdoc = info.fileType === 'rmdoc'
        newFiles.push({
          id: crypto.randomUUID(),
          path: p,
          filename,
          editedName: isRmdoc && info.visibleName
            ? info.visibleName
            : filename.replace(/\.(pdf|epub|rmdoc)$/i, ''),
          size: info.size,
          pageCount: info.pageCount,
          pageCountOverride: null,
          coverPage: info.fileType === 'pdf' ? 'first' : 'none',
          fileType: info.fileType as 'pdf' | 'epub' | 'rmdoc',
          status: 'staged',
        })
      } catch (err) {
        const filename = p.replace(/\\/g, '/').split('/').pop() ?? p
        setError(handleError(err, `Could not read ${filename}.`))
      }
    }
    if (newFiles.length > 0) {
      setFiles((prev) => [...prev, ...newFiles])
    }
  }, [])

  useEffect(() => {
    if (!open) return

    let dragCounter = 0

    const handleDragEnter = (e: DragEvent) => {
      e.preventDefault()
      dragCounter++
      if (e.dataTransfer?.types.includes('Files')) setIsDragging(true)
    }
    const handleDragLeave = (e: DragEvent) => {
      e.preventDefault()
      dragCounter--
      if (dragCounter === 0) setIsDragging(false)
    }
    const handleDragOver = (e: DragEvent) => { e.preventDefault() }
    const handleDrop = (e: DragEvent) => {
      e.preventDefault()
      dragCounter = 0
      setIsDragging(false)
    }

    const handleFileDrop = (paths: EventData<'files:dropped'>) => {
      dragCounter = 0
      setIsDragging(false)
      if (!paths || paths.length === 0) return
      addPaths(paths)
    }

    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('drop', handleDrop)
    const unsub = onEvent('files:dropped', handleFileDrop)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDrop)
      unsub()
    }
  }, [open, addPaths])

  const handleImportRef = useRef<() => Promise<void>>(async () => {})
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && files.length > 0 && !commandRunning) {
        e.preventDefault()
        handleImportRef.current()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open, files, commandRunning])

  const handleBrowse = async () => {
    const paths = await FilesService.SelectImportFiles()
    if (paths && paths.length > 0) addPaths(paths)
  }

  const updateFile = (id: string, update: Partial<StagedFile>) => {
    setFiles((prev) => prev.map((f) => (f.id === id ? { ...f, ...update } : f)))
  }

  const removeFile = (id: string) => {
    setFiles((prev) => prev.filter((f) => f.id !== id))
  }

  const isImportable = (file: StagedFile) => file.status !== 'done' && !!file.editedName.trim()

  const handleImport = async () => {
    const toImport = files.filter(isImportable)
    if (toImport.length === 0) return

    setError(null)
    let succeeded = 0
    let failed = 0

    const result = await startOperation({
      kind: 'import',
      title: 'Import documents',
      background: true,
      reportsOwnFailure: true,
      onProgress: progress => {
        const file = toImport[progress.index]
        if (!file) return
        if (progress.status === InstallStatus.InstallImporting) {
          updateFile(file.id, { status: 'uploading', error: undefined })
        } else if (progress.status === InstallStatus.InstallCompleted) {
          updateFile(file.id, { status: 'done' })
          succeeded++
        } else if (progress.status === InstallStatus.InstallError) {
          updateFile(file.id, { status: 'error', error: handleError(progress.message, 'Could not import this file.') })
          failed++
        }
      },
      run: id => FilesService.ImportDocuments(
        id,
        toImport.map(file => ({
          path: file.path,
          visibleName: file.editedName.trim(),
          pageCountOverride: file.fileType === 'pdf' ? file.pageCountOverride ?? 0 : 0,
          coverPageNumber: file.fileType === 'pdf' ? (file.coverPage === 'first' ? 0 : -1) : null,
        })),
        canRescanLibrary,
        !canRescanLibrary && restartXochitl,
      ),
    })

    if (result.ok) {
      toast.success(`${succeeded} document${succeeded !== 1 ? 's' : ''} imported`)
      onOpenChange(false)
      return
    }
    if (result.canceled) return
    if (failed > 0) {
      setError(`${failed} of ${toImport.length} import${toImport.length !== 1 ? 's' : ''} failed.`)
    } else if (succeeded > 0) {
      setError(handleError(operationFailure(result), canRescanLibrary ? 'Imported, but could not rescan the library.' : 'Imported, but could not restart the reMarkable UI.'))
    } else {
      setError(handleError(operationFailure(result), 'Could not import the documents.'))
    }
  }

  handleImportRef.current = handleImport

  const handleClose = () => { if (!importing) onOpenChange(false) }

  const hasFiles = files.length > 0

  return (
    <Dialog open={open} onOpenChange={(o) => !o && handleClose()}>
      <DialogContent size="2xl">
        <DialogHeader>
          <DialogTitle>Document Import</DialogTitle>
          <DialogDescription>
            {hasTouchPointer
              ? 'Upload PDF, EPUB, or rmdoc files to your reMarkable.'
              : 'Upload PDF, EPUB, or rmdoc files to your reMarkable. Drag and drop or browse to add files.'}
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-hidden flex flex-col gap-2">
          <div
            className={[
              'border-2 border-dashed rounded-lg text-center select-none transition-colors shrink-0',
              hasFiles ? 'py-1.5 cursor-pointer' : 'p-5 cursor-pointer',
              isDragging
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-primary hover:bg-muted/20',
            ].join(' ')}
            onClick={!importing ? handleBrowse : undefined}
          >
            {hasFiles ? (
              <div className="flex items-center justify-center gap-1.5 text-muted-foreground">
                <Plus className="h-3.5 w-3.5" />
                <span className="text-xs font-medium">Add more files</span>
              </div>
            ) : (
              <>
                <UploadSimple className={`h-7 w-7 mx-auto mb-1 ${isDragging ? 'text-primary' : 'text-muted-foreground'}`} />
                {hasTouchPointer ? (
                  <p className="text-sm font-medium">Choose files</p>
                ) : (
                  <>
                    <p className="text-sm font-medium">{isDragging ? 'Drop to stage' : 'Drop files here'}</p>
                    <p className="text-xs text-muted-foreground mt-0.5">or browse</p>
                  </>
                )}
              </>
            )}
          </div>

          {hasFiles && (
            <p className="text-xs text-muted-foreground">
              {files.length} file{files.length !== 1 ? 's' : ''} selected
            </p>
          )}

          {hasFiles && (
            <div className="overflow-y-auto flex-1 min-h-0 max-h-[40vh]">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {files.map((file) => (
                  <FileCard
                    key={file.id}
                    file={file}
                    importing={importing}
                    onUpdate={(update) => updateFile(file.id, update)}
                    onRemove={() => removeFile(file.id)}
                    nameInputRef={(el) => {
                      if (el) nameInputRefs.current.set(file.id, el)
                      else nameInputRefs.current.delete(file.id)
                    }}
                  />
                ))}
              </div>
            </div>
          )}
        </div>

        {hasFiles && (
          <div className="border-t pt-3 mt-3">
            <div className="flex items-center gap-2">
              <Checkbox
                id="pdfRestart"
                checked={canRescanLibrary || restartXochitl}
                onCheckedChange={(v: boolean | 'indeterminate') => setRestartXochitl(!!v)}
                disabled={importing || canRescanLibrary}
              />
              <Label htmlFor="pdfRestart" className={`font-normal text-sm ${canRescanLibrary ? '' : 'cursor-pointer'}`}>
                {canRescanLibrary ? 'Rescan library after import' : 'Restart UI after import'}
              </Label>
            </div>
            <p className="text-xs text-muted-foreground mt-1 ml-6">
              {canRescanLibrary
                ? 'The librarian mod rescans the library without restarting the UI.'
                : hasLibrarian
                  ? <>Documents appear after the UI restarts.<br />Start mods to import without restarting.</>
                  : <>Documents appear after the UI restarts.<br />Install the librarian mod to import without restarting.</>}
            </p>
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={importing}>
            Cancel
          </Button>
          {hasFiles && (
            <Button onClick={handleImport} disabled={commandRunning || !files.some(isImportable) || !isConnected}>
              {importing ? (
                <>
                  <CircleNotch className="mr-2 h-4 w-4 animate-spin" />
                  Importing…
                </>
              ) : (
                'Import'
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function FileCard({
  file,
  importing,
  onUpdate,
  onRemove,
  nameInputRef,
}: {
  file: StagedFile
  importing: boolean
  onUpdate: (update: Partial<StagedFile>) => void
  onRemove: () => void
  nameInputRef: (el: HTMLInputElement | null) => void
}) {
  const [pagePopoverOpen, setPagePopoverOpen] = useState(false)
  const [pageInputValue, setPageInputValue] = useState('')

  const effectivePages = file.pageCountOverride ?? file.pageCount
  const isOverridden = file.pageCountOverride !== null

  const openPagePopover = () => {
    setPageInputValue(isOverridden ? String(file.pageCountOverride) : '')
    setPagePopoverOpen(true)
  }

  const applyPageOverride = () => {
    const val = parseInt(pageInputValue)
    if (val > 0) {
      onUpdate({ pageCountOverride: val })
    }
    setPagePopoverOpen(false)
  }

  const resetPageOverride = () => {
    onUpdate({ pageCountOverride: null })
    setPagePopoverOpen(false)
  }

  const stepPage = (delta: number) => {
    const current = pageInputValue ? parseInt(pageInputValue) : file.pageCount
    const next = Math.max(1, (current || 1) + delta)
    setPageInputValue(String(next))
  }

  const statusBorder = file.status === 'error'
    ? 'border-destructive'
    : file.status === 'done'
      ? 'border-green-500/50'
      : file.status === 'uploading'
        ? 'border-primary'
        : 'border-border'

  return (
    <div className={`border rounded-md overflow-hidden ${statusBorder}`}>
      <div className="flex items-center gap-1 pl-1.5 pr-2.5 pt-2 pb-0.5">
        <input
          ref={nameInputRef}
          type="text"
          spellCheck={false}
          className="flex-1 min-w-0 bg-transparent border border-transparent rounded px-1 py-0.5 text-sm touch:no-focus-zoom font-medium leading-[1.375rem] outline-none hover:border-border hover:bg-muted focus:border-ring focus:bg-muted"
          value={file.editedName}
          onChange={(e) => onUpdate({ editedName: e.target.value })}
          disabled={importing}
        />
        {!importing && (
          <button
            onClick={onRemove}
            aria-label="Remove file"
            className="inline-flex items-center justify-center w-6 h-6 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 shrink-0"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
        {file.status === 'uploading' && <CircleNotch className="h-4 w-4 animate-spin text-primary shrink-0" />}
      </div>
      <div className="flex items-center gap-1.5 px-3 pb-2 pt-0.5">
        <span className="text-xs text-muted-foreground">{formatBytes(file.size)}</span>
        <span className="text-xs text-muted-foreground">&middot;</span>

        {file.fileType === 'pdf' ? (
          <Popover open={pagePopoverOpen} onOpenChange={setPagePopoverOpen}>
            <PopoverTrigger asChild>
              <button
                onClick={openPagePopover}
                className={`text-xs rounded px-0.5 border-b border-dashed border-transparent hover:border-muted-foreground ${
                  isOverridden ? 'text-foreground font-medium' : 'text-muted-foreground'
                }`}
              >
                {effectivePages} page{effectivePages !== 1 ? 's' : ''}
              </button>
            </PopoverTrigger>
            <PopoverContent className="w-[200px] p-3" align="start">
              <p className="text-sm font-medium mb-2">Page count override</p>
              <div className="flex items-center">
                <input
                  type="text"
                  className="flex-1 h-9 min-w-0 border border-input rounded-l-md px-2.5 text-sm touch:no-focus-zoom bg-transparent outline-none focus:border-ring focus:ring-2 focus:ring-ring/25 placeholder:text-muted-foreground"
                  placeholder={`${file.pageCount} (auto-detected)`}
                  value={pageInputValue}
                  onChange={(e) => setPageInputValue(e.target.value.replace(/[^0-9]/g, ''))}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') { e.preventDefault(); applyPageOverride() }
                    if (e.key === 'Escape') { e.preventDefault(); setPagePopoverOpen(false) }
                    e.stopPropagation()
                  }}
                  autoFocus
                />
                <div className="flex flex-col shrink-0">
                  <button
                    onClick={() => stepPage(1)}
                    aria-label="Increase page count"
                    className="flex items-center justify-center w-7 h-[18px] border border-input border-l-0 rounded-tr-md hover:bg-muted"
                  >
                    <CaretUp className="h-3.5 w-3.5 text-muted-foreground" />
                  </button>
                  <button
                    onClick={() => stepPage(-1)}
                    aria-label="Decrease page count"
                    className="flex items-center justify-center w-7 h-[18px] border border-input border-l-0 border-t-0 rounded-br-md hover:bg-muted"
                  >
                    <CaretDown className="h-3.5 w-3.5 text-muted-foreground" />
                  </button>
                </div>
              </div>
              <div className="flex justify-end gap-1.5 mt-2.5">
                <Button variant="outline" size="sm" className="h-8 text-sm" onClick={resetPageOverride}>
                  Reset
                </Button>
                <Button size="sm" className="h-8 text-sm" onClick={applyPageOverride}>
                  Apply
                </Button>
              </div>
            </PopoverContent>
          </Popover>
        ) : file.fileType === 'epub' ? (
          <span className="text-xs text-muted-foreground">EPUB</span>
        ) : (
          <span className="text-xs text-muted-foreground">
            {effectivePages} page{effectivePages !== 1 ? 's' : ''}
          </span>
        )}

        <span className="flex-1" />

        {file.fileType === 'pdf' ? (
          <Select
            value={file.coverPage}
            onValueChange={(v: 'first' | 'none') => onUpdate({ coverPage: v })}
            disabled={importing}
          >
            <SelectTrigger className="h-auto w-auto gap-1 border-border bg-transparent px-2 py-0.5 text-xs text-muted-foreground [&>svg]:h-2.5 [&>svg]:w-2.5">
              <span className="font-medium">Cover:</span>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="first">First page</SelectItem>
              <SelectItem value="none">Last visited</SelectItem>
            </SelectContent>
          </Select>
        ) : file.fileType === 'epub' ? (
          <span className="text-xs font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
            EPUB
          </span>
        ) : (
          <span className="text-xs font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
            Notebook
          </span>
        )}
      </div>
      {file.status === 'error' && file.error && (
        <p className="text-xs text-destructive px-3 pb-2 truncate">{file.error}</p>
      )}
    </div>
  )
}
