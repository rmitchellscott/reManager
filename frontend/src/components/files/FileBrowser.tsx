import { ArrowsClockwise, ArrowsCounterClockwise, CheckSquare, X, DotsThreeVertical, Eye, EyeSlash, FolderOpen, FolderPlus, House, Pencil, Terminal, Trash, UploadSimple, Warning } from '@phosphor-icons/react'
import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { useAppContext } from '@/contexts/AppContext'
import { useMediaQuery, NARROW_VIEWPORT } from '@/hooks/useMediaQuery'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { FileList } from './FileList'
import { TransferPanel } from './TransferPanel'
import type { FileActionHandlers } from './fileActions'
import type { SortColumn, SortOrder } from './types'
import { TransferKind, TransferState, type FileInfo } from '@/lib/types'
import { ConfigService, FilesService } from '@bindings/reManager'
import { operationFailure } from '@/lib/operations'
import { onEvent, type EventData } from '@/lib/events'

interface FileBrowserProps {
  isConnected: boolean
  suppressSystemFileWarnings: boolean
  isVisible: boolean
  sleepScreenSupported: boolean
  currentSleepScreen: string | null
  onSleepScreenChange: (path: string | null) => void
  initialPath?: string
  pickMode?: boolean
  onPick?: (path: string) => void
  onClose?: () => void
}

const SYSTEM_ACTION_PHRASES = {
  upload: 'upload files to',
  delete: 'delete files from',
  rename: 'rename a file in',
  mkdir: 'create a folder in',
  move: 'move files in',
}

interface SystemFileWarning {
  action: keyof typeof SYSTEM_ACTION_PHRASES
  path: string
  onConfirm: () => void
}

interface Clipboard {
  files: FileInfo[]
  sourcePath: string
}

const HOME = '/home/root'

const isSystemPath = (path: string) => !path.startsWith(HOME)

const isInside = (path: string, ancestor: string) => path === ancestor || path.startsWith(ancestor + '/')

const barButton = 'h-[36px] px-[12px] text-[14px] max-md:h-[40px] touch:hit-target'
const barIcon = 'h-[16px] w-[16px]'

export function FileBrowser({
  isConnected,
  suppressSystemFileWarnings,
  isVisible,
  sleepScreenSupported,
  currentSleepScreen,
  onSleepScreenChange,
  initialPath,
  pickMode = false,
  onPick,
  onClose,
}: FileBrowserProps) {
  const { transfers, lastTransferFinished, transferError, setTransferError, commandRunning, startOperation } = useAppContext()
  const narrow = useMediaQuery(NARROW_VIEWPORT)

  const [showHidden, setShowHidden] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [sortBy, setSortBy] = useState<SortColumn>('name')
  const [sortOrder, setSortOrder] = useState<SortOrder>('asc')
  const [currentPath, setCurrentPath] = useState(initialPath || HOME)
  const [files, setFiles] = useState<FileInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [errorDetails, setErrorDetails] = useState('')
  const showFailure = (err: unknown, fallback: string) => {
    setError(handleError(err, fallback))
    setErrorDetails(errorDetail(err))
  }
  const showMessage = (message: string | null) => {
    setError(message)
    setErrorDetails('')
  }

  const [selection, setSelection] = useState<Set<string>>(new Set())
  const [selecting, setSelecting] = useState(false)
  const [clipboard, setClipboard] = useState<Clipboard | null>(null)
  const [collisions, setCollisions] = useState<string[] | null>(null)
  const lastSelectedRef = useRef<string | null>(null)

  const [deleteDialog, setDeleteDialog] = useState<FileInfo[] | null>(null)
  const [renameDialog, setRenameDialog] = useState<FileInfo | null>(null)
  const [newFolderDialog, setNewFolderDialog] = useState(false)
  const [renameName, setRenameName] = useState('')
  const [newFolderName, setNewFolderName] = useState('')
  const [systemFileWarning, setSystemFileWarning] = useState<SystemFileWarning | null>(null)
  const [restartXochitlDialog, setRestartXochitlDialog] = useState<{
    message: string
    onRestart: () => void
  } | null>(null)

  const [isEditingPath, setIsEditingPath] = useState(false)
  const [editedPath, setEditedPath] = useState('')
  const pathInputRef = useRef<HTMLInputElement>(null)

  const confirmSystemFileAction = (action: SystemFileWarning['action'], path: string, onConfirm: () => void) => {
    if (isSystemPath(path) && !suppressSystemFileWarnings) {
      setSystemFileWarning({ action, path, onConfirm })
    } else {
      onConfirm()
    }
  }

  const loadDirectory = useCallback(async (path: string) => {
    if (!isConnected) return
    setLoading(true)
    showMessage(null)
    try {
      const result = await FilesService.ListDirectory(path)
      setFiles(result || [])
      setCurrentPath(path)
      setSelection(new Set())
      setSelecting(false)
      lastSelectedRef.current = null
    } catch (err) {
      showFailure(err, 'Could not open this folder.')
      throw err
    } finally {
      setLoading(false)
    }
  }, [isConnected])

  useEffect(() => {
    if (isConnected) {
      loadDirectory(currentPath)
    } else {
      setFiles([])
    }
  }, [isConnected])

  const handledTransferRef = useRef<string | null>(null)
  useEffect(() => {
    if (!lastTransferFinished) return
    if (handledTransferRef.current === lastTransferFinished.id) return
    handledTransferRef.current = lastTransferFinished.id
    if (lastTransferFinished.kind === TransferKind.TransferUpload && lastTransferFinished.state === TransferState.TransferDone) {
      loadDirectory(currentPath)
    }
  }, [lastTransferFinished, currentPath])

  useEffect(() => {
    if (!transferError) return
    showFailure(transferError.code ? transferError : transferError.message, 'Could not transfer the files.')
    setTransferError(null)
  }, [transferError])

  const handleEnterEditMode = () => {
    setEditedPath(currentPath)
    setIsEditingPath(true)
  }

  const handlePathSubmit = async () => {
    const trimmedPath = editedPath.trim()

    if (!trimmedPath) {
      showMessage('Enter a path.')
      return
    }

    try {
      await loadDirectory(trimmedPath)
      setIsEditingPath(false)
    } catch {
    }
  }

  const handlePathCancel = () => {
    setIsEditingPath(false)
    setEditedPath('')
    showMessage(null)
  }

  const handlePathKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handlePathSubmit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handlePathCancel()
    }
  }

  useEffect(() => {
    if (isEditingPath && pathInputRef.current) {
      pathInputRef.current.focus()
      pathInputRef.current.select()
    }
  }, [isEditingPath])

  useEffect(() => {
    if (!isVisible) return

    let dragCounter = 0

    const handleDragEnter = (e: DragEvent) => {
      e.preventDefault()
      dragCounter++
      if (e.dataTransfer?.types.includes('Files')) {
        setIsDragging(true)
      }
    }

    const handleDragLeave = (e: DragEvent) => {
      e.preventDefault()
      dragCounter--
      if (dragCounter === 0) {
        setIsDragging(false)
      }
    }

    const handleDragOver = (e: DragEvent) => {
      e.preventDefault()
    }

    const handleDrop = (e: DragEvent) => {
      e.preventDefault()
      dragCounter = 0
      setIsDragging(false)
    }

    const handleFileDrop = (paths: EventData<'files:dropped'>) => {
      dragCounter = 0
      setIsDragging(false)
      if (!isConnected || !paths || paths.length === 0) return

      const uploadFiles = () => {
        FilesService.UploadFilesFromPaths(paths, currentPath + '/')
      }

      if (isSystemPath(currentPath) && !suppressSystemFileWarnings) {
        setSystemFileWarning({
          action: 'upload',
          path: currentPath,
          onConfirm: uploadFiles,
        })
      } else {
        uploadFiles()
      }
    }

    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('drop', handleDrop)

    const unsubFileDrop = onEvent('files:dropped', handleFileDrop)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDrop)
      unsubFileDrop()
    }
  }, [isVisible, isConnected, currentPath, suppressSystemFileWarnings])

  const handleNavigate = (file: FileInfo) => {
    if (file.isDir) loadDirectory(file.path)
  }

  const handleGoUp = () => {
    const parent = currentPath.split('/').slice(0, -1).join('/') || '/'
    loadDirectory(parent)
  }

  const handleUpload = async () => {
    const paths = await FilesService.SelectFilesForUpload()
    if (!paths || paths.length === 0) return
    confirmSystemFileAction('upload', currentPath, () => {
      FilesService.UploadFilesFromPaths(paths, currentPath + '/')
    })
  }

  const handleUploadFolder = () => {
    confirmSystemFileAction('upload', currentPath, () => {
      FilesService.UploadFolder(currentPath + '/')
    })
  }

  const handleSort = (column: SortColumn) => {
    if (sortBy === column) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
    } else {
      setSortBy(column)
      setSortOrder('asc')
    }
  }

  const displayFiles = useMemo(() => {
    const filtered = showHidden ? files : files.filter(f => !f.name.startsWith('.'))
    return [...filtered].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1

      let cmp = 0
      switch (sortBy) {
        case 'name':
          cmp = a.name.toLowerCase().localeCompare(b.name.toLowerCase())
          break
        case 'size':
          cmp = a.size - b.size
          break
        case 'modTime':
          cmp = a.modTime - b.modTime
          break
      }
      return sortOrder === 'asc' ? cmp : -cmp
    })
  }, [files, showHidden, sortBy, sortOrder])

  const selectedFiles = useMemo(
    () => displayFiles.filter(f => selection.has(f.path)),
    [displayFiles, selection]
  )

  const handleToggleSelect = (file: FileInfo, shiftKey: boolean) => {
    setSelection(prev => {
      const next = new Set(prev)
      const anchor = lastSelectedRef.current
      if (shiftKey && anchor) {
        const from = displayFiles.findIndex(f => f.path === anchor)
        const to = displayFiles.findIndex(f => f.path === file.path)
        if (from !== -1 && to !== -1) {
          const [lo, hi] = from < to ? [from, to] : [to, from]
          for (let i = lo; i <= hi; i++) next.add(displayFiles[i].path)
          return next
        }
      }
      if (next.has(file.path)) next.delete(file.path)
      else next.add(file.path)
      return next
    })
    lastSelectedRef.current = file.path
  }

  const handleLongPress = (file: FileInfo) => {
    if (pickMode) return
    setSelecting(true)
    setSelection(new Set([file.path]))
    lastSelectedRef.current = file.path
  }

  const exitSelection = () => {
    setSelecting(false)
    setSelection(new Set())
    lastSelectedRef.current = null
  }

  const handleStartMove = () => {
    if (selectedFiles.length === 0) return
    setClipboard({ files: selectedFiles, sourcePath: currentPath })
    exitSelection()
  }

  // /home/root and the root filesystem are separate partitions on device, and SFTP rename cannot cross one.
  const pasteBlockedReason = useMemo(() => {
    if (!clipboard) return null
    if (clipboard.sourcePath === currentPath) return 'Already in this folder'
    if (clipboard.files.some(f => f.isDir && isInside(currentPath, f.path))) {
      return 'Cannot move a folder into itself'
    }
    if (isSystemPath(clipboard.sourcePath) !== isSystemPath(currentPath)) {
      return 'Cannot move between the home and system partitions'
    }
    return null
  }, [clipboard, currentPath])

  const runMove = async (replace: boolean) => {
    if (!clipboard) return
    const failures: string[] = []
    for (const file of clipboard.files) {
      const destination = currentPath + '/' + file.name
      try {
        if (replace) {
          await FilesService.DeletePath(destination).catch(() => {})
        }
        await FilesService.RenamePath(file.path, destination)
      } catch (err) {
        const reason = handleError(err, '')
        failures.push(reason ? `${file.name}: ${reason}` : `Could not move ${file.name}.`)
      }
    }
    setClipboard(null)
    setCollisions(null)
    if (failures.length > 0) showMessage(failures.join('\n'))
    loadDirectory(currentPath)
  }

  const handlePaste = () => {
    if (!clipboard || pasteBlockedReason) return
    const existing = new Set(files.map(f => f.name))
    const conflicting = clipboard.files.filter(f => existing.has(f.name)).map(f => f.name)

    const proceed = () => {
      if (conflicting.length > 0) {
        setCollisions(conflicting)
      } else {
        runMove(false)
      }
    }

    confirmSystemFileAction('move', currentPath, proceed)
  }

  const initiateDelete = (targets: FileInfo[]) => {
    if (targets.length === 0) return
    const systemTarget = targets.find(f => isSystemPath(f.path))
    if (systemTarget && !suppressSystemFileWarnings) {
      confirmSystemFileAction('delete', systemTarget.path, () => setDeleteDialog(targets))
    } else {
      setDeleteDialog(targets)
    }
  }

  const handleDelete = async () => {
    if (!deleteDialog) return
    const failures: string[] = []
    for (const file of deleteDialog) {
      try {
        await FilesService.DeletePath(file.path)
      } catch (err) {
        const reason = handleError(err, '')
        failures.push(reason ? `${file.name}: ${reason}` : `Could not delete ${file.name}.`)
      }
    }
    const deletedPaths = deleteDialog.map(f => f.path)
    setDeleteDialog(null)
    exitSelection()
    if (failures.length > 0) showMessage(failures.join('\n'))

    const containing = deletedPaths.find(p => isInside(currentPath, p))
    if (containing) {
      loadDirectory(containing.substring(0, containing.lastIndexOf('/')) || '/')
    } else {
      loadDirectory(currentPath)
    }
  }

  const initiateRename = (file: FileInfo) => {
    const open = () => {
      setRenameName(file.name)
      setRenameDialog(file)
    }
    if (isSystemPath(file.path) && !suppressSystemFileWarnings) {
      confirmSystemFileAction('rename', file.path, open)
    } else {
      open()
    }
  }

  const selectRenameStem = useCallback((input: HTMLInputElement | null) => {
    if (!input || !renameDialog) return
    const extensionStart = renameDialog.isDir ? -1 : renameDialog.name.lastIndexOf('.')
    input.setSelectionRange(0, extensionStart > 0 ? extensionStart : renameDialog.name.length)
  }, [renameDialog])

  const handleRename = async () => {
    if (!isConnected || !renameDialog || !renameName.trim()) return
    const newPath = currentPath + '/' + renameName.trim()
    try {
      await FilesService.RenamePath(renameDialog.path, newPath)
      setRenameDialog(null)
      setRenameName('')
      loadDirectory(currentPath)
    } catch (err) {
      showFailure(err, `Could not rename ${renameDialog.name}.`)
    }
  }

  const handleCreateFolder = async () => {
    if (!isConnected || !newFolderName.trim()) return
    const path = currentPath + '/' + newFolderName.trim()

    const doCreate = async () => {
      try {
        await FilesService.CreateDirectory(path)
        setNewFolderDialog(false)
        setNewFolderName('')
        loadDirectory(currentPath)
      } catch (err) {
        showFailure(err, 'Could not create the folder.')
      }
    }

    if (isSystemPath(path) && !suppressSystemFileWarnings) {
      setNewFolderDialog(false)
      confirmSystemFileAction('mkdir', path, doCreate)
    } else {
      doCreate()
    }
  }

  const handleSetSleepScreen = async (file: FileInfo) => {
    const set = await startOperation({
      kind: 'sleep-screen',
      title: 'Set the sleep screen',
      background: true,
      reportsOwnFailure: true,
      run: id => ConfigService.SetSleepScreen(id, file.path),
    })
    if (!set.ok) {
      showFailure(operationFailure(set), 'Could not set the sleep screen.')
      return
    }
    onSleepScreenChange(file.path)
    setRestartXochitlDialog({
      message: `Sleep screen set to "${file.name}". It takes effect after the UI restarts.`,
      onRestart: async () => {
        const restarted = await startOperation({
          kind: 'restart-ui',
          title: 'Restart the reMarkable UI',
          background: true,
          reportsOwnFailure: true,
          run: id => ConfigService.RestartXochitl(id),
        })
        if (!restarted.ok) showFailure(operationFailure(restarted), 'Could not restart the reMarkable UI.')
        setRestartXochitlDialog(null)
      }
    })
  }

  const handlers: FileActionHandlers = {
    onDownload: (file) => FilesService.DownloadFile(file.path),
    onDownloadFolder: (file) => FilesService.DownloadFolder(file.path),
    onSetSleepScreen: handleSetSleepScreen,
    onRename: initiateRename,
    onCopyPath: (path) => navigator.clipboard.writeText(path),
    onDelete: (file) => initiateDelete([file]),
  }

  const pathSegments = currentPath.split('/').filter(Boolean)
  const collapsedSegments = narrow && pathSegments.length > 2
    ? pathSegments.slice(0, pathSegments.length - 2)
    : []
  const visibleSegments = pathSegments.slice(collapsedSegments.length)

  const segmentPath = (index: number) => '/' + pathSegments.slice(0, index + 1).join('/')

  return (
    <div className="relative space-y-[16px]">
      {isDragging && (
        <div className="absolute inset-0 z-40 bg-muted/60 border-2 border-dashed border-border rounded-[var(--radius)] flex items-center justify-center pointer-events-none">
          <div className="bg-background/95 px-6 py-4 rounded-[var(--radius)] shadow-lg text-center">
            <UploadSimple className="h-8 w-8 mx-auto mb-2 text-muted-foreground" />
            <p className="text-sm font-semibold">Drop files to upload</p>
            <p className="text-xs text-muted-foreground">to {currentPath}</p>
          </div>
        </div>
      )}

      <div className="flex items-center gap-[8px]">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-[32px] w-[32px] shrink-0 p-0 max-md:h-[40px] max-md:w-[40px]"
              onClick={handleEnterEditMode}
              disabled={loading || isEditingPath}
              aria-label="Edit path"
            >
              <Terminal className="h-[16px] w-[16px]" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Edit path</TooltipContent>
        </Tooltip>
        {isEditingPath ? (
          <Input
            ref={pathInputRef}
            value={editedPath}
            onChange={(e) => setEditedPath(e.target.value)}
            onKeyDown={handlePathKeyDown}
            onBlur={handlePathSubmit}
            className="flex-1 font-mono text-sm h-8 focus-visible:ring-1"
            placeholder="/home/root"
          />
        ) : (
          <Breadcrumb className="min-w-0">
            <BreadcrumbList className="min-w-0 flex-nowrap">
              {collapsedSegments.length > 0 ? (
                <BreadcrumbItem>
                  <DropdownMenu>
                    <DropdownMenuTrigger className="px-1">…</DropdownMenuTrigger>
                    <DropdownMenuContent align="start">
                      <DropdownMenuItem onSelect={() => loadDirectory('/')}>/</DropdownMenuItem>
                      {collapsedSegments.map((segment, i) => (
                        <DropdownMenuItem key={i} onSelect={() => loadDirectory(segmentPath(i))}>
                          {segment}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </BreadcrumbItem>
              ) : (
                <BreadcrumbItem>
                  {currentPath === '/' ? (
                    <BreadcrumbPage>/</BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink className="cursor-pointer" onClick={() => loadDirectory('/')}>
                      /
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
              )}
              {visibleSegments.map((segment, i) => {
                const index = collapsedSegments.length + i
                const isLast = index === pathSegments.length - 1
                return (
                  <React.Fragment key={index}>
                    <BreadcrumbSeparator />
                    <BreadcrumbItem className="min-w-0">
                      {isLast ? (
                        <BreadcrumbPage className="truncate">{segment}</BreadcrumbPage>
                      ) : (
                        <BreadcrumbLink className="cursor-pointer truncate" onClick={() => loadDirectory(segmentPath(index))}>
                          {segment}
                        </BreadcrumbLink>
                      )}
                    </BreadcrumbItem>
                  </React.Fragment>
                )
              })}
            </BreadcrumbList>
          </Breadcrumb>
        )}
        {onClose && (
          <Button
            variant="ghost"
            size="xs"
            className="ml-auto h-[32px] w-[32px] shrink-0 p-0 max-md:h-[40px] max-md:w-[40px] touch:hit-target"
            onClick={onClose}
            aria-label={pickMode ? 'Close image picker' : 'Close file browser'}
          >
            <X className="h-[16px] w-[16px]" />
          </Button>
        )}
      </div>

      {selecting ? (
        <div className="flex flex-wrap items-center gap-[8px]">
          <span className="text-sm font-medium">{selection.size} selected</span>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleStartMove} disabled={selection.size === 0}>
              Move
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => initiateDelete(selectedFiles)}
              disabled={selection.size === 0}
              className="text-destructive border-destructive"
            >
              Delete
            </Button>
            <Button variant="ghost" size="sm" onClick={exitSelection}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-[8px]">
          <Button
            variant="outline"
            size="sm"
            className={`${barButton} max-md:w-[40px] max-md:px-0`}
            onClick={() => loadDirectory(HOME)}
            disabled={currentPath === HOME || loading}
            aria-label="Home"
          >
            <House className={`${barIcon} md:mr-1`} />
            <span className="max-md:hidden">Home</span>
          </Button>

          {pickMode ? (
            <Button variant="outline" size="sm" className={barButton} onClick={handleUpload} disabled={loading}>
              <UploadSimple className={`${barIcon} mr-1`} />
              Upload
            </Button>
          ) : (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className={barButton} disabled={loading}>
                  <UploadSimple className={`${barIcon} mr-1`} />
                  Upload
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuItem onSelect={handleUpload}>
                  <UploadSimple className="h-4 w-4" />
                  Upload files
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={handleUploadFolder}>
                  <FolderOpen className="h-4 w-4" />
                  Upload folder
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}

          {!pickMode && (
            <Button variant="outline" size="sm" className={barButton} onClick={() => setNewFolderDialog(true)} disabled={loading}>
              <FolderPlus className={`${barIcon} mr-1`} />
              New folder
            </Button>
          )}

          <div className="hidden md:flex md:flex-wrap items-center gap-[8px]">
            <Button variant="outline" size="sm" className={barButton} onClick={() => setShowHidden(!showHidden)}>
              {showHidden ? <EyeSlash className={`${barIcon} mr-1`} /> : <Eye className={`${barIcon} mr-1`} />}
              {showHidden ? 'Hide hidden files' : 'Show hidden files'}
            </Button>
            {!pickMode && (
              <Button variant="outline" size="sm" className={barButton} onClick={() => setSelecting(true)} disabled={loading}>
                <CheckSquare className={`${barIcon} mr-1`} />
                Select
              </Button>
            )}
            <Button variant="outline" size="sm" className={`${barButton} w-[36px] px-0 max-md:w-[40px]`} onClick={() => loadDirectory(currentPath)} disabled={loading} aria-label="Refresh">
              <ArrowsClockwise className={`${barIcon} ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>

          <div className="ml-auto md:hidden">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className={`${barButton} w-[40px] px-0`} aria-label="More actions">
                  <DotsThreeVertical className={barIcon} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => loadDirectory(currentPath)}>
                  <ArrowsClockwise className="h-4 w-4" />
                  Refresh
                </DropdownMenuItem>
                {!pickMode && (
                  <DropdownMenuItem onSelect={() => setSelecting(true)}>
                    <CheckSquare className="h-4 w-4" />
                    Select
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onSelect={() => setShowHidden(!showHidden)}>
                  {showHidden ? <EyeSlash className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  {showHidden ? 'Hide hidden files' : 'Show hidden files'}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuLabel>Sort by</DropdownMenuLabel>
                <DropdownMenuRadioGroup value={sortBy} onValueChange={(v) => handleSort(v as SortColumn)}>
                  <DropdownMenuRadioItem value="name">Name</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="size">Size</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="modTime">Modified</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      )}

      {clipboard && (
        <div className="flex flex-wrap items-center gap-[8px] rounded-[var(--radius)] border bg-card px-3 py-2">
          <div className="min-w-0">
            <p className="text-sm">
              {clipboard.files.length === 1
                ? `Moving "${clipboard.files[0].name}"`
                : `Moving ${clipboard.files.length} items`}
            </p>
            {pasteBlockedReason && (
              <p className="text-xs text-muted-foreground">{pasteBlockedReason}</p>
            )}
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <Button size="sm" onClick={handlePaste} disabled={Boolean(pasteBlockedReason)}>
              Move here
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setClipboard(null)}>
              Cancel
            </Button>
          </div>
        </div>
      )}

      {error && (
        <div className="bg-destructive/10 text-destructive text-sm p-2 rounded-[var(--radius)] whitespace-pre-line">
          {error}
          <Button variant="ghost" size="sm" className="ml-2 h-6" onClick={() => showMessage(null)}>
            Dismiss
          </Button>
        </div>
      )}
      {error && <OutputDisclosure output={errorDetails} />}

      <TransferPanel transfers={transfers} />

      <FileList
        files={displayFiles}
        loading={loading}
        narrow={narrow}
        currentPath={currentPath}
        sortBy={sortBy}
        sortOrder={sortOrder}
        onSort={handleSort}
        onNavigate={handleNavigate}
        onGoUp={handleGoUp}
        selection={selection}
        selecting={selecting}
        onToggleSelect={handleToggleSelect}
        onLongPress={handleLongPress}
        currentSleepScreen={currentSleepScreen}
        actionContext={{ sleepScreenSupported, readOnly: pickMode }}
        handlers={handlers}
        pickMode={pickMode}
        onPick={(file) => onPick?.(file.path)}
      />

      <Dialog open={deleteDialog !== null} onOpenChange={(open) => !open && setDeleteDialog(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Trash}>
            <DialogTitle>
              {deleteDialog && deleteDialog.length > 1
                ? `Delete ${deleteDialog.length} items`
                : `Delete ${deleteDialog?.[0]?.isDir ? 'folder' : 'file'}`}
            </DialogTitle>
            <DialogDescription>
              {deleteDialog && deleteDialog.length > 1
                ? `${deleteDialog.length} items, and everything inside any folders, will be deleted.`
                : deleteDialog?.[0]?.isDir
                  ? `"${deleteDialog[0].name}" and everything inside it will be deleted.`
                  : `"${deleteDialog?.[0]?.name}" will be deleted.`}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={!isConnected}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={collisions !== null} onOpenChange={(open) => !open && setCollisions(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Replace existing files?</DialogTitle>
            <DialogDescription>
              {collisions?.length === 1
                ? `"${collisions[0]}" already exists here and will be replaced.`
                : `${collisions?.length} files already exist here and will be replaced.`}
            </DialogDescription>
          </DialogHeader>
          {collisions && collisions.length > 1 && (
            <ul className="list-disc ml-5 space-y-1 text-sm text-muted-foreground">
              {collisions.map(name => <li key={name}>{name}</li>)}
            </ul>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setCollisions(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={() => runMove(true)} disabled={!isConnected}>
              Replace
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={renameDialog !== null} onOpenChange={(open) => !open && setRenameDialog(null)}>
        <DialogContent>
          <DialogHeader severity="neutral" icon={Pencil}>
            <DialogTitle>Rename</DialogTitle>
            <DialogDescription>
              Enter a new name for "{renameDialog?.name}".
            </DialogDescription>
          </DialogHeader>
          <Input
            ref={selectRenameStem}
            value={renameName}
            onChange={(e) => setRenameName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleRename()}
            autoFocus
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameDialog(null)}>
              Cancel
            </Button>
            <Button onClick={handleRename} disabled={!isConnected}>Rename</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={newFolderDialog} onOpenChange={setNewFolderDialog}>
        <DialogContent>
          <DialogHeader severity="neutral" icon={FolderPlus}>
            <DialogTitle>New folder</DialogTitle>
            <DialogDescription>Enter a name for the new folder.</DialogDescription>
          </DialogHeader>
          <Input
            value={newFolderName}
            onChange={(e) => setNewFolderName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreateFolder()}
            placeholder="Folder name"
            autoFocus
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setNewFolderDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreateFolder} disabled={!isConnected}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={systemFileWarning !== null} onOpenChange={(open) => !open && setSystemFileWarning(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Modify system files?</DialogTitle>
            <DialogDescription>
              You are about to {systemFileWarning && SYSTEM_ACTION_PHRASES[systemFileWarning.action]} the system partition.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 text-sm text-muted-foreground">
            <ul className="list-disc ml-5 space-y-1">
              <li>Modifying system files can cause your reMarkable to fail to boot or behave unexpectedly.</li>
              <li>Changes will be overwritten by OS updates.</li>
            </ul>
            <p>Only proceed if you know what you're doing.</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSystemFileWarning(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={!isConnected}
              onClick={() => {
                systemFileWarning?.onConfirm()
                setSystemFileWarning(null)
              }}
            >
              Proceed anyway
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={restartXochitlDialog !== null} onOpenChange={(open) => !open && setRestartXochitlDialog(null)}>
        <DialogContent>
          <DialogHeader severity="info" icon={ArrowsCounterClockwise}>
            <DialogTitle>Restart the reMarkable UI?</DialogTitle>
            <DialogDescription>
              {restartXochitlDialog?.message}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRestartXochitlDialog(null)}>
              Not now
            </Button>
            <Button onClick={restartXochitlDialog?.onRestart} disabled={commandRunning || !isConnected}>
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
