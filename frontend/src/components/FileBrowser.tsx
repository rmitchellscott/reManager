import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { handleError } from '@/lib/errorMessages'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Popover,
  PopoverClose,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import {
  Folder,
  File,
  FileText,
  Image,
  Archive,
  Upload,
  FolderPlus,
  RefreshCw,
  Download,
  Trash2,
  Pencil,
  MoreVertical,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
  Home,
  Copy,
  Eye,
  EyeOff,
  Terminal,
} from 'lucide-react'

interface FileInfo {
  name: string
  path: string
  size: number
  isDir: boolean
  modTime: number
  mode: string
}

interface TransferProgress {
  filename: string
  bytesSent: number
  totalBytes: number
  percentage: number
  status: string
}

interface FileBrowserProps {
  isConnected: boolean
  suppressSystemFileWarnings: boolean
  isVisible: boolean
}

interface SystemFileWarning {
  action: 'upload' | 'delete' | 'rename' | 'mkdir'
  path: string
  onConfirm: () => void
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '--'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function formatDate(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function getFileIcon(file: FileInfo) {
  if (file.isDir) return <Folder className="h-4 w-4 text-blue-500" />
  const ext = file.name.split('.').pop()?.toLowerCase()
  switch (ext) {
    case 'txt':
    case 'md':
    case 'json':
    case 'xml':
    case 'sh':
    case 'conf':
    case 'cfg':
      return <FileText className="h-4 w-4 text-gray-500" />
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'svg':
    case 'bmp':
      return <Image className="h-4 w-4 text-green-500" />
    case 'tar':
    case 'gz':
    case 'zip':
    case 'bz2':
    case 'xz':
    case 'apk':
      return <Archive className="h-4 w-4 text-yellow-500" />
    default:
      return <File className="h-4 w-4 text-gray-400" />
  }
}

export function FileBrowser({ isConnected, suppressSystemFileWarnings, isVisible }: FileBrowserProps) {
  const [showHidden, setShowHidden] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [sortBy, setSortBy] = useState<'name' | 'size' | 'modTime'>('name')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc')
  const [currentPath, setCurrentPath] = useState('/home/root')
  const [files, setFiles] = useState<FileInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [transferProgress, setTransferProgress] = useState<TransferProgress | null>(null)

  const [deleteDialog, setDeleteDialog] = useState<FileInfo | null>(null)
  const [renameDialog, setRenameDialog] = useState<FileInfo | null>(null)
  const [newFolderDialog, setNewFolderDialog] = useState(false)
  const [renameName, setRenameName] = useState('')
  const [newFolderName, setNewFolderName] = useState('')
  const [contextMenu, setContextMenu] = useState<{ file: FileInfo; x: number; y: number } | null>(null)
  const [systemFileWarning, setSystemFileWarning] = useState<SystemFileWarning | null>(null)
  const [sleepScreenSupported, setSleepScreenSupported] = useState(false)
  const [restartXochitlDialog, setRestartXochitlDialog] = useState<{
    message: string
    onRestart: () => void
  } | null>(null)

  const [isEditingPath, setIsEditingPath] = useState(false)
  const [editedPath, setEditedPath] = useState('')
  const pathInputRef = useRef<HTMLInputElement>(null)

  const isSystemPath = (path: string) => !path.startsWith('/home/root')
  const isPngFile = (file: FileInfo) => !file.isDir && file.name.toLowerCase().endsWith('.png')

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
    setError(null)
    try {
      const result = await window.go.main.App.ListDirectory(path)
      setFiles(result || [])
      setCurrentPath(path)
    } catch (err) {
      setError(handleError(err, 'File browser'))
      throw err
    } finally {
      setLoading(false)
    }
  }, [isConnected])

  useEffect(() => {
    if (isConnected) {
      loadDirectory(currentPath)
      window.go.main.App.IsSleepScreenSupported().then(setSleepScreenSupported).catch(() => setSleepScreenSupported(false))
    } else {
      setFiles([])
      setSleepScreenSupported(false)
    }
  }, [isConnected])

  useEffect(() => {
    const handleProgress = (progress: unknown) => {
      setTransferProgress(progress as TransferProgress)
    }

    const handleComplete = () => {
      setTransferProgress(null)
      loadDirectory(currentPath)
    }

    const handleFileBrowserError = (err: unknown) => {
      const errorObj = err as { message: string; code?: string }
      setError(errorObj.code ? handleError(errorObj, 'File transfer') : handleError(errorObj.message, 'File transfer'))
      setTransferProgress(null)
    }

    const unsubProgress = window.runtime.EventsOn('filebrowser:progress', handleProgress)
    const unsubDownload = window.runtime.EventsOn('filebrowser:download-complete', handleComplete)
    const unsubUpload = window.runtime.EventsOn('filebrowser:upload-complete', handleComplete)
    const unsubError = window.runtime.EventsOn('filebrowser:error', handleFileBrowserError)

    return () => {
      unsubProgress()
      unsubDownload()
      unsubUpload()
      unsubError()
    }
  }, [currentPath, loadDirectory])

  const handleEnterEditMode = () => {
    setEditedPath(currentPath)
    setIsEditingPath(true)
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

  const handlePathSubmit = async () => {
    const trimmedPath = editedPath.trim()

    if (!trimmedPath) {
      setError('Path cannot be empty')
      return
    }

    try {
      await loadDirectory(trimmedPath)
      setIsEditingPath(false)
    } catch {
      // Error already set by loadDirectory
    }
  }

  const handlePathCancel = () => {
    setIsEditingPath(false)
    setEditedPath('')
    setError(null)
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

    const handleFileDrop = (...args: unknown[]) => {
      dragCounter = 0
      setIsDragging(false)
      const paths = args[2] as string[]
      if (!isConnected || !paths || paths.length === 0) return

      const uploadFiles = () => {
        window.go.main.App.UploadFilesFromPaths(paths, currentPath + '/')
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

    const unsubFileDrop = window.runtime.EventsOn('wails:file-drop', handleFileDrop)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDrop)
      unsubFileDrop()
    }
  }, [isVisible, isConnected, currentPath, suppressSystemFileWarnings])

  const handleNavigate = (file: FileInfo) => {
    if (file.isDir) {
      loadDirectory(file.path)
    }
  }

  const handleGoUp = () => {
    const parent = currentPath.split('/').slice(0, -1).join('/') || '/'
    loadDirectory(parent)
  }

  const handleUpload = () => {
    confirmSystemFileAction('upload', currentPath, () => {
      window.go.main.App.UploadFile(currentPath + '/')
    })
  }

  const handleDownload = (file: FileInfo) => {
    if (!file.isDir) {
      window.go.main.App.DownloadFile(file.path)
    }
  }

  const initiateDelete = (file: FileInfo) => {
    if (isSystemPath(file.path) && !suppressSystemFileWarnings) {
      confirmSystemFileAction('delete', file.path, () => setDeleteDialog(file))
    } else {
      setDeleteDialog(file)
    }
  }

  const handleDelete = async () => {
    if (!deleteDialog) return
    try {
      await window.go.main.App.DeletePath(deleteDialog.path)
      const deletedPath = deleteDialog.path
      setDeleteDialog(null)
      if (currentPath === deletedPath || currentPath.startsWith(deletedPath + '/')) {
        const parentPath = deletedPath.substring(0, deletedPath.lastIndexOf('/')) || '/'
        loadDirectory(parentPath)
      } else {
        loadDirectory(currentPath)
      }
    } catch (err) {
      setError(handleError(err, 'Delete'))
    }
  }

  const initiateRename = (file: FileInfo) => {
    if (isSystemPath(file.path) && !suppressSystemFileWarnings) {
      confirmSystemFileAction('rename', file.path, () => {
        setRenameName(file.name)
        setRenameDialog(file)
      })
    } else {
      setRenameName(file.name)
      setRenameDialog(file)
    }
  }

  const handleRename = async () => {
    if (!renameDialog || !renameName.trim()) return
    const newPath = currentPath + '/' + renameName.trim()
    try {
      await window.go.main.App.RenamePath(renameDialog.path, newPath)
      setRenameDialog(null)
      setRenameName('')
      loadDirectory(currentPath)
    } catch (err) {
      setError(handleError(err, 'Rename'))
    }
  }

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    const path = currentPath + '/' + newFolderName.trim()

    const doCreate = async () => {
      try {
        await window.go.main.App.CreateDirectory(path)
        setNewFolderDialog(false)
        setNewFolderName('')
        loadDirectory(currentPath)
      } catch (err) {
        setError(handleError(err, 'Create folder'))
      }
    }

    if (isSystemPath(path) && !suppressSystemFileWarnings) {
      setNewFolderDialog(false)
      confirmSystemFileAction('mkdir', path, doCreate)
    } else {
      doCreate()
    }
  }

  const handleContextMenu = (e: React.MouseEvent, file: FileInfo) => {
    e.preventDefault()
    const menuWidth = 192
    const menuHeight = 200
    const x = Math.min(e.clientX, window.innerWidth - menuWidth - 8)
    const y = Math.min(e.clientY, window.innerHeight - menuHeight - 8)
    setContextMenu({ file, x, y })
  }

  const closeContextMenu = () => setContextMenu(null)

  const copyPath = (path: string) => {
    navigator.clipboard.writeText(path)
    closeContextMenu()
  }

  const handleSetSleepScreen = async (file: FileInfo) => {
    closeContextMenu()
    try {
      await window.go.main.App.SetSleepScreen(file.path)
      setRestartXochitlDialog({
        message: `Sleep screen set to "${file.name}". Restart reMarkable UI to apply the change?`,
        onRestart: async () => {
          try {
            await window.go.main.App.RestartXochitl()
          } catch (err) {
            setError(handleError(err, 'Restart UI'))
          }
          setRestartXochitlDialog(null)
        }
      })
    } catch (err) {
      setError(handleError(err, 'Set sleep screen'))
    }
  }

  const handleSort = (column: 'name' | 'size' | 'modTime') => {
    if (sortBy === column) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
    } else {
      setSortBy(column)
      setSortOrder('asc')
    }
  }

  const pathSegments = currentPath.split('/').filter(Boolean)

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

  return (
    <div className="space-y-4 relative" onClick={closeContextMenu}>
      {isDragging && (
        <div className="absolute inset-0 z-40 bg-primary/10 border-2 border-dashed border-primary rounded-lg flex items-center justify-center pointer-events-none">
          <div className="bg-background/95 px-6 py-4 rounded-lg shadow-lg text-center">
            <Upload className="h-12 w-12 mx-auto mb-2 text-primary" />
            <p className="text-lg font-medium">Drop files to upload</p>
            <p className="text-sm text-muted-foreground">to {currentPath}</p>
          </div>
        </div>
      )}
      {/* Breadcrumb / Path Editor */}
          <div className="flex items-center gap-2 overflow-x-auto p-2">
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 shrink-0"
                    onClick={handleEnterEditMode}
                    disabled={loading || isEditingPath}
                  >
                    <Terminal className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Edit path</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            {isEditingPath ? (
              <Input
                ref={pathInputRef}
                value={editedPath}
                onChange={(e) => setEditedPath(e.target.value)}
                onKeyDown={handlePathKeyDown}
                onBlur={handlePathSubmit}
                className="flex-1 font-mono text-sm h-8 focus-visible:ring-1"
                placeholder="Enter path (e.g., /home/root)"
              />
            ) : (
              <Breadcrumb>
                <BreadcrumbList>
                  <BreadcrumbItem>
                    {currentPath === '/' ? (
                      <BreadcrumbPage>/</BreadcrumbPage>
                    ) : (
                      <BreadcrumbLink className="cursor-pointer" onClick={() => loadDirectory('/')}>
                        /
                      </BreadcrumbLink>
                    )}
                  </BreadcrumbItem>
                  {pathSegments.map((segment, i) => {
                    const isLast = i === pathSegments.length - 1
                    const segmentPath = '/' + pathSegments.slice(0, i + 1).join('/')
                    return (
                      <React.Fragment key={i}>
                        <BreadcrumbSeparator />
                        <BreadcrumbItem>
                          {isLast ? (
                            <BreadcrumbPage>{segment}</BreadcrumbPage>
                          ) : (
                            <BreadcrumbLink className="cursor-pointer" onClick={() => loadDirectory(segmentPath)}>
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
          </div>

          {/* Toolbar */}
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => loadDirectory('/home/root')}
              disabled={currentPath === '/home/root' || loading}
            >
              <Home className="h-4 w-4 mr-1" />
              Home
            </Button>
            <Button variant="outline" size="sm" onClick={handleUpload} disabled={loading}>
              <Upload className="h-4 w-4 mr-1" />
              Upload
            </Button>
            <Button variant="outline" size="sm" onClick={() => setNewFolderDialog(true)} disabled={loading}>
              <FolderPlus className="h-4 w-4 mr-1" />
              New Folder
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowHidden(!showHidden)}>
              {showHidden ? <EyeOff className="h-4 w-4 mr-1" /> : <Eye className="h-4 w-4 mr-1" />}
              {showHidden ? 'Hide Hidden' : 'Show Hidden'}
            </Button>
            <Button variant="outline" size="sm" onClick={() => loadDirectory(currentPath)} disabled={loading}>
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>

          {/* Error display */}
          {error && (
            <div className="bg-destructive/10 text-destructive text-sm p-2 rounded">
              {error}
              <Button variant="ghost" size="sm" className="ml-2 h-6" onClick={() => setError(null)}>
                Dismiss
              </Button>
            </div>
          )}

          {/* Transfer progress */}
          {transferProgress && (
            <div className="bg-muted p-3 rounded space-y-2">
              <div className="flex justify-between text-sm">
                <span>
                  {transferProgress.status === 'uploading' ? 'Uploading' : 'Downloading'}: {transferProgress.filename}
                </span>
                <span>{transferProgress.percentage.toFixed(0)}%</span>
              </div>
              <Progress value={transferProgress.percentage} />
              <div className="text-xs text-muted-foreground">
                {formatSize(transferProgress.bytesSent)} / {formatSize(transferProgress.totalBytes)}
              </div>
            </div>
          )}

          {/* File table */}
          <div className="border rounded-md overflow-hidden">
            <Table className="table-fixed w-full">
              <TableHeader>
                <TableRow>
                  <TableHead className="cursor-pointer select-none" onClick={() => handleSort('name')}>
                    <div className="flex items-center gap-1">
                      Name
                      {sortBy === 'name' ? (
                        sortOrder === 'asc' ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />
                      ) : (
                        <ArrowUpDown className="h-3 w-3 text-muted-foreground" />
                      )}
                    </div>
                  </TableHead>
                  <TableHead className="w-28 cursor-pointer select-none whitespace-nowrap" onClick={() => handleSort('size')}>
                    <div className="flex items-center gap-1">
                      Size
                      {sortBy === 'size' ? (
                        sortOrder === 'asc' ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />
                      ) : (
                        <ArrowUpDown className="h-3 w-3 text-muted-foreground" />
                      )}
                    </div>
                  </TableHead>
                  <TableHead className="w-28 cursor-pointer select-none hidden md:table-cell" onClick={() => handleSort('modTime')}>
                    <div className="flex items-center gap-1">
                      Modified
                      {sortBy === 'modTime' ? (
                        sortOrder === 'asc' ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />
                      ) : (
                        <ArrowUpDown className="h-3 w-3 text-muted-foreground" />
                      )}
                    </div>
                  </TableHead>
                  <TableHead className="w-12 pr-2"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {currentPath !== '/' && (
                  <TableRow className="cursor-pointer" onClick={handleGoUp}>
                    <TableCell className="flex items-center gap-2">
                      <ArrowUp className="h-4 w-4 text-muted-foreground" />
                      <span>..</span>
                    </TableCell>
                    <TableCell></TableCell>
                    <TableCell className="hidden md:table-cell"></TableCell>
                    <TableCell className="pr-2"></TableCell>
                  </TableRow>
                )}
                {displayFiles.map((file) => (
                  <TableRow
                    key={file.path}
                    className="cursor-pointer"
                    onClick={() => file.isDir && handleNavigate(file)}
                    onContextMenu={(e) => handleContextMenu(e, file)}
                  >
                    <TableCell className="flex items-center gap-2">
                      {getFileIcon(file)}
                      <span className="truncate">{file.name}</span>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{file.isDir ? '--' : formatSize(file.size)}</TableCell>
                    <TableCell className="hidden md:table-cell">{formatDate(file.modTime)}</TableCell>
                    <TableCell className="pr-2">
                      <Popover>
                        <PopoverTrigger asChild>
                          <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={(e) => e.stopPropagation()}>
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </PopoverTrigger>
                        <PopoverContent className={`${sleepScreenSupported && isPngFile(file) ? 'w-48' : 'w-40'} p-1`} align="end">
                          {!file.isDir && (
                            <PopoverClose asChild>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="w-full justify-start font-normal"
                                onClick={() => handleDownload(file)}
                              >
                                <Download className="h-4 w-4 mr-2" />
                                Download
                              </Button>
                            </PopoverClose>
                          )}
                          {sleepScreenSupported && isPngFile(file) && (
                            <PopoverClose asChild>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="w-full justify-start font-normal"
                                onClick={() => handleSetSleepScreen(file)}
                              >
                                <Image className="h-4 w-4 mr-2" />
                                Set as sleep screen
                              </Button>
                            </PopoverClose>
                          )}
                          <PopoverClose asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="w-full justify-start font-normal"
                              onClick={() => initiateRename(file)}
                            >
                              <Pencil className="h-4 w-4 mr-2" />
                              Rename
                            </Button>
                          </PopoverClose>
                          <PopoverClose asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="w-full justify-start font-normal"
                              onClick={() => copyPath(file.path)}
                            >
                              <Copy className="h-4 w-4 mr-2" />
                              Copy Path
                            </Button>
                          </PopoverClose>
                          <PopoverClose asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="w-full justify-start font-normal"
                              onClick={() => initiateDelete(file)}
                            >
                              <Trash2 className="h-4 w-4 mr-2" />
                              Delete
                            </Button>
                          </PopoverClose>
                        </PopoverContent>
                      </Popover>
                    </TableCell>
                  </TableRow>
                ))}
                {displayFiles.length === 0 && !loading && (
                  <TableRow>
                    <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                      Empty directory
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>

          {/* Context menu (positioned absolute) */}
          {contextMenu && (
            <>
              <div className="fixed inset-0 z-40" onClick={closeContextMenu} />
              <div
                className={`fixed bg-popover border rounded-md shadow-md p-1 z-50 ${sleepScreenSupported && isPngFile(contextMenu.file) ? 'w-48' : 'w-40'}`}
                style={{ left: contextMenu.x, top: contextMenu.y }}
              >
              {!contextMenu.file.isDir && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start font-normal"
                  onClick={() => {
                    handleDownload(contextMenu.file)
                    closeContextMenu()
                  }}
                >
                  <Download className="h-4 w-4 mr-2" />
                  Download
                </Button>
              )}
              {sleepScreenSupported && isPngFile(contextMenu.file) && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start font-normal"
                  onClick={() => {
                    handleSetSleepScreen(contextMenu.file)
                    closeContextMenu()
                  }}
                >
                  <Image className="h-4 w-4 mr-2" />
                  Set as sleep screen
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                className="w-full justify-start font-normal"
                onClick={() => {
                  initiateRename(contextMenu.file)
                  closeContextMenu()
                }}
              >
                <Pencil className="h-4 w-4 mr-2" />
                Rename
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="w-full justify-start font-normal"
                onClick={() => copyPath(contextMenu.file.path)}
              >
                <Copy className="h-4 w-4 mr-2" />
                Copy Path
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="w-full justify-start font-normal"
                onClick={() => {
                  initiateDelete(contextMenu.file)
                  closeContextMenu()
                }}
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            </div>
            </>
          )}

      {/* Delete confirmation dialog */}
      <Dialog open={deleteDialog !== null} onOpenChange={(open) => !open && setDeleteDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {deleteDialog?.isDir ? 'Folder' : 'File'}</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete "{deleteDialog?.name}"?
              {deleteDialog?.isDir && ' This will delete all contents inside.'}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rename dialog */}
      <Dialog open={renameDialog !== null} onOpenChange={(open) => !open && setRenameDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename</DialogTitle>
            <DialogDescription>
              Enter a new name for "{renameDialog?.name}"
            </DialogDescription>
          </DialogHeader>
          <Input
            value={renameName}
            onChange={(e) => setRenameName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleRename()}
            autoFocus
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameDialog(null)}>
              Cancel
            </Button>
            <Button onClick={handleRename}>Rename</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* New folder dialog */}
      <Dialog open={newFolderDialog} onOpenChange={setNewFolderDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Folder</DialogTitle>
            <DialogDescription>Enter a name for the new folder</DialogDescription>
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
            <Button onClick={handleCreateFolder}>Create</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* System file warning dialog */}
      <Dialog open={systemFileWarning !== null} onOpenChange={(open) => !open && setSystemFileWarning(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Modify System Files?</DialogTitle>
            <DialogDescription className="space-y-2 pt-2">
              <p>You are about to {systemFileWarning?.action} in the system partition.</p>
              <ul className="list-disc ml-5 space-y-1">
                <li>Modifying system files can cause your device to fail to boot or behave unexpectedly.</li>
                <li>Changes will be overwritten by OS updates.</li>
              </ul>
              <p>Only proceed if you know what you're doing.</p>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSystemFileWarning(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                systemFileWarning?.onConfirm()
                setSystemFileWarning(null)
              }}
            >
              Proceed Anyway
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Restart xochitl confirmation dialog */}
      <Dialog open={restartXochitlDialog !== null} onOpenChange={(open) => !open && setRestartXochitlDialog(null)} priority>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restart Required</DialogTitle>
            <DialogDescription>
              {restartXochitlDialog?.message}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setRestartXochitlDialog(null)}>
              Not Now
            </Button>
            <Button onClick={restartXochitlDialog?.onRestart}>
              Restart Now
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
