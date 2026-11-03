import { useCallback, useEffect, useMemo, useState } from 'react'
import { FollowInfo, SyncKey } from '@/lib/types'
import { PackagesService } from '@bindings/reManager'
import { onSyncedChange } from '@/lib/syncEvents'

const FOLLOWS_CHANGED = 'follows:changed'

export function useFollowedDevs() {
  const [follows, setFollows] = useState<FollowInfo[]>([])

  useEffect(() => {
    const load = () => PackagesService.GetFollows()
      .then(loaded => setFollows(loaded || []))
      .catch(err => console.error('Failed to load follows:', err))
    load()
    return onSyncedChange(SyncKey.FollowsSyncKey, load)
  }, [])

  useEffect(() => {
    const onChanged = (event: Event) => {
      setFollows((event as CustomEvent<{ follows: FollowInfo[] }>).detail.follows)
    }
    window.addEventListener(FOLLOWS_CHANGED, onChanged)
    return () => window.removeEventListener(FOLLOWS_CHANGED, onChanged)
  }, [])

  const broadcast = useCallback((next: FollowInfo[]) => {
    setFollows(next)
    window.dispatchEvent(new CustomEvent(FOLLOWS_CHANGED, { detail: { follows: next } }))
  }, [])

  const toggleFollow = useCallback(async (dev: string) => {
    try {
      const nowFollowing = await PackagesService.ToggleFollow(dev)
      const withoutDev = follows.filter(f => f.name !== dev)
      broadcast(nowFollowing
        ? [...withoutDev, { name: dev, followedAt: new Date().toISOString(), seen: [] }]
        : withoutDev)
    } catch (err) {
      console.error('Failed to toggle follow:', err)
    }
  }, [follows, broadcast])

  const setPackageSeen = useCallback(async (dev: string, pkg: string, seen: boolean) => {
    const follow = follows.find(f => f.name === dev)
    if (!follow) return
    if ((follow.seen ?? []).includes(pkg) === seen) return
    try {
      await PackagesService.SetPackageSeen(dev, pkg, seen)
      broadcast(follows.map(f => f.name === dev
        ? { ...f, seen: seen ? [...(f.seen ?? []), pkg] : (f.seen ?? []).filter(name => name !== pkg) }
        : f))
    } catch (err) {
      console.error('Failed to update seen state:', err)
    }
  }, [follows, broadcast])

  const followedDevs = useMemo(() => new Set(follows.map(f => f.name)), [follows])

  return { follows, followedDevs, toggleFollow, setPackageSeen }
}
