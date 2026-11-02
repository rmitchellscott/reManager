import { useEffect, useRef, useState } from 'react'
import { BREAKPOINT } from './useMediaQuery'

const DIRECTION_THRESHOLD = 8
const COLLAPSE_AFTER = 80
const EXPAND_FRACTION = 0.5
const MIN_SCROLLABLE_VIEWPORTS = 2

export function useCollapseOnScroll() {
  const [collapsed, setCollapsed] = useState(false)
  const lastScrollY = useRef(0)
  const upwardTravel = useRef(0)

  useEffect(() => {
    const onScroll = () => {
      if (window.innerWidth >= BREAKPOINT.md) {
        setCollapsed(false)
        return
      }

      const maxScroll =
        document.documentElement.scrollHeight - window.innerHeight
      if (
        document.documentElement.scrollHeight <
        window.innerHeight * MIN_SCROLLABLE_VIEWPORTS
      ) {
        setCollapsed(false)
        return
      }

      const current = window.scrollY
      const delta = current - lastScrollY.current
      if (Math.abs(delta) < DIRECTION_THRESHOLD) return
      lastScrollY.current = current

      if (delta > 0) {
        upwardTravel.current = 0
        if (current > COLLAPSE_AFTER) setCollapsed(true)
        return
      }

      upwardTravel.current += -delta
      if (
        current <= COLLAPSE_AFTER ||
        upwardTravel.current >=
          Math.min(window.innerHeight, maxScroll) * EXPAND_FRACTION
      ) {
        setCollapsed(false)
      }
    }

    const resetTravel = () => {
      upwardTravel.current = 0
    }

    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('touchstart', resetTravel, { passive: true })
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('touchstart', resetTravel)
    }
  }, [])

  return {
    collapsed,
    expand: () => {
      upwardTravel.current = 0
      setCollapsed(false)
    },
  }
}
