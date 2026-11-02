import { onEvent } from '@/lib/events'
import type { SyncKey } from '@/lib/types'

export function onSyncedChange(syncKey: SyncKey, reload: () => void): () => void {
  return onEvent('sync:changed', changed => {
    if (changed === syncKey) reload()
  })
}
