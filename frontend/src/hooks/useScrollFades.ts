import { useRef, useState, useEffect, useCallback } from 'react'

// Pairs with the .scroll-fade-* classes in index.css.
export function useScrollFades<T extends HTMLElement = HTMLDivElement>() {
  const scrollRef = useRef<T>(null)
  const [fadeLeft, setFadeLeft] = useState(false)
  const [fadeRight, setFadeRight] = useState(false)

  const updateFades = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const style = getComputedStyle(el)
    const padLeft = parseFloat(style.paddingLeft) || 0
    const padRight = parseFloat(style.paddingRight) || 0
    const maxScroll = el.scrollWidth - el.clientWidth
    setFadeLeft(el.scrollLeft > padLeft + 1)
    setFadeRight(el.scrollLeft < maxScroll - padRight - 1)
  }, [])

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    el.addEventListener('scroll', updateFades, { passive: true })

    const resize = new ResizeObserver(updateFades)
    const observeAll = () => {
      resize.disconnect()
      resize.observe(el)
      for (const child of el.children) resize.observe(child)
      updateFades()
    }
    observeAll()

    const mutation = new MutationObserver(observeAll)
    mutation.observe(el, { childList: true })
    return () => {
      el.removeEventListener('scroll', updateFades)
      resize.disconnect()
      mutation.disconnect()
    }
  }, [updateFades])

  return { scrollRef, fadeLeft, fadeRight }
}
