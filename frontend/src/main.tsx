import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'
import { SystemService } from '@bindings/reManager'
import { onEvent } from '@/lib/events'
import { ColorScheme } from '@/lib/types'

function setDark(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark)
  // Runs before the Wails runtime is ready on first paint, so a rejection here
  // is expected rather than exceptional.
  void SystemService.ApplyNativeTheme(dark ? 'dark' : 'light').catch(() => {})
}

export function applyTheme(theme: string) {
  if (theme === 'system') {
    setDark(window.matchMedia('(prefers-color-scheme: dark)').matches)
  } else {
    setDark(theme === 'dark')
  }
}

export async function applyThemeWithPortal(theme: string) {
  if (theme === 'system') {
    try {
      const portalScheme = await SystemService.GetSystemColorScheme()
      if (portalScheme !== ColorScheme.ColorSchemeUnknown) {
        setDark(portalScheme === ColorScheme.ColorSchemeDark)
        return
      }
    } catch {}
  }
  applyTheme(theme)
}

function followsSystemTheme() {
  return (localStorage.getItem('theme') || 'system') === 'system'
}

void applyThemeWithPortal(localStorage.getItem('theme') || 'system')

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (followsSystemTheme()) {
    applyTheme('system')
  }
})

// WebKitGTK does not fire the media query change when the desktop scheme
// changes, so on Linux the portal signal relayed from Go is the live source.
onEvent('system-theme-changed', scheme => {
  if (followsSystemTheme()) {
    setDark(scheme === ColorScheme.ColorSchemeDark)
  }
})

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)
