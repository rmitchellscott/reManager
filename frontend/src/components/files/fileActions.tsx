import { Copy, DownloadSimple, Image, Pencil, Trash } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import type { FileInfo } from '@/lib/types'

export interface FileAction {
  id: string
  label: string
  Icon: Icon
  destructive?: boolean
  run: () => void
}

export interface FileActionHandlers {
  onDownload: (file: FileInfo) => void
  onDownloadFolder: (file: FileInfo) => void
  onSetSleepScreen: (file: FileInfo) => void
  onRename: (file: FileInfo) => void
  onCopyPath: (path: string) => void
  onDelete: (file: FileInfo) => void
}

export interface FileActionContext {
  sleepScreenSupported: boolean
  readOnly: boolean
}

export function fileActions(
  file: FileInfo,
  { sleepScreenSupported, readOnly }: FileActionContext,
  handlers: FileActionHandlers
): FileAction[] {
  const isPng = !file.isDir && file.name.toLowerCase().endsWith('.png')

  const actions: FileAction[] = [
    file.isDir
      ? { id: 'download', label: 'Download folder', Icon: DownloadSimple, run: () => handlers.onDownloadFolder(file) }
      : { id: 'download', label: 'Download', Icon: DownloadSimple, run: () => handlers.onDownload(file) },
  ]

  if (sleepScreenSupported && isPng) {
    actions.push({ id: 'sleep', label: 'Set as sleep screen', Icon: Image, run: () => handlers.onSetSleepScreen(file) })
  }

  actions.push({ id: 'copy', label: 'Copy path', Icon: Copy, run: () => handlers.onCopyPath(file.path) })

  if (!readOnly) {
    actions.push(
      { id: 'rename', label: 'Rename', Icon: Pencil, run: () => handlers.onRename(file) },
      { id: 'delete', label: 'Delete', Icon: Trash, destructive: true, run: () => handlers.onDelete(file) },
    )
  }

  return actions
}
