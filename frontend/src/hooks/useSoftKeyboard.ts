import { useEffect, useState } from 'react'
import { onEvent } from '@/lib/events'
import { SystemService } from '@bindings/reManager'

interface SoftKeyboard {
  visible: boolean
  inset: number
}

// With a hardware keyboard, iOS reports a frame for its floating shortcut bar alone (66-71pt vs 340pt with on-screen keys up), so a frame this short counts as no keyboard.
const SHORTCUT_BAR_MAX_HEIGHT = 120

const keyboardResizesView = SystemService.GetPlatformCapabilities()
  .then((capabilities) => capabilities.keyboardResizesView)
  .catch(() => false)

// Android resizes the web view for the keyboard (nothing covered, only visibility used); iOS covers the page and reports the height in points, which are CSS px.
export function useSoftKeyboard(): SoftKeyboard {
  const [keyboard, setKeyboard] = useState<SoftKeyboard>({ visible: false, inset: 0 })

  useEffect(() => {
    return onEvent('common:keyboard', async frame => {
      const keyboardCoversPage = !(await keyboardResizesView)
      const height = frame?.height ?? 0
      const onlyShortcutBar = keyboardCoversPage && height <= SHORTCUT_BAR_MAX_HEIGHT
      const visible = Boolean(frame?.visible) && !onlyShortcutBar
      const inset = visible && keyboardCoversPage ? (frame?.covered ?? height) : 0
      setKeyboard({ visible, inset })
    })
  }, [])

  return keyboard
}
