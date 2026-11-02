import { CaretDown } from '@phosphor-icons/react'
import { forwardRef, useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useScrollFades } from '@/hooks/useScrollFades'
import { useMediaQuery, NARROW_VIEWPORT } from '@/hooks/useMediaQuery'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

interface CategoryChipsProps {
  categories: string[]
  categoryCounts: Record<string, number>
  totalCount: number
  selected: string
  onSelect: (cat: string) => void
  highlightActive: boolean
  starredCount?: number
}

// Slugs come from the package index; a category added there must render without a code change here.
const ACRONYMS = new Set(['ui', 'os'])

// Must match gap-2 on both rows: the fit is measured off the mirror and applied to the visible row.
const CHIP_GAP = 8

function categoryLabel(slug: string): string {
  return slug
    .split('-')
    .map(word => (ACRONYMS.has(word) ? word.toUpperCase() : word.charAt(0).toUpperCase() + word.slice(1)))
    .join(' ')
}

const chipClass = (active: boolean) => cn(
  'inline-flex items-center gap-1.5 px-3 py-1.5 touch:hit-target rounded-[calc(var(--radius)-2px)] text-sm font-medium',
  'border cursor-pointer transition-all duration-150 whitespace-nowrap shrink-0',
  active
    ? 'bg-primary text-primary-foreground border-primary'
    : 'bg-card text-card-foreground border-border hover:bg-accent',
)

function ChipCount({ count, active }: { count: number; active: boolean }) {
  return (
    <span className={cn('text-[11px] relative top-[0.5px]', active ? 'opacity-80' : 'opacity-60')}>
      {count}<span className="sr-only"> mods</span>
    </span>
  )
}

function Chip({ label, count, active, onClick }: { label: string; count?: number | null; active: boolean; onClick?: () => void }) {
  return (
    <button onClick={onClick} aria-pressed={active} className={chipClass(active)}>
      {label}
      {count != null && <ChipCount count={count} active={active} />}
    </button>
  )
}

// DropdownMenuTrigger mounts this with `asChild`, so the click handler, ARIA state and ref arrive as props.
const MoreChip = forwardRef<HTMLButtonElement, React.ComponentPropsWithoutRef<'button'> & { active?: boolean }>(
  ({ className, children, active = false, ...props }, ref) => (
    <button ref={ref} {...props} className={cn(chipClass(active), className)}>
      More
      <CaretDown className={cn('h-3 w-3', active ? 'opacity-80' : 'opacity-60')} weight="bold" />
      {children}
    </button>
  ),
)
MoreChip.displayName = 'MoreChip'

export function CategoryChips({ categories, categoryCounts, totalCount, selected, onSelect, highlightActive, starredCount = 0 }: CategoryChipsProps) {
  const { scrollRef, fadeLeft, fadeRight } = useScrollFades<HTMLDivElement>()
  const mirrorRef = useRef<HTMLDivElement>(null)
  const pinnedChips: { key: string; label: string; count: number | null }[] = [
    { key: 'overview', label: 'Overview', count: null },
    { key: 'all', label: 'All', count: totalCount },
    { key: 'starred', label: 'Starred', count: starredCount },
  ]
  const pinnedCount = pinnedChips.length
  const narrow = useMediaQuery(NARROW_VIEWPORT)
  const [fit, setFit] = useState(categories.length)

  const measure = useCallback(() => {
    const row = scrollRef.current
    const mirror = mirrorRef.current
    if (!row || !mirror) return

    const style = getComputedStyle(row)
    const available = row.clientWidth - (parseFloat(style.paddingLeft) || 0) - (parseFloat(style.paddingRight) || 0)
    const widths = Array.from(mirror.children, el => el.getBoundingClientRect().width)
    if (widths.length !== pinnedCount + categories.length + 1) return

    const catWidths = widths.slice(pinnedCount, pinnedCount + categories.length)
    const pinned = widths.slice(0, pinnedCount).reduce((sum, w, i) => sum + (i ? CHIP_GAP : 0) + w, 0)
    const moreWidth = widths[widths.length - 1]

    const everything = catWidths.reduce((sum, w) => sum + CHIP_GAP + w, pinned)
    if (everything <= available) {
      setFit(categories.length)
      return
    }

    let used = pinned + CHIP_GAP + moreWidth
    let count = 0
    while (count < catWidths.length && used + CHIP_GAP + catWidths[count] <= available) {
      used += CHIP_GAP + catWidths[count]
      count++
    }
    setFit(count)
  }, [categories.length, pinnedCount, scrollRef])

  useLayoutEffect(() => {
    if (narrow) {
      setFit(categories.length)
      return
    }
    const row = scrollRef.current
    const mirror = mirrorRef.current
    if (!row || !mirror) return

    measure()
    // The mirror is observed too: counts arrive with the package data and the font can swap afterward, either of which rewidths every chip.
    const observer = new ResizeObserver(measure)
    observer.observe(row)
    observer.observe(mirror)
    return () => observer.disconnect()
  }, [narrow, measure, categories.length, categoryCounts, totalCount, starredCount, scrollRef])

  const shown = narrow ? categories : categories.slice(0, fit)
  const overflow = narrow ? [] : categories.slice(fit)

  const categoryChip = (cat: string) => (
    <Chip
      key={cat}
      label={categoryLabel(cat)}
      count={categoryCounts[cat] || 0}
      active={highlightActive && selected === cat}
      onClick={() => onSelect(cat)}
    />
  )

  return (
    <>
      {/* Hidden, not unmounted: an unmounted chip has no width to fit against, and invisible keeps the duplicates out of the tab order. */}
      <div className="relative h-0 overflow-hidden" aria-hidden>
        <div ref={mirrorRef} className="absolute left-0 top-0 flex gap-2 invisible">
          {pinnedChips.map(chip => <Chip key={chip.key} label={chip.label} count={chip.count} active={false} />)}
          {categories.map(cat => (
            <Chip key={cat} label={categoryLabel(cat)} count={categoryCounts[cat] || 0} active={false} />
          ))}
          <MoreChip />
        </div>
      </div>

      <div
        ref={scrollRef}
        className={cn(
          'flex gap-2 overflow-x-auto overflow-y-hidden overscroll-x-none [scrollbar-width:none] [&::-webkit-scrollbar]:hidden md:overflow-visible',
          // overflow-x:auto forces overflow-y:auto, which clips the chips' touch overlay to the rail.
          '[--room:calc((var(--hit-target)_-_34px)/2)]',
          '-mt-[var(--room)] pt-[var(--room)] pb-[var(--room)] mb-[calc(24px_-_var(--room))] md:pb-0 md:mb-5',
          '-ml-gutter-left -mr-gutter-right pl-gutter-left pr-gutter-right',
          fadeLeft && 'scroll-fade-left',
          fadeRight && 'scroll-fade-right',
        )}
      >
        {pinnedChips.map(chip => (
          <Chip
            key={chip.key}
            label={chip.label}
            count={chip.count}
            active={highlightActive && selected === chip.key}
            onClick={() => onSelect(chip.key)}
          />
        ))}
        {shown.map(categoryChip)}
        {overflow.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <MoreChip active={highlightActive && overflow.includes(selected)}>
                <span className="sr-only">
                  {overflow.includes(selected)
                    ? `${categoryLabel(selected)} selected, ${overflow.length} more categories`
                    : `${overflow.length} more categories`}
                </span>
              </MoreChip>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {overflow.map(cat => (
                <DropdownMenuItem
                  key={cat}
                  onSelect={() => onSelect(cat)}
                  aria-current={highlightActive && selected === cat}
                  className={cn('cursor-pointer justify-between gap-4', highlightActive && selected === cat && 'font-semibold')}
                >
                  {categoryLabel(cat)}
                  <ChipCount count={categoryCounts[cat] || 0} active={false} />
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </>
  )
}

export { categoryLabel }
