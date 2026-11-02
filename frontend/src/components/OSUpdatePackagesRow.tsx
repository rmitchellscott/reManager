import { CircleNotch } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { panelAction, PanelRow } from '@/components/Panel'
import type { OsSyncStatus } from '@/tabs/ModsTab'
import type { PanelSeverity } from '@/components/Panel'

interface OSUpdatePackagesRowProps {
  marker: React.ReactNode
  compatiblePackages: string[]
  incompatiblePackages: string[]
  fetchFailed: boolean
  disabled: boolean
  targetOs: string
  sync: OsSyncStatus | null
  settled: boolean
  onReview: () => void
  onApply: () => void
  isNext: boolean
}

const mods = (n: number) => `${n} mod${n === 1 ? '' : 's'}`

interface RowCopy {
  detail: string
  severity: PanelSeverity | null
  action: 'Review' | 'Finish' | 'Show' | null
}

function rowCopy({ compatiblePackages, incompatiblePackages, fetchFailed, targetOs, sync, settled }: OSUpdatePackagesRowProps): RowCopy {
  if (settled) return { detail: `Mods are ready for OS ${targetOs}`, severity: null, action: null }
  if (sync?.state === 'running') return { detail: `Applying changes, ${sync.settled} of ${sync.total}`, severity: null, action: 'Show' }
  if (sync?.state === 'stopped' || sync?.state === 'failed') {
    return { detail: `Stopped after ${sync.applied} of ${sync.requested} changes`, severity: 'warning', action: 'Review' }
  }
  if (fetchFailed) return { detail: 'Could not check compatibility. You may be offline.', severity: 'warning', action: 'Review' }
  const total = compatiblePackages.length + incompatiblePackages.length
  const incompatible = sync?.incompatible ?? incompatiblePackages.length
  if (incompatible > 0) return { detail: `${mods(incompatible)} won't work on OS ${targetOs} and must be removed`, severity: 'warning', action: 'Review' }
  const changing = sync?.changing ?? null
  const removals = sync?.removals ?? 0
  if (removals > 0) {
    return {
      detail: changing ? `${mods(removals)} to remove, ${mods(changing)} need new versions` : `${mods(removals)} to remove`,
      severity: 'warning',
      action: 'Review',
    }
  }
  // Unknown (Vellum has not simulated the sync yet) is treated as a change, so a failed preview never skips the review.
  if (changing === null) return { detail: `Compatible mods (${total})`, severity: null, action: 'Review' }
  if (changing > 0) return { detail: `${mods(changing)} need new versions for OS ${targetOs}`, severity: 'warning', action: 'Review' }
  return { detail: total ? `Your ${mods(total)} already work on OS ${targetOs}` : 'No mods installed', severity: null, action: 'Finish' }
}

export function OSUpdatePackagesRow(props: OSUpdatePackagesRowProps) {
  const { marker, disabled, onReview, onApply, isNext, sync } = props
  const { detail, severity, action } = rowCopy(props)
  const running = sync?.state === 'running'

  return (
    <PanelRow label="Compatibility" detail={detail} severity={severity} marker={marker}>
      {action ? (
        <Button
          className={panelAction}
          variant={isNext && !running ? 'default' : 'outline'}
          onClick={action === 'Finish' ? onApply : onReview}
          disabled={disabled && !running}
        >
          {running && <CircleNotch className="mr-2 h-4 w-4 animate-spin" />}
          {action}
        </Button>
      ) : (
        <Button className={panelAction} variant="outline" aria-hidden tabIndex={-1} style={{ visibility: 'hidden' }}>
          Review
        </Button>
      )}
    </PanelRow>
  )
}
