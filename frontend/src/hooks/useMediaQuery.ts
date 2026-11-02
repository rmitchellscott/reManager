import { useEffect, useState } from 'react'

export const BREAKPOINT = {
  sm: 640,
  md: 768,
  lg: 1024,
} as const

export const NARROW_VIEWPORT = `(max-width: ${BREAKPOINT.md - 1}px)`

export const COMPACT_VIEWPORT = `(max-width: ${BREAKPOINT.sm - 1}px)`

// No desktop browser reports a coarse pointer, so this reads the data-touch attribute the page bootstrap sets in index.html.
export const hasTouchPointer = document.documentElement.hasAttribute('data-touch')

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches)

  useEffect(() => {
    const mql = window.matchMedia(query)
    const sync = () => setMatches(mql.matches)
    sync()
    mql.addEventListener('change', sync)
    return () => mql.removeEventListener('change', sync)
  }, [query])

  return matches
}

export function useTouchLayout(): boolean {
  return useMediaQuery(NARROW_VIEWPORT) && hasTouchPointer
}
