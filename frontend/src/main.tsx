import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

export function applyTheme(theme: string) {
  if (theme === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    document.documentElement.classList.toggle('dark', prefersDark)
  } else {
    document.documentElement.classList.toggle('dark', theme === 'dark')
  }
}

export async function applyThemeWithPortal(theme: string) {
  if (theme === 'system') {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const portalScheme = await (window as any).go?.main?.App?.GetSystemColorScheme()
      if (portalScheme === 'dark') {
        document.documentElement.classList.add('dark')
        return
      } else if (portalScheme === 'light') {
        document.documentElement.classList.remove('dark')
        return
      }
    } catch {
      // Fall through to CSS media query
    }
  }
  applyTheme(theme)
}

async function initTheme() {
  const savedTheme = localStorage.getItem('theme') || 'system'

  if (savedTheme === 'system') {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const portalScheme = await (window as any).go?.main?.App?.GetSystemColorScheme()
      if (portalScheme === 'dark') {
        document.documentElement.classList.add('dark')
        return
      } else if (portalScheme === 'light') {
        document.documentElement.classList.remove('dark')
        return
      }
    } catch {
      // Fall through to CSS media query
    }
  }

  applyTheme(savedTheme)
}

initTheme()

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if ((localStorage.getItem('theme') || 'system') === 'system') {
    applyTheme('system')
  }
})

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)
