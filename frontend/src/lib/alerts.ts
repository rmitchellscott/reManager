import type { StatusRow } from '@/lib/systemStatus'

export type AlertLevel = 'info' | 'pending' | 'warning' | 'destructive'

const LEVEL_RANK: Record<AlertLevel, number> = {
  info: 1,
  pending: 2,
  warning: 3,
  destructive: 4,
}

export function pullsFocus(level: AlertLevel | null): boolean {
  return level === 'warning' || level === 'destructive'
}

export function systemAlertLevel(
  rows: StatusRow[],
  extras: { guideNews: boolean; shellRunning: boolean },
): AlertLevel | null {
  const levels: AlertLevel[] = rows.flatMap(row => (row.severity ? [row.severity] : []))
  if (extras.guideNews || extras.shellRunning) levels.push('info')
  return levels.reduce<AlertLevel | null>(
    (worst, level) => (!worst || LEVEL_RANK[level] > LEVEL_RANK[worst] ? level : worst),
    null,
  )
}
