import type { EventData, EventName } from '@/lib/events'

type Listener = (event: { data: unknown }) => void

const listeners = new Map<string, Set<Listener>>()

export const fakeEvents = {
  On(name: string, callback: Listener): () => void {
    const forName = listeners.get(name) ?? new Set<Listener>()
    forName.add(callback)
    listeners.set(name, forName)
    return () => { forName.delete(callback) }
  },
}

export function emit<E extends EventName>(name: E, data: EventData<E>): void {
  for (const callback of [...(listeners.get(name) ?? [])]) callback({ data })
}

export function listenerCount(): number {
  return [...listeners.values()].reduce((count, forName) => count + forName.size, 0)
}

export function resetEvents(): void {
  listeners.clear()
}
