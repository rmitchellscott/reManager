import { CaretLeft, CaretRight } from '@phosphor-icons/react'
import { type ReactNode } from 'react'
import { useScrollFades } from '@/hooks/useScrollFades'
import { cn, prefersReducedMotion } from '@/lib/utils'

interface ScrollRowProps {
  title: string
  children: ReactNode
  onViewAll?: () => void
}

export function ScrollRow({ title, children, onViewAll }: ScrollRowProps) {
  const { scrollRef, fadeLeft, fadeRight } = useScrollFades<HTMLDivElement>()

  const scroll = (dir: -1 | 1) => {
    const el = scrollRef.current
    if (!el) return
    // Mandatory scroll-snap re-resolves on the next style change, so an off-snap target visibly jumps back after the scroll ends.
    const snapInset = parseFloat(getComputedStyle(el).scrollPaddingLeft) || 0
    const scrollerLeft = el.getBoundingClientRect().left
    const maxScroll = el.scrollWidth - el.clientWidth
    const snapPositions = Array.from(el.children, child =>
      Math.min(maxScroll, Math.max(0, el.scrollLeft + child.getBoundingClientRect().left - scrollerLeft - snapInset)),
    )
    const desired = el.scrollLeft + dir * 300
    const aheadInDirection = snapPositions.filter(pos => dir * (pos - el.scrollLeft) > 1)
    if (aheadInDirection.length === 0) return
    const target = aheadInDirection.reduce((best, pos) => (Math.abs(pos - desired) < Math.abs(best - desired) ? pos : best))
    // CSS scroll-behavior can be neutralized by a media query, but a behavior passed to scrollTo cannot, so this reads the reduced-motion preference itself.
    el.scrollTo({ left: target, behavior: prefersReducedMotion() ? 'auto' : 'smooth' })
  }

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-2.5 min-h-8">
        <h3
          className={cn('text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground', onViewAll && 'cursor-pointer hover:text-foreground')}
          onClick={onViewAll}
        >
          {title}
        </h3>
        {onViewAll && (
          <button
            onClick={onViewAll}
            aria-label={`See all ${title}`}
            className="text-xs font-medium text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer px-2 py-1 touch:hit-target"
          >
            See all
          </button>
        )}
      </div>
      <div className="relative">
        <button
          className={cn(
            'absolute top-[calc(50%-3px)] -translate-y-1/2 z-10 w-7 h-7 rounded-full bg-card border border-border text-muted-foreground',
            'flex items-center justify-center cursor-pointer',
            'drop-shadow-[0_1px_2px_rgb(0_0_0/0.05)] drop-shadow-[0_4px_12px_rgb(0_0_0/0.1)]',
            'hover:text-foreground hover:drop-shadow-[0_2px_4px_rgb(0_0_0/0.08)] hover:drop-shadow-[0_6px_16px_rgb(0_0_0/0.15)]',
            'transition-[opacity,color] duration-150',
            'touch:hidden -left-3.5',
            fadeLeft ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
          )}
          aria-label={`Scroll ${title} left`}
          disabled={!fadeLeft}
          onClick={() => scroll(-1)}
        >
          <CaretLeft className="h-3.5 w-3.5" weight="bold" />
        </button>
        <div
          ref={scrollRef}
          className={cn(
            'flex overflow-x-auto overscroll-x-none pb-1.5 scroll-smooth [scrollbar-width:none] [&::-webkit-scrollbar]:hidden [scroll-snap-type:x_mandatory] [&>*]:[scroll-snap-align:start]',
            '[--shelf-gap:10px]',
            'gap-[var(--shelf-gap)] -ml-gutter-left -mr-gutter-right pl-gutter-left pr-gutter-right',
            'scroll-pl-[calc(var(--spacing-gutter-left)+var(--spacing-gutter)+var(--shelf-gap))] scroll-pr-gutter-right',
            fadeLeft && 'scroll-fade-left',
            fadeRight && 'scroll-fade-right',
          )}
        >
          {children}
        </div>
        <button
          className={cn(
            'absolute top-[calc(50%-3px)] -translate-y-1/2 z-10 w-7 h-7 rounded-full bg-card border border-border text-muted-foreground',
            'flex items-center justify-center cursor-pointer',
            'drop-shadow-[0_1px_2px_rgb(0_0_0/0.05)] drop-shadow-[0_4px_12px_rgb(0_0_0/0.1)]',
            'hover:text-foreground hover:drop-shadow-[0_2px_4px_rgb(0_0_0/0.08)] hover:drop-shadow-[0_6px_16px_rgb(0_0_0/0.15)]',
            'transition-[opacity,color] duration-150',
            'touch:hidden -right-3.5',
            fadeRight ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
          )}
          aria-label={`Scroll ${title} right`}
          disabled={!fadeRight}
          onClick={() => scroll(1)}
        >
          <CaretRight className="h-3.5 w-3.5" weight="bold" />
        </button>
      </div>
    </div>
  )
}
