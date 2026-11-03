import { Events } from '@wailsio/runtime'

declare module '@wailsio/runtime' {
  namespace Events {
    interface CustomEvents {
      'common:keyboard': KeyboardFrame | null
      'android:back': Record<string, never>
    }
  }
}

interface KeyboardFrame {
  visible?: boolean
  height?: number
  covered?: number
}

export type EventName = keyof Events.CustomEvents
export type EventData<E extends EventName> = Events.CustomEvents[E]

export function onEvent<E extends EventName>(name: E, callback: (data: EventData<E>) => void): () => void {
  return Events.On(name, event => callback(event.data))
}
