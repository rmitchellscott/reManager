import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { FileText, Upload, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'

interface ImportPDFDialogProps {
  open: boolean
  isConnected: boolean
  onClose: () => void
}

export function ImportPDFDialog({ open, isConnected, onClose }: ImportPDFDialogProps) {
  const [stagedPath, setStagedPath] = useState<string | null>(null)
  const [editedName, setEditedName] = useState('')
  const [manualPageCount, setManualPageCount] = useState('')
  const [restartXochitl, setRestartXochitl] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Reset internal state every time the dialog opens
  useEffect(() => {
    if (open) {
      setStagedPath(null)
      setEditedName('')
      setManualPageCount('')
      setRestartXochitl(true)
      setUploading(false)
      setIsDragging(false)
      setError(null)
    }
  }, [open])

  const stageFile = useCallback((filePath: string) => {
    const normalized = filePath.replace(/\\/g, '/')
    const filename = normalized.split('/').pop() ?? filePath
    const nameWithoutExt = filename.replace(/\.pdf$/i, '')
    setStagedPath(filePath)
    setEditedName(nameWithoutExt)
    setManualPageCount('')
    setError(null)
  }, [])

  // Subscribe to Wails file-drop events while the dialog is open
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

    const handleFileDrop = (...args: unknown[]) => {
      dragCounter = 0
      setIsDragging(false)
      const paths = args[2] as string[]
      if (!paths || paths.length === 0) return
      const pdfPaths = paths.filter((p) => p.toLowerCase().endsWith('.pdf'))
      if (pdfPaths.length === 0) {
        setError('Please upload a PDF file.')
        return
      }
      stageFile(pdfPaths[0])
    }

    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('drop', handleDrop)
    const unsub = window.runtime.EventsOn('wails:file-drop', handleFileDrop)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDrop)
      unsub()
    }
  }, [open, stageFile])

  const handleBrowse = async () => {
    const filePath = await window.go.main.App.SelectPDFFile()
    if (filePath) stageFile(filePath)
  }

  const handleImport = async () => {
    if (!stagedPath || !editedName.trim()) return

    const trimmedPageCount = manualPageCount.trim()
    let pageCountOverride = 0
    if (trimmedPageCount !== '') {
      const parsed = Number(trimmedPageCount)
      if (!Number.isInteger(parsed) || parsed <= 0) {
        setError('Page count must be a nonnegative integer.')
        return
      }
      pageCountOverride = parsed
    }

    setUploading(true)
    setError(null)
    try {
      await window.go.main.App.ImportPDFFromPath(stagedPath, editedName.trim(), restartXochitl, pageCountOverride)
      toast.success('PDF imported successfully')
      onClose()
    } catch (err: unknown) {
      setError(String(err))
    } finally {
      setUploading(false)
    }
  }

  const handleClose = () => { if (!uploading) onClose() }

  const stagedFilename = stagedPath?.replace(/\\/g, '/').split('/').pop()

  return (
    <Dialog open={open} onOpenChange={(o) => !o && handleClose()}>
      <DialogContent className="w-[92vw] sm:w-[50vw] max-w-2xl">
        <DialogHeader>
          <DialogTitle>Import PDF</DialogTitle>
        </DialogHeader>

        {/* Drop zone */}
        <div
          className={[
            'relative border-2 border-dashed rounded-lg p-8 text-center transition-colors select-none',
            isDragging
              ? 'border-primary bg-primary/5'
              : stagedPath
                ? 'border-border bg-muted/30'
                : 'border-border cursor-pointer hover:border-primary hover:bg-muted/20',
          ].join(' ')}
          onClick={!stagedPath && !uploading ? handleBrowse : undefined}
        >
          {stagedPath ? (
            <div className="flex items-center gap-3">
              <FileText className="h-8 w-8 text-primary shrink-0" />
              <div className="flex-1 text-left min-w-0">
                <p className="text-sm font-medium truncate">{stagedFilename}</p>
                <button
                  className="text-xs text-muted-foreground hover:text-foreground mt-0.5"
                  onClick={(e) => { e.stopPropagation(); setStagedPath(null); setEditedName('') }}
                  disabled={uploading}
                >
                  Change file
                </button>
              </div>
            </div>
          ) : (
            <>
              <Upload className={`h-10 w-10 mx-auto mb-2 ${isDragging ? 'text-primary' : 'text-muted-foreground'}`} />
              <p className="text-sm font-medium">{isDragging ? 'Drop to stage' : 'Drop a PDF here'}</p>
              <p className="text-xs text-muted-foreground mt-1">or click to browse</p>
            </>
          )}
        </div>

        {/* Only shown once a file is staged */}
        {stagedPath && (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="pdfName">Document name</Label>
              <Input
                id="pdfName"
                value={editedName}
                onChange={(e) => setEditedName(e.target.value)}
                disabled={uploading}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pdfPageCount">Page count override (optional)</Label>
              <Input
                id="pdfPageCount"
                type="number"
                placeholder="Auto-detect"
                value={manualPageCount}
                onChange={(e) => setManualPageCount(e.target.value)}
                disabled={uploading}
              />
              <p className="text-xs text-muted-foreground">
                Fallback if auto-detect fails.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="pdfRestart"
                checked={restartXochitl}
                onCheckedChange={(v) => setRestartXochitl(!!v)}
                disabled={uploading}
              />
              <Label htmlFor="pdfRestart" className="cursor-pointer font-normal text-sm">
                Restart xochitl after upload.
              </Label>
            </div>
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={uploading}>
            Cancel
          </Button>
          {stagedPath && (
            <Button onClick={handleImport} disabled={uploading || !editedName.trim() || !isConnected}>
              {uploading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Uploading…
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
