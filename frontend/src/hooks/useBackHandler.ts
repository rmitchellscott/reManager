import { useEffect, useRef, useState } from 'react'
import type { MutableRefObject } from 'react'
import { onEvent } from '@/lib/events'
import { hasTouchPointer } from '@/hooks/useMediaQuery'
import type { SwipeHandlers } from '@/hooks/useSwipeToDismiss'

interface AndroidBackBridge {
  setIntercepting(intercepting: boolean): void
}

declare global {
  interface Window {
    androidBack?: AndroidBackBridge
  }
}

interface BackStep {
  stepBack: () => void
  edgeSwipe: Partial<SwipeHandlers>
}

const openSteps: Array<MutableRefObject<BackStep>> = []

function reportToNative() {
  window.androidBack?.setIntercepting(openSteps.length > 0)
}

onEvent('android:back', () => {
  openSteps[openSteps.length - 1]?.current.stepBack()
})

const EDGE_WIDTH = 24

let swipeInProgress: { handlers: Partial<SwipeHandlers>; x: number; y: number; holding: boolean } | null = null

function startEdgeSwipe(e: PointerEvent) {
  swipeInProgress = null
  if (e.pointerType !== 'touch' || !e.isPrimary || e.clientX > EDGE_WIDTH) return
  const handlers = openSteps[openSteps.length - 1]?.current.edgeSwipe
  if (!handlers?.onPointerDown) return
  swipeInProgress = { handlers, x: e.clientX, y: e.clientY, holding: false }
  handlers.onPointerDown(e)
}

// The edge sits in page padding with no touch-action of its own, so iOS would take the drag as a pan and cancel the pointer unless its first touchmove is held.
function holdEdgeSwipe(e: TouchEvent) {
  if (!swipeInProgress) return
  const touch = e.touches[0]
  const horizontal = Math.abs(touch.clientX - swipeInProgress.x) >= Math.abs(touch.clientY - swipeInProgress.y)
  if (!swipeInProgress.holding && !horizontal) {
    swipeInProgress = null
    return
  }
  swipeInProgress.holding = true
  if (e.cancelable) e.preventDefault()
}

function endEdgeSwipe(e: PointerEvent) {
  const swipe = swipeInProgress
  swipeInProgress = null
  if (e.type === 'pointerup') swipe?.handlers.onPointerUp?.(e)
  else swipe?.handlers.onPointerCancel?.(e)
}

// Android swipes back natively and delivers it as android:back, so only iOS needs the edge gesture.
if (hasTouchPointer && !window.androidBack) {
  window.addEventListener('pointerdown', startEdgeSwipe, { capture: true })
  window.addEventListener('pointermove', e => swipeInProgress?.handlers.onPointerMove?.(e), { capture: true })
  window.addEventListener('pointerup', endEdgeSwipe, { capture: true })
  window.addEventListener('pointercancel', endEdgeSwipe, { capture: true })
  window.addEventListener('touchmove', holdEdgeSwipe, { capture: true, passive: false })
}

// edgeSwipe makes the step a left-edge swipe on iOS; steps without it (overlays, menus) leave the swipe alone, as iOS does.
export function useBackHandler(stepBack: () => void, active: boolean, edgeSwipe: Partial<SwipeHandlers> = {}) {
  const latestStep = useRef<BackStep>({ stepBack, edgeSwipe })
  latestStep.current = { stepBack, edgeSwipe }

  useEffect(() => {
    if (!active) return
    openSteps.push(latestStep)
    reportToNative()
    return () => {
      openSteps.splice(openSteps.lastIndexOf(latestStep), 1)
      reportToNative()
    }
  }, [active])
}

interface OpenState {
  open?: boolean
  defaultOpen?: boolean
  onOpenChange?: (open: boolean) => void
}

export function useBackClosableOpenState({ open, defaultOpen, onOpenChange }: OpenState) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen ?? false)
  const isOpen = open ?? uncontrolledOpen
  const setOpen = (next: boolean) => {
    if (open === undefined) setUncontrolledOpen(next)
    onOpenChange?.(next)
  }
  useBackHandler(() => setOpen(false), isOpen)
  return { open: isOpen, onOpenChange: setOpen }
}
