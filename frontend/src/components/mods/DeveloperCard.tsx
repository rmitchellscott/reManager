import { DownloadSimple, Package } from '@phosphor-icons/react'
import { formatCount } from '@/lib/format'
import { cn } from '@/lib/utils'
import { cardActionHeight, cardFocusRing, cardTrigger } from './PackageCard'

interface DeveloperCardProps {
  name: string
  packageCount: number
  totalDownloads: number
  isFollowing: boolean
  onToggleFollow: () => void
  onClick: () => void
}

export function DeveloperCard({ name, packageCount, totalDownloads, isFollowing, onToggleFollow, onClick }: DeveloperCardProps) {
  return (
    <div
      className={cn(
        'relative rounded-[var(--radius)] border border-border bg-card px-3 py-3 w-[190px] flex-shrink-0 cursor-pointer hover:border-[var(--ring)] hover:shadow-[0_2px_8px_rgba(0,0,0,0.06)] transition-[border-color,box-shadow] flex flex-col',
        cardFocusRing,
      )}
      onClick={onClick}
    >
      <button type="button" data-card-trigger aria-label={`Mods by ${name}`} className={cn(cardTrigger, 'block w-full text-sm font-semibold truncate')}>{name}</button>
      <div className="flex-grow mt-0.5">
        <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Package className="h-3 w-3" />{packageCount}<span className="sr-only"> mods</span>
          </span>
          <span className="flex items-center gap-1">
            <DownloadSimple className="h-3 w-3" />{formatCount(totalDownloads)}<span className="sr-only"> downloads</span>
          </span>
        </div>
      </div>
      <div className="h-px bg-border my-2" />
      <button
        className={cn(
          cardActionHeight,
          'inline-flex items-center justify-center gap-1 text-xs font-medium w-full rounded-[calc(var(--radius)-4px)] cursor-pointer border transition-all duration-150',
          isFollowing
            ? 'bg-primary text-primary-foreground border-primary'
            : 'bg-transparent text-muted-foreground border-border hover:bg-accent hover:text-foreground',
        )}
        aria-pressed={isFollowing}
        aria-label={`Follow ${name}`}
        onClick={(e) => { e.stopPropagation(); onToggleFollow() }}
      >
        {isFollowing ? 'Following' : 'Follow'}
      </button>
    </div>
  )
}
