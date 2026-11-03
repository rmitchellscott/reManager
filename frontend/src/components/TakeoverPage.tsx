import { X } from '@phosphor-icons/react'
import { useEffect, type ReactNode } from 'react'
import { useBackHandler } from '@/hooks/useBackHandler'
import { useSoftKeyboard } from '@/hooks/useSoftKeyboard'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import { SectionLabel } from '@/components/Panel'
import { cn } from '@/lib/utils'

interface TakeoverPageProps {
  title: string
  narrowTitle: string
  device: string
  deviceMachine?: string
  firmware?: string
  onSettings: () => void
  onDisconnect: () => void
  onClose?: () => void
  showCloseButton?: boolean
  fill?: boolean
  // Hidden rather than unmounted: unmounting kills the terminal's live shell session.
  hidden?: boolean
  children: ReactNode
}

// iOS WKWebView scrolls the body even with overflow:hidden, so the lock pins it with position:fixed and restores the offset on exit.
// Counted, not per-instance: Settings opens over a takeover, and a nested lock must not re-save an already-pinned scrollY.
let scrollLocks = 0
let lockedScrollY = 0

function lockBodyScroll() {
  if (scrollLocks++ > 0) return
  lockedScrollY = window.scrollY
  const style = document.body.style
  style.position = 'fixed'
  style.top = `-${lockedScrollY}px`
  style.left = '0'
  style.right = '0'
  style.width = '100%'
  style.overflow = 'hidden'
}

function unlockBodyScroll() {
  scrollLocks = Math.max(0, scrollLocks - 1)
  if (scrollLocks > 0) return
  const style = document.body.style
  style.position = ''
  style.top = ''
  style.left = ''
  style.right = ''
  style.width = ''
  style.overflow = ''
  window.scrollTo(0, lockedScrollY)
}

// Fixed px, not rem: iOS Dynamic Type scales rem spacing and pushes content off the screen.
export function TakeoverPage({
  title,
  narrowTitle,
  device,
  deviceMachine,
  firmware,
  onSettings,
  onDisconnect,
  onClose,
  showCloseButton = true,
  fill = false,
  hidden = false,
  children,
}: TakeoverPageProps) {
  useEffect(() => {
    if (hidden) return
    lockBodyScroll()
    return unlockBodyScroll
  }, [hidden])

  const closeButton = showCloseButton ? onClose : undefined

  const { ref: overlayRef, swipeHandlers: edgeSwipe } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: () => onClose?.(),
    restoreAfterDismiss: true,
  })
  useBackHandler(() => onClose?.(), !!onClose && !hidden, edgeSwipe)

  // iOS leaves the layout viewport at full height under the keyboard, so the layout is ended above the keys to leave nothing to scroll.
  const { visible: keyboardVisible, inset: keyboardInset } = useSoftKeyboard()

  // A full-height fill surface swallows every touchmove so iOS cannot pan the page; elements marked data-touch-scroll (xterm scrolls its own buffer) are exempt.
  useEffect(() => {
    const overlay = overlayRef.current
    if (!fill || hidden || !overlay) return
    const holdPage = (e: TouchEvent) => {
      if (e.target instanceof Element && e.target.closest('[data-touch-scroll]')) return
      e.preventDefault()
    }
    overlay.addEventListener('touchmove', holdPage, { passive: false })
    return () => overlay.removeEventListener('touchmove', holdPage)
  }, [fill, hidden])

  // iOS also pans the page to reveal the caret on focus (no drag); the body is locked, so any scroll here is that pan and is undone.
  useEffect(() => {
    if (!fill || hidden) return
    const unpan = () => {
      if (window.scrollY !== 0 || (window.visualViewport?.offsetTop ?? 0) !== 0) window.scrollTo(0, 0)
    }
    window.addEventListener('scroll', unpan)
    window.visualViewport?.addEventListener('scroll', unpan)
    return () => {
      window.removeEventListener('scroll', unpan)
      window.visualViewport?.removeEventListener('scroll', unpan)
    }
  }, [fill, hidden])

  return (
    <div
      ref={overlayRef}
      hidden={hidden}
      data-compact={fill && keyboardVisible ? '' : undefined}
      className={cn('fixed inset-0 z-50 overscroll-contain bg-background', fill ? 'overflow-hidden' : 'overflow-auto')}
    >
      <div
        className={cn(
          'p-[24px] pl-[max(24px,var(--safe-area-left))] pr-[max(24px,var(--safe-area-right))] max-md:pt-[calc(var(--safe-area-top)+8px)] touch:pt-[calc(var(--safe-area-top)+8px)]',
          fill
            ? 'flex flex-col compact:pt-[calc(var(--safe-area-top)+8px)] compact:pl-[max(12px,var(--safe-area-left))] compact:pr-[max(12px,var(--safe-area-right))]'
            : 'min-h-dvh pb-[max(24px,var(--safe-area-bottom))]',
          fill && (keyboardVisible
            ? 'compact:pb-[8px]'
            : 'pb-[max(24px,var(--safe-area-bottom))] compact:pb-[calc(8px+var(--safe-area-bottom))]'),
        )}
        style={fill ? { height: `calc(100dvh - ${keyboardInset}px)` } : undefined}
      >
        <div
          className={cn(
            'space-y-[24px]',
            fill && 'flex min-h-0 flex-1 flex-col compact:space-y-[8px]',
          )}
        >
          <div className={cn(fill && 'compact:hidden')}>
            <PageHeader
              device={device}
              deviceMachine={deviceMachine}
              firmware={firmware}
              onSettings={onSettings}
              onDisconnect={onDisconnect}
              narrowTitle={narrowTitle}
            />
          </div>

          <div className={fill ? 'compact:hidden' : 'max-md:hidden'}>
            <hr className="border-border" />
          </div>

          <div
            className={cn(
              'flex items-center justify-between gap-[12px]',
              !closeButton && !fill && 'max-md:hidden',
            )}
          >
            <div className={fill ? 'compact:hidden' : 'max-md:hidden'}>
              <SectionLabel>{title}</SectionLabel>
            </div>
            {fill && <span className="hidden text-sm font-semibold compact:block">{narrowTitle}</span>}
            {closeButton && (
              <Button
                variant="ghost"
                size="xs"
                className="ml-auto -mr-2 h-[32px] w-[32px] shrink-0 p-0 compact:h-[40px] compact:w-[40px] touch:hit-target"
                onClick={onClose}
                aria-label={`Close ${title}`}
              >
                <X className="h-[16px] w-[16px]" />
              </Button>
            )}
          </div>

          {children}
        </div>
      </div>
    </div>
  )
}
