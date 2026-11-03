import { useCallback, useEffect, useState } from 'react'
import { PackagesService } from '@bindings/reManager'
import { onSyncedChange } from '@/lib/syncEvents'
import { SyncKey } from '@/lib/types'

const FAVORITES_CHANGED = 'favorites:changed'

export function usePackageFavorites() {
  const [starred, setStarred] = useState<Set<string>>(new Set())

  useEffect(() => {
    const load = () => PackagesService.GetFavorites()
      .then(names => setStarred(new Set(names || [])))
      .catch(err => console.error('Failed to load favorites:', err))
    load()
    return onSyncedChange(SyncKey.FavoritesSyncKey, load)
  }, [])

  useEffect(() => {
    const onChanged = (event: Event) => {
      const { name, starred: nowStarred } = (event as CustomEvent<{ name: string; starred: boolean }>).detail
      setStarred(prev => {
        if (prev.has(name) === nowStarred) return prev
        const next = new Set(prev)
        nowStarred ? next.add(name) : next.delete(name)
        return next
      })
    }
    window.addEventListener(FAVORITES_CHANGED, onChanged)
    return () => window.removeEventListener(FAVORITES_CHANGED, onChanged)
  }, [])

  const toggleStar = useCallback(async (name: string) => {
    try {
      const nowStarred = await PackagesService.ToggleFavorite(name)
      window.dispatchEvent(new CustomEvent(FAVORITES_CHANGED, { detail: { name, starred: nowStarred } }))
    } catch (err) {
      console.error('Failed to toggle favorite:', err)
    }
  }, [])

  return { starred, toggleStar }
}
