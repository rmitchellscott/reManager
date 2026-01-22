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

const savedTheme = localStorage.getItem('theme') || 'system'
applyTheme(savedTheme)

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if ((localStorage.getItem('theme') || 'system') === 'system') {
    applyTheme('system')
  }
})

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)
