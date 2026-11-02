import { ArrowDown, ArrowUp, ArrowsDownUp, CaretRight, DotsThreeVertical } from '@phosphor-icons/react'
import { useRef } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { formatBytes, formatDate, formatDateFull } from '@/lib/format'
import { FileIcon } from './fileIcons'
import { fileActions, type FileActionContext, type FileActionHandlers } from './fileActions'
import type { FileInfo } from '@/lib/types'
import type { SortColumn, SortOrder } from './types'

interface FileListProps {
  files: FileInfo[]
  loading: boolean
  narrow: boolean
  currentPath: string
  sortBy: SortColumn
  sortOrder: SortOrder
  onSort: (column: SortColumn) => void
  onNavigate: (file: FileInfo) => void
  onGoUp: () => void
  selection: Set<string>
  selecting: boolean
  onToggleSelect: (file: FileInfo, shiftKey: boolean) => void
  onLongPress: (file: FileInfo) => void
  currentSleepScreen: string | null
  actionContext: FileActionContext
  handlers: FileActionHandlers
  pickMode: boolean
  onPick: (file: FileInfo) => void
}

function SortGlyph({ active, order }: { active: boolean; order: SortOrder }) {
  if (!active) return <ArrowsDownUp className="h-3 w-3 text-muted-foreground" />
  return order === 'asc' ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />
}

const rowActivator = 'min-w-0 rounded-sm text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

function SortableHead({ column, label, sortBy, sortOrder, onSort, className }: {
  column: SortColumn
  label: string
  sortBy: SortColumn
  sortOrder: SortOrder
  onSort: (column: SortColumn) => void
  className?: string
}) {
  const active = sortBy === column
  return (
    <TableHead className={className} aria-sort={active ? (sortOrder === 'asc' ? 'ascending' : 'descending') : undefined}>
      <button type="button" className="flex w-full select-none items-center gap-1 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => onSort(column)}>
        {label}
        <SortGlyph active={active} order={sortOrder} />
      </button>
    </TableHead>
  )
}

function ActionsMenu({ file, actionContext, handlers }: {
  file: FileInfo
  actionContext: FileActionContext
  handlers: FileActionHandlers
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 w-8 p-0 touch:hit-target"
          onClick={(e) => e.stopPropagation()}
          aria-label={`Actions for ${file.name}`}
        >
          <DotsThreeVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
        {fileActions(file, actionContext, handlers).map(({ id, label, Icon, destructive, run }) => (
          <DropdownMenuItem key={id} onSelect={run} className={destructive ? 'text-destructive' : undefined}>
            <Icon className="h-4 w-4" />
            {label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function FileList(props: FileListProps) {
  const {
    files, loading, narrow, currentPath, sortBy, sortOrder, onSort, onNavigate, onGoUp,
    selection, selecting, onToggleSelect, onLongPress, currentSleepScreen,
    actionContext, handlers, pickMode, onPick,
  } = props

  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearLongPress = () => {
    if (longPressTimer.current) clearTimeout(longPressTimer.current)
    longPressTimer.current = null
  }

  const longPressHandlers = (file: FileInfo) => ({
    onTouchStart: () => {
      clearLongPress()
      longPressTimer.current = setTimeout(() => onLongPress(file), 500)
    },
    onTouchMove: clearLongPress,
    onTouchEnd: clearLongPress,
    onTouchCancel: clearLongPress,
  })

  const showParent = currentPath !== '/'

  const activate = (file: FileInfo, shiftKey: boolean) => {
    if (selecting) {
      onToggleSelect(file, shiftKey)
      return
    }
    if (file.isDir) {
      onNavigate(file)
      return
    }
    if (pickMode) onPick(file)
  }

  const isPickable = (file: FileInfo) => !pickMode || file.isDir || file.name.toLowerCase().endsWith('.png')

  const rows = files.filter(isPickable)

  if (narrow) {
    return (
      <div className="rounded-[var(--radius)] border bg-card divide-y overflow-hidden">
        {showParent && (
          <button
            className="flex w-full items-center gap-3 px-3.5 py-2.5 text-left min-h-[var(--hit-target)] hover:bg-accent"
            onClick={onGoUp}
          >
            <ArrowUp className="h-4 w-4 shrink-0 text-muted-foreground" />
            <span className="text-sm">..</span>
          </button>
        )}
        {rows.map((file) => {
          const selected = selection.has(file.path)
          return (
            <div
              key={file.path}
              className={`flex items-center gap-3 px-3.5 py-2.5 min-h-[var(--hit-target)] select-none [-webkit-touch-callout:none] ${selected ? 'bg-accent' : ''}`}
              onClick={(e) => activate(file, e.shiftKey)}
              {...longPressHandlers(file)}
            >
              {selecting ? (
                <span className="shrink-0" onClick={(e) => e.stopPropagation()}>
                  <Checkbox
                    checked={selected}
                    className="touch:hit-target"
                    onCheckedChange={() => onToggleSelect(file, false)}
                    aria-label={`Select ${file.name}`}
                  />
                </span>
              ) : (
                <FileIcon file={file} className="h-5 w-5 shrink-0" />
              )}
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2">
                  <button type="button" className={`${rowActivator} truncate text-sm`}>{file.name}</button>
                  {currentSleepScreen === file.path && (
                    <Badge variant="secondary" className="shrink-0">Sleep screen</Badge>
                  )}
                </div>
                <p className="truncate text-xs text-muted-foreground">
                  {file.isDir ? 'Folder' : formatBytes(file.size)}
                  {' · '}
                  {formatDate(file.modTime)}
                </p>
              </div>
              {!selecting && !pickMode && (
                file.isDir
                  ? <CaretRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                  : null
              )}
              {!selecting && !pickMode && (
                <ActionsMenu
                  file={file}
                  actionContext={actionContext}
                  handlers={handlers}
                />
              )}
            </div>
          )
        })}
        {rows.length === 0 && !loading && (
          <p className="py-8 text-center text-sm text-muted-foreground">Empty folder</p>
        )}
      </div>
    )
  }

  return (
    <div className="border rounded-[var(--radius)] overflow-hidden">
      <Table className="table-fixed w-full">
        <TableHeader>
          <TableRow>
            {selecting && <TableHead className="w-10" />}
            <SortableHead column="name" label="Name" sortBy={sortBy} sortOrder={sortOrder} onSort={onSort} />
            <SortableHead column="size" label="Size" sortBy={sortBy} sortOrder={sortOrder} onSort={onSort} className="w-28 whitespace-nowrap" />
            <SortableHead column="modTime" label="Modified" sortBy={sortBy} sortOrder={sortOrder} onSort={onSort} className="w-28" />
            <TableHead className="w-12 pr-2" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {showParent && (
            <TableRow className="cursor-pointer" onClick={onGoUp}>
              {selecting && <TableCell />}
              <TableCell>
                <button type="button" className={`${rowActivator} flex items-center gap-2`} aria-label="Parent folder">
                  <ArrowUp className="h-4 w-4 text-muted-foreground" />
                  <span>..</span>
                </button>
              </TableCell>
              <TableCell />
              <TableCell />
              <TableCell className="pr-2" />
            </TableRow>
          )}
          {rows.map((file) => {
            const selected = selection.has(file.path)
            return (
              <ContextMenu key={file.path}>
                <ContextMenuTrigger asChild disabled={pickMode}>
                  <TableRow
                    className={`cursor-pointer ${selected ? 'bg-accent' : ''}`}
                    onClick={(e) => activate(file, e.shiftKey)}
                    onContextMenu={(e) => {
                      if (pickMode) e.preventDefault()
                    }}
                  >
                    {selecting && (
                      <TableCell onClick={(e) => e.stopPropagation()}>
                        <Checkbox
                          checked={selected}
                          onCheckedChange={() => onToggleSelect(file, false)}
                          aria-label={`Select ${file.name}`}
                        />
                      </TableCell>
                    )}
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <FileIcon file={file} />
                        <button type="button" className={`${rowActivator} truncate`}>{file.name}</button>
                        {currentSleepScreen === file.path && (
                          <Badge variant="secondary" className="shrink-0">Sleep screen</Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap">{file.isDir ? '' : formatBytes(file.size)}</TableCell>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span>{formatDate(file.modTime)}</span>
                        </TooltipTrigger>
                        <TooltipContent>{formatDateFull(file.modTime)}</TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell className="pr-2">
                      {!pickMode && (
                        <ActionsMenu
                          file={file}
                          actionContext={actionContext}
                          handlers={handlers}
                        />
                      )}
                    </TableCell>
                  </TableRow>
                </ContextMenuTrigger>
                <ContextMenuContent onClick={(e) => e.stopPropagation()}>
                  {fileActions(file, actionContext, handlers).map(({ id, label, Icon, destructive, run }) => (
                    <ContextMenuItem key={id} onSelect={run} className={destructive ? 'text-destructive' : undefined}>
                      <Icon className="h-4 w-4" />
                      {label}
                    </ContextMenuItem>
                  ))}
                </ContextMenuContent>
              </ContextMenu>
            )
          })}
          {rows.length === 0 && !loading && (
            <TableRow>
              <TableCell colSpan={selecting ? 5 : 4} className="text-center text-muted-foreground py-8">
                Empty folder
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )
}
