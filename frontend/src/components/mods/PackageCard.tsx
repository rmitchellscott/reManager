import { ArrowRight, ArrowUp, Check, Clock, DownloadSimple, Plus, Star, Trash, X } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { StatusBadge } from '@/components/StatusBadge'
import { categoryLabel } from './CategoryChips'
import { memo, ReactNode } from 'react'
import { PackageInfo } from '@/lib/types'
import { prefetchContent } from '@/lib/contentCache'
import { cn } from '@/lib/utils'
import { relativeTime } from '@/lib/format'
import { formatCount, incompatibleReasonLabel } from '@/lib/format'

export type CardStatus = 'installed' | 'queued' | 'implied' | 'update' | 'remove' | 'incompatible' | 'none'
export type CardContext = 'discover' | 'installed' | 'updates'

interface PackageCardProps {
  pkg: PackageInfo
  status: CardStatus
  context: CardContext
  variant: 'grid' | 'row'
  wide?: boolean
  onClick: (pkg: PackageInfo) => void
  onAction: (name: string) => void
  onAuthorClick?: (author: string) => void
  starred?: boolean
  hideAction?: boolean
  onToggleStar?: (name: string) => void
  conflict?: string | null
  disabled?: boolean
  installedVersion?: string
  targetVersion?: string
  installTime?: number
  activeCategory?: string
  dlField?: 'downloads30d' | 'downloadsAll'
  timeField?: 'released' | 'firstReleased'
  plan?: boolean
  trailing?: ReactNode
  muted?: boolean
  applied?: boolean
  seen?: boolean
  onToggleSeen?: (name: string) => void
  note?: ReactNode
  conflicted?: boolean
}

// Width is split from the height so a text action can override w-9 with w-full; cn collapses the base width but sm:w-8 is a separate variant that still wins above sm.
export const cardActionHeight = 'h-9 p-0 sm:h-8 shrink-0 hit-target'

const cardActionSize = `${cardActionHeight} w-9 sm:w-8`

export const cardFocusRing = 'has-[[data-card-trigger]:focus-visible]:inset-ring-2 has-[[data-card-trigger]:focus-visible]:inset-ring-[var(--ring)]'

export const cardTrigger = 'min-w-0 text-left focus-visible:outline-none'

function timeAgo(dateStr: string): string {
  if (!dateStr) return ''
  return relativeTime(new Date(dateStr).getTime() / 1000)
}


export function StarToggle({ starred, onToggle, name }: { starred?: boolean; onToggle: (name: string) => void; name: string }) {
  return (
    <span
      role="checkbox"
      aria-checked={!!starred}
      aria-label={starred ? `Unstar ${name}` : `Star ${name}`}
      tabIndex={0}
      className={cn(
        'hit-target shrink-0 cursor-pointer',
        starred ? 'text-foreground' : 'text-muted-foreground/50 hover:text-muted-foreground',
      )}
      onClick={(e) => { e.stopPropagation(); onToggle(name) }}
      onKeyDown={(e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return
        e.preventDefault()
        e.stopPropagation()
        onToggle(name)
      }}
    >
      <Star className="h-4 w-4" weight={starred ? 'fill' : 'regular'} />
    </span>
  )
}

function IconAction({ action, label, onClick, disabled, children }: {
  action: string; label: string; onClick: (e: React.MouseEvent) => void; disabled?: boolean; children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="outline" size="sm" className={cardActionSize} onClick={onClick} disabled={disabled} aria-label={label}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{action}</TooltipContent>
    </Tooltip>
  )
}

function ActionButton({ pkg, status, context, conflict, disabled, onAction }: {
  pkg: PackageInfo
  status: CardStatus
  context: CardContext
  conflict?: string | null
  disabled?: boolean
  onAction: () => void
}) {
  if (status === 'remove') {
    return (
      <IconAction action="Keep" label={`Keep ${pkg.name}`} onClick={(e) => { e.stopPropagation(); onAction() }}>
        <X className="h-4 w-4" />
      </IconAction>
    )
  }

  if (status === 'implied') {
    return <Badge variant="outline" className="text-xs font-normal">{context === 'installed' ? 'Also removed' : 'Dependency'}</Badge>
  }

  if (context === 'updates') {
    return status === 'queued' ? (
      <IconAction action="Don't upgrade" label={`Don't upgrade ${pkg.name}`} onClick={(e) => { e.stopPropagation(); onAction() }}>
        <X className="h-4 w-4" />
      </IconAction>
    ) : (
      <IconAction action="Add upgrade" label={`Add ${pkg.name} upgrade`} onClick={(e) => { e.stopPropagation(); onAction() }} disabled={disabled}>
        <ArrowUp className="h-4 w-4" />
      </IconAction>
    )
  }

  if (context === 'installed') {
    return (
      <IconAction action="Remove" label={`Remove ${pkg.name}`} onClick={(e) => { e.stopPropagation(); onAction() }} disabled={disabled}>
        <Trash className="h-4 w-4" />
      </IconAction>
    )
  }

  if (status === 'installed') {
    return <Badge variant="secondary" className="text-xs">Installed</Badge>
  }

  if (status === 'incompatible') {
    return <Badge variant="secondary" className="text-xs font-normal">{incompatibleReasonLabel(pkg)}</Badge>
  }

  if (conflict) {
    return <Badge variant="secondary" className="text-xs font-normal">Conflicts</Badge>
  }

  return (
    <IconAction
      action={status === 'queued' ? "Don't install" : 'Add'}
      label={status === 'queued' ? `Don't install ${pkg.name}` : `Add ${pkg.name}`}
      onClick={(e) => { e.stopPropagation(); onAction() }}
      disabled={disabled}
    >
      {status === 'queued' ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
    </IconAction>
  )
}

export const PackageCard = memo(function PackageCard({
  pkg, status, context, variant, wide, onClick, onAction, onAuthorClick, starred, onToggleStar, hideAction,
  conflict, disabled, installedVersion, targetVersion, installTime, activeCategory,
  dlField = 'downloadsAll', timeField = 'released', plan = false, trailing, muted, applied = false, seen = false, onToggleSeen,
  note, conflicted = false,
}: PackageCardProps) {
  const filteredCategories = (pkg.categories || []).filter(c => c !== activeCategory)
  const dlValue = dlField === 'downloads30d' ? pkg.downloads30d : pkg.downloadsAll
  const timeValue = timeField === 'firstReleased' ? pkg.firstReleased : pkg.released
  const dlTooltip = dlField === 'downloads30d' ? 'Downloads last 30 days' : 'Total downloads'
  const timeTooltip = timeField === 'firstReleased' ? 'Added to catalog' : 'Last updated'
  const installTimeStr = installTime ? timeAgo(new Date(installTime * 1000).toISOString()) : null
  const timeLabel = context === 'installed' && installTimeStr ? installTimeStr : timeAgo(timeValue)
  const upgradeTarget = targetVersion ?? pkg.version
  const dlLabel = dlField === 'downloads30d' ? `${formatCount(dlValue)}/mo` : formatCount(dlValue)
  const openDetails = () => onClick(pkg)
  const warmReadme = () => prefetchContent(pkg.readmeUrl)
  const runAction = () => onAction(pkg.name)

  const queuedIsUpgrade = context === 'updates'

  const flagIncompatible = context === 'installed'

  const dimmed = status === 'incompatible' && !flagIncompatible

  // index.css draws the row accent from the data-accent attribute set below.
  const rowAccentName = {
    installed: '',
    queued: queuedIsUpgrade ? 'info' : 'success',
    implied: 'implied',
    update: '',
    remove: 'destructive',
    incompatible: flagIncompatible ? 'warning' : '',
    none: '',
  }[plan ? 'none' : status]

  const cardAccent = {
    installed: '',
    queued: queuedIsUpgrade
      ? 'border-[var(--info)] inset-ring-1 inset-ring-[var(--info)]'
      : 'border-[var(--success)] inset-ring-1 inset-ring-[var(--success)]',
    implied: 'border-[oklch(from_var(--success)_l_c_h_/_40%)] inset-ring-1 inset-ring-[oklch(from_var(--success)_l_c_h_/_40%)]',
    update: '',
    remove: 'border-[var(--destructive)] inset-ring-1 inset-ring-[var(--destructive)]',
    incompatible: flagIncompatible ? 'border-[var(--warning)] inset-ring-1 inset-ring-[var(--warning)]' : '',
    none: '',
  }[plan ? 'none' : status]

  const accented = cardAccent !== ''

  const seenMute = seen && 'opacity-70'

  const unreadWash = onToggleSeen && !seen && 'bg-[var(--unread-surface)]'

  if (variant === 'row') {
    return (
      <div
        className={cn(
          'flex flex-wrap items-center gap-x-4 gap-y-1 px-3.5 py-2.5 cursor-pointer transition-colors',
          'border border-transparent [border-bottom-color:var(--border)] last:[border-bottom-color:transparent]',
          'first:rounded-t-[var(--radius)] last:rounded-b-[var(--radius)] only:rounded-[var(--radius)]',
          (plan && !applied) || rowAccentName ? 'hover:bg-foreground/[0.04]' : 'hover:bg-muted',
          applied && 'bg-card',
          muted && 'opacity-45 transition-opacity duration-300',
          cardFocusRing,
          dimmed && 'text-muted-foreground',
        )}
        data-accent={conflicted ? 'destructive' : rowAccentName || undefined}
        onClick={openDetails}
        onPointerEnter={warmReadme}
        onPointerDown={warmReadme}
      >
        <div className="flex-1 min-w-[55%]">
          <div className="flex items-center gap-2 flex-wrap">
            <button type="button" data-card-trigger className={cn(cardTrigger, 'text-sm font-semibold break-words')}>{pkg.name}</button>
            {!plan && filteredCategories.map(c => <Badge key={c} variant="outline" className="text-xs px-1.5 py-0">{categoryLabel(c)}</Badge>)}
            <StatusBadge status={pkg.status} />
            {context === 'installed' && status === 'incompatible' && (
              <Badge variant="secondary" className="text-xs font-normal">{incompatibleReasonLabel(pkg)}</Badge>
            )}
          </div>
          {context === 'updates' && installedVersion && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5">
              v{installedVersion}
              <ArrowRight className="h-3 w-3" />
              <span className={cn('font-semibold', applied ? 'text-foreground' : 'text-[var(--info-text)]')}>v{upgradeTarget}</span>
            </div>
          )}
          {!plan && <p className="text-xs text-muted-foreground mt-0.5 truncate">{pkg.description}</p>}
          {note && <p className={cn('text-xs mt-0.5', conflicted ? 'text-destructive' : 'text-muted-foreground')}>{note}</p>}
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2 min-w-0 ml-auto">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className={cn("max-sm:hidden text-xs text-muted-foreground flex items-center gap-1 min-w-14 justify-end", plan && "hidden")}>
                <DownloadSimple className="h-[11px] w-[11px]" /> {dlLabel}
              </span>
            </TooltipTrigger>
            <TooltipContent>{dlTooltip}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className={cn("max-sm:hidden text-xs text-muted-foreground flex items-center gap-1 w-16 justify-end whitespace-nowrap", plan && "hidden")}>
                {timeLabel && <><Clock className="h-[11px] w-[11px]" /> {timeLabel}</>}
              </span>
            </TooltipTrigger>
            <TooltipContent>{context === 'installed' && installTimeStr ? 'Installed' : timeTooltip}</TooltipContent>
          </Tooltip>
          <div className="flex items-center justify-end gap-4" onClick={(e) => e.stopPropagation()}>
            {onToggleStar && <StarToggle starred={starred} onToggle={onToggleStar} name={pkg.name} />}
            {!hideAction && (trailing ?? <ActionButton pkg={pkg} status={status} context={context} conflict={conflict} disabled={disabled} onAction={runAction} />)}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'rounded-[var(--radius)] border border-border bg-card text-card-foreground px-3.5 py-3 cursor-pointer transition-[border-color,box-shadow,background-color] overflow-hidden',
        unreadWash,
        '@container',
        'hover:shadow-[0_2px_8px_rgba(0,0,0,0.06)] hover:z-[5]',
        !accented && 'hover:border-[var(--ring)]',
        'flex flex-col relative',
        cardFocusRing,
        cardAccent,
        dimmed && 'text-muted-foreground',
        wide && 'w-[min(17.5rem,78vw)] flex-shrink-0 min-h-[11.25rem]',
      )}
      onClick={openDetails}
      onPointerEnter={warmReadme}
      onPointerDown={warmReadme}
    >
      <div className={cn('flex justify-between items-start gap-2', wide ? 'mb-3' : 'mb-1')}>
        <div className="flex-1 min-w-0">
          <span className={cn('flex items-center gap-1.5 text-sm font-semibold', dimmed && 'text-muted-foreground', seenMute)}>
            <button type="button" data-card-trigger className={cn(cardTrigger, 'truncate')}>{pkg.name}</button>
          </span>
          {(filteredCategories.length > 0 || pkg.status !== 'maintained' || (context === 'installed' && status === 'incompatible')) && (
            <div className={cn('flex items-center gap-1 flex-wrap mt-1', seenMute)}>
              {filteredCategories.map(c => <Badge key={c} variant="outline" className="text-xs px-1.5 py-0">{categoryLabel(c)}</Badge>)}
              <StatusBadge status={pkg.status} />
              {context === 'installed' && status === 'incompatible' && (
                <Badge variant="secondary" className="text-xs font-normal">{incompatibleReasonLabel(pkg)}</Badge>
              )}
            </div>
          )}
        </div>
        <div className="flex flex-shrink-0 items-center gap-4" onClick={(e) => e.stopPropagation()}>
          {onToggleStar && <StarToggle starred={starred} onToggle={onToggleStar} name={pkg.name} />}
          {!hideAction && (trailing ?? <ActionButton pkg={pkg} status={status} context={context} conflict={conflict} disabled={disabled} onAction={runAction} />)}
        </div>
      </div>
      <p className={cn('text-xs mb-1.5 line-clamp-3 text-muted-foreground', seenMute)}>
        {pkg.description}
      </p>
      {context === 'updates' && installedVersion && (
        <div className="flex items-center gap-1 mb-1">
          <span className="text-xs text-muted-foreground">v{installedVersion}</span>
          <ArrowRight className="h-3 w-3 text-muted-foreground" />
          <span className="text-xs font-semibold text-[var(--info-text)]">v{upgradeTarget}</span>
        </div>
      )}
      <div className="h-px bg-border mt-auto" />
      <div className={cn('flex items-center justify-between pt-2', seenMute)}>
        {pkg.upstreamAuthor && onAuthorClick ? (
          <button
            className="text-xs text-muted-foreground cursor-pointer hover:text-foreground transition-colors truncate bg-transparent border-none p-0"
            aria-label={`Mods by ${pkg.upstreamAuthor}`}
            onClick={(e) => { e.stopPropagation(); onAuthorClick(pkg.upstreamAuthor) }}
          >
            {pkg.upstreamAuthor}
          </button>
        ) : pkg.upstreamAuthor ? (
          <span className="text-xs text-muted-foreground truncate">{pkg.upstreamAuthor}</span>
        ) : <span />}
        <div className="flex items-center gap-3 flex-shrink-0">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-xs text-muted-foreground flex items-center gap-1 whitespace-nowrap @max-[10em]:hidden">
                <DownloadSimple className="h-3 w-3" /> {dlLabel}
              </span>
            </TooltipTrigger>
            <TooltipContent>{dlTooltip}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-xs text-muted-foreground flex items-center gap-1 whitespace-nowrap @max-[15em]:hidden">
                {timeLabel && <><Clock className="h-3 w-3" /> {timeLabel}</>}
              </span>
            </TooltipTrigger>
            <TooltipContent>{context === 'installed' && installTimeStr ? 'Installed' : timeTooltip}</TooltipContent>
          </Tooltip>
        </div>
      </div>
      {onToggleSeen && (
        <button
          className={cn(
            cardActionHeight,
            'inline-flex items-center justify-center gap-1 text-xs font-medium w-full rounded-[calc(var(--radius)-4px)] cursor-pointer border transition-all duration-150 mt-2',
            seen
              ? 'bg-secondary text-muted-foreground border-transparent hover:bg-accent'
              : 'bg-card text-muted-foreground border-border hover:bg-accent hover:text-foreground',
          )}
          aria-pressed={seen}
          aria-label={seen ? `Mark ${pkg.name} unseen` : `Mark ${pkg.name} seen`}
          onClick={(e) => { e.stopPropagation(); onToggleSeen(pkg.name) }}
        >
          {seen && <Check className="h-3.5 w-3.5" />}
          {seen ? 'Seen' : 'Mark seen'}
        </button>
      )}
    </div>
  )
})
