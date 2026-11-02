import { useEffect, useRef, useState } from 'react'

export function useLoaderVisible(
  active: boolean,
  { delayMs = 350, minVisibleMs = 600 }: { delayMs?: number; minVisibleMs?: number } = {},
) {
  const [visible, setVisible] = useState(false)
  const shownAt = useRef<number | null>(null)

  useEffect(() => {
    if (active && !visible) {
      const timer = window.setTimeout(() => {
        shownAt.current = Date.now()
        setVisible(true)
      }, delayMs)
      return () => window.clearTimeout(timer)
    }

    if (!active && visible) {
      const elapsed = shownAt.current === null ? minVisibleMs : Date.now() - shownAt.current
      const timer = window.setTimeout(() => {
        shownAt.current = null
        setVisible(false)
      }, Math.max(0, minVisibleMs - elapsed))
      return () => window.clearTimeout(timer)
    }
  }, [active, visible, delayMs, minVisibleMs])

  return visible
}
