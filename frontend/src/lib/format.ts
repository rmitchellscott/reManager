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

export function formatDateFull(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: 'numeric', minute: '2-digit',
  })
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

    if (minC && maxC) {
      const maxInclusive = (parseFloat(maxC.version) - 0.01).toFixed(2)
      return minC.version === maxInclusive ? minC.version : `${minC.version} – ${maxInclusive}`
    }

    if (minC) return `${minC.version}+`
    if (maxC) {
      const maxInclusive = (parseFloat(maxC.version) - 0.01).toFixed(2)
      return `≤ ${maxInclusive}`
    }
  }

  if (!p.osMin && !p.osMax) return null
  if (p.osMin && p.osMax) {
    const maxInclusive = (parseFloat(p.osMax) - 0.01).toFixed(2)
    return p.osMin === maxInclusive ? p.osMin : `${p.osMin} – ${maxInclusive}`
  }
  if (p.osMin) return `${p.osMin}+`
  if (p.osMax) {
    const maxInclusive = (parseFloat(p.osMax) - 0.01).toFixed(2)
    return `≤ ${maxInclusive}`
  }
  return null
}

export const STOCK_LAUNCHER = 'none'

export function launcherSelected(name: string): boolean {
  return name !== '' && name !== STOCK_LAUNCHER
}
