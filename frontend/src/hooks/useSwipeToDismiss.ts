import { useEffect, useRef } from 'react'
import type { MutableRefObject } from 'react'
import { flushSync } from 'react-dom'

export type SwipeDirection = 'left' | 'right' | 'up' | 'down'

interface SwipeToDismissOptions {
  direction: SwipeDirection
  onDismiss: () => void
  enabled?: boolean
  scrimRef?: MutableRefObject<HTMLElement | null>
  restoreAfterDismiss?: boolean
}

// Structural so the edge-swipe listener in useBackHandler can feed native window events to the same handlers.
type SwipePointer = Pick<PointerEvent, 'pointerId' | 'pointerType' | 'clientX' | 'clientY' | 'timeStamp'>

export interface SwipeHandlers {
  onPointerDown: (e: SwipePointer) => void
  onPointerMove: (e: SwipePointer) => void
  onPointerUp: (e: SwipePointer) => void
  onPointerCancel: (e: SwipePointer) => void
}

const AXIS_LOCK_SLOP = 10
const AXIS_DOMINANCE = 1.2
const DISTANCE_FRACTION = 0.35
const VELOCITY_THRESHOLD = 0.5
const RUBBER_BAND = 0.25
const SNAP_BACK_MS = 200

export function useSwipeToDismiss({
  direction, onDismiss, enabled = true, scrimRef: externalScrimRef, restoreAfterDismiss = false,
}: SwipeToDismissOptions) {
  const ref = useRef<HTMLDivElement | null>(null)
  const ownScrimRef = useRef<HTMLElement | null>(null)
  const scrimRef = externalScrimRef ?? ownScrimRef
  const start = useRef<{ x: number; y: number; t: number; id: number } | null>(null)
  const claimed = useRef(false)
  const travel = useRef(0)
  const settleTimer = useRef(0)

  const horizontal = direction === 'left' || direction === 'right'
  const sign = direction === 'right' || direction === 'down' ? 1 : -1

  const moveTo = (offset: number, animate: boolean) => {
    const el = ref.current
    if (!el) return
    el.style.transition = animate ? `transform ${SNAP_BACK_MS}ms ease-out` : 'none'
    el.style.transform = offset === 0
      ? ''
      : horizontal
        ? `translateX(${offset}px)`
        : `translateY(${offset}px)`
  }

  const setDragging = (dragging: boolean) => {
    const el = ref.current
    if (el) el.style.willChange = dragging ? 'transform' : ''
  }

  const reset = () => {
    start.current = null
    claimed.current = false
    travel.current = 0
    if (settleTimer.current) {
      window.clearTimeout(settleTimer.current)
      settleTimer.current = 0
    }
  }

  useEffect(() => {
    if (!enabled) reset()
  }, [enabled])

  const onPointerDown = (e: SwipePointer) => {
    if (e.pointerType === 'mouse') return
    start.current = { x: e.clientX, y: e.clientY, t: e.timeStamp, id: e.pointerId }
    claimed.current = false
    travel.current = 0
    const scrim = scrimRef.current
    if (scrim) scrim.style.opacity = ''
  }

  const onPointerMove = (e: SwipePointer) => {
    const from = start.current
    if (!from || from.id !== e.pointerId) return

    const dx = e.clientX - from.x
    const dy = e.clientY - from.y

    if (!claimed.current) {
      const along = Math.abs(horizontal ? dx : dy)
      const across = Math.abs(horizontal ? dy : dx)
      if (along < AXIS_LOCK_SLOP && across < AXIS_LOCK_SLOP) return

      if (across > along * AXIS_DOMINANCE) {
        start.current = null
        return
      }
      if (along <= across * AXIS_DOMINANCE) return

      claimed.current = true
      setDragging(true)
      ref.current?.setPointerCapture(e.pointerId)
    }

    const raw = (horizontal ? dx : dy) * sign
    travel.current = raw > 0 ? raw : raw * RUBBER_BAND
    moveTo(travel.current * sign, false)
  }

  const settle = (e: SwipePointer) => {
    const from = start.current
    if (from && from.id !== e.pointerId) return
    start.current = null
    if (!from || !claimed.current) return
    claimed.current = false

    const el = ref.current
    const size = el ? (horizontal ? el.offsetWidth : el.offsetHeight) : 0
    const velocity = travel.current / Math.max(1, e.timeStamp - from.t)

    if (travel.current > size * DISTANCE_FRACTION || velocity > VELOCITY_THRESHOLD) {
      if (el) el.style.animation = 'none'
      const scrim = scrimRef.current
      if (scrim) {
        scrim.style.animation = 'none'
        scrim.style.transition = `opacity ${SNAP_BACK_MS}ms ease-out`
        scrim.style.opacity = '0'
      }
      moveTo(size * sign, true)
      settleTimer.current = window.setTimeout(() => {
        settleTimer.current = 0
        setDragging(false)
        if (!restoreAfterDismiss) {
          onDismiss()
          return
        }
        // Flushed first so a surface that stays mounted moves back only once it shows what the dismiss revealed.
        flushSync(onDismiss)
        moveTo(0, false)
      }, SNAP_BACK_MS)
      return
    }
    moveTo(0, true)
    settleTimer.current = window.setTimeout(() => {
      settleTimer.current = 0
      setDragging(false)
    }, SNAP_BACK_MS)
  }

  const handlers: SwipeHandlers = {
    onPointerDown,
    onPointerMove,
    onPointerUp: settle,
    onPointerCancel: settle,
  }

  return {
    ref,
    scrimRef,
    swipeHandlers: enabled ? handlers : {},
    touchAction: horizontal ? 'touch-pan-y' : 'touch-pan-x',
  }
}
