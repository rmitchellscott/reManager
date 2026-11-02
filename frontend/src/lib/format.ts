import { compareVersions } from '@/lib/version'

export function formatCount(n: number): string {
  if (!n) return '0'
  return n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n)
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

const BYTE_SCALES: { limit: number; divisor: number; unit: string; digits: number }[] = [
  { limit: 1024, divisor: 1, unit: 'B', digits: 0 },
  { limit: 1024 * 1024, divisor: 1024, unit: 'KB', digits: 1 },
  { limit: 1024 * 1024 * 1024, divisor: 1024 * 1024, unit: 'MB', digits: 1 },
  { limit: Infinity, divisor: 1024 * 1024 * 1024, unit: 'GB', digits: 2 },
]

// Both sides use the total's unit so the readout keeps its width
export function formatBytesPair(done: number, total: number): string {
  const scale = BYTE_SCALES.find(candidate => total < candidate.limit) ?? BYTE_SCALES[BYTE_SCALES.length - 1]
  const left = (done / scale.divisor).toFixed(scale.digits)
  const right = (total / scale.divisor).toFixed(scale.digits)
  return `${left} / ${right} ${scale.unit}`
}

export function formatDuration(seconds: number): string {
  if (!isFinite(seconds) || seconds <= 0) return ''
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ${Math.round(seconds % 60)}s`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

export function formatDate(ts: number): string {
  const date = new Date(ts * 1000)
  const now = new Date()
  const isToday = date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  if (isToday) {
    return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  }
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

export function relativeTime(seconds: number): string {
  if (!seconds) return 'never'
  const mins = Math.floor((Date.now() - seconds * 1000) / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  const weeks = Math.floor(days / 7)
  if (weeks < 5) return `${weeks}w ago`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}mo ago`
  return `${Math.floor(months / 12)}y ago`
}

export function formatDateFull(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: 'numeric', minute: '2-digit',
  })
}

function releaseBefore(exclusiveBound: string): string | null {
  const parts = exclusiveBound.split('.')
  const last = Number(parts[parts.length - 1])
  if (!Number.isInteger(last) || last <= 0) return null
  return [...parts.slice(0, -1), String(last - 1)].join('.')
}

function osRangeText(min: string | null | undefined, exclusiveMax: string | null | undefined): string | null {
  const max = exclusiveMax ? releaseBefore(exclusiveMax) : null
  if (min && exclusiveMax) {
    if (!max) return `≥ ${min}, < ${exclusiveMax}`
    return compareVersions(min, max) === 0 ? min : `${min} – ${max}`
  }
  if (min) return `${min}+`
  if (exclusiveMax) return max ? `≤ ${max}` : `< ${exclusiveMax}`
  return null
}

export function formatOsRange(p: {
  osConstraints?: { version: string; operator: string }[] | null
  osMin?: string | null
  osMax?: string | null
}): string | null {
  if (p.osConstraints && p.osConstraints.length > 0) {
    const minC = p.osConstraints.find(c => c.operator === '>=')
    const maxC = p.osConstraints.find(c => c.operator === '<')
    const exactC = p.osConstraints.find(c => c.operator === '=')

    if (exactC) return exactC.version
    if (minC || maxC) return osRangeText(minC?.version, maxC?.version)
  }

  return osRangeText(p.osMin, p.osMax)
}

export function incompatibleReasonLabel(p: {
  incompatibleReason?: string
  osConstraints?: { version: string; operator: string }[] | null
  osMin?: string | null
  osMax?: string | null
}): string {
  if (p.incompatibleReason === 'device') return 'Other models'
  const range = formatOsRange(p)
  return range ? `Needs OS ${range}` : 'Other OS'
}

const STOCK_LAUNCHER = 'none'

export function launcherSelected(name: string): boolean {
  return name !== '' && name !== STOCK_LAUNCHER
}
