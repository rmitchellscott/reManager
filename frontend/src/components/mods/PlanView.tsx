import { CaretUp, Check, Circle, CircleNotch, StopCircle, Warning, WarningCircle } from '@phosphor-icons/react'
import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { PackageCard, type CardStatus, type CardContext } from './PackageCard'
import { SectionLabel } from '@/components/Panel'
import { PackageInfo, UpgradeCandidate } from '@/lib/types'
import { installedOnlyPackage } from '@/lib/search'
import { cn } from '@/lib/utils'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { StickyActionBar } from './StickyActionBar'
import { installedVersionOf } from '@/lib/updates'
import { gateCompanion, type Gate, type InstallAnswers, type InstallQuestion } from './InstallQuestionDialog'

export type PlanPhase = 'remove' | 'upgrade' | 'install'

export interface PlanSnapshot {
  remove: string[]
  upgrade: string[]
  install: string[]
  implied: string[]
  impliedRemove: string[]
  // apk upgrades these inside the install phase, so they run with it but show under Upgrade.
  impliedUpgrade: Map<string, UpgradeCandidate>
  sync?: { to: string; targets: Map<string, UpgradeCandidate> | null }
}

// During an OS change Vellum upgrades everything or nothing, so the sync rows are not choices.
export interface OsChangePlan {
  from: string
  to: string
  incompatible: string[]
  sync: string[]
  syncTargets: Map<string, UpgradeCandidate> | null
  targetsAwaitRemovals: boolean
  onStageRemoval: (name: string) => void
  onStageAllRemovals: () => void
  onOpenOSManager: () => void
}

export interface PlanProgress {
  running: Set<string>
  done: Set<string>
  failed: Set<string>
}

export type PlanResult =
  | { outcome: 'succeeded' }
  | { outcome: 'stopped' }
  | { outcome: 'failed'; phase?: PlanPhase; error: string }

const phaseFailedTitle: Record<PlanPhase, string> = {
  remove: 'Removal failed',
  upgrade: 'Upgrade failed',
  install: 'Install failed',
}

const phaseNoun: Record<PlanPhase, string> = {
  remove: 'removal',
  upgrade: 'upgrade',
  install: 'install',
}

export const failedTitle = (phase: PlanPhase, syncing: boolean) =>
  phase === 'upgrade' && syncing ? 'Update failed' : phaseFailedTitle[phase]
const skippedNoun = (phase: PlanPhase, syncing: boolean) =>
  phase === 'upgrade' && syncing ? 'update' : phaseNoun[phase]

export const changes = (n: number) => `${n} change${n === 1 ? '' : 's'}`

export interface PlanQuestions {
  open: InstallQuestion[]
  answers: InstallAnswers
  waiting: Set<string>
  conflictNotes: Map<string, string>
  onOpen: (question: InstallQuestion) => void
  onChangeProvider: (virtual: string) => void
  onChangeGate: (gate: Gate) => void
  onDropCompanion: (name: string) => void
}

const gateTitle: Record<Gate, string> = {
  oxide: 'launcherctl for Oxide',
  'xovi-extensions': 'Starting mods after reboot',
}

function questionText(question: InstallQuestion) {
  if (question.kind === 'provider') {
    const requiredBy = question.choice.requiredBy ?? []
    return {
      title: <>{requiredBy.join(', ')} require{requiredBy.length === 1 ? 's' : ''} <span className="font-mono">{question.choice.virtual}</span></>,
      action: 'Choose',
    }
  }
  return question.gate === 'oxide'
    ? { title: <>Oxide needs launcherctl to start</>, hint: 'Oxide is part of your changes', action: 'Review' }
    : { title: <>Mods need a manual start after reboot</>, hint: 'xovi-extensions is part of your changes', action: 'Review' }
}

interface PlanGroup {
  key: PlanPhase
  label: string
  context: CardContext
  accent: string
  names: string[]
  onCancel: (name: string) => void
}

interface PlanViewProps {
  packages: PackageInfo[]
  installedPackages: Map<string, string>
  installQueue: Set<string>
  upgradeQueue: Set<string>
  uninstallQueue: Set<string>
  impliedPackages: Set<string>
  impliedRemovals: Set<string>
  impliedUpgrades: Map<string, UpgradeCandidate>
  upgradeCandidates: Map<string, UpgradeCandidate> | null
  questions?: PlanQuestions
  // apk can refuse a plan without naming any package pair, leaving Apply blocked with nothing on screen to say why.
  resolutionFailure: string | null
  onSelectPackage: (pkg: PackageInfo) => void
  onRemoveFromInstall: (name: string) => void
  onRemoveFromUpgrade: (name: string) => void
  onRemoveFromUninstall: (name: string) => void
  onClearAll: () => void
  onApply: () => void
  applyLabel: string | null
  applyBusy: boolean
  solving?: boolean
  applyBlocked: boolean
  disabled: boolean
  snapshot: PlanSnapshot | null
  progress: PlanProgress
  result: PlanResult | null
  onFinishApply: () => void
  onCloseApply: () => void
  onRetryApply: () => void
  onStopApply: () => void
  stopRequested: boolean
  output: string[]
  terminalTheme: 'dark' | 'light'
  osChange?: OsChangePlan
}

function GroupLabel({ children, count }: { children: React.ReactNode; count: number }) {
  return (
    <SectionLabel>
      {children}
      <span className="ml-1.5 font-normal tabular-nums">{count}</span>
    </SectionLabel>
  )
}

export function PlanView({
  packages, installedPackages, installQueue, upgradeQueue, uninstallQueue,
  impliedPackages, impliedRemovals, impliedUpgrades, upgradeCandidates, questions, resolutionFailure, onSelectPackage,
  onRemoveFromInstall, onRemoveFromUpgrade, onRemoveFromUninstall,
  onClearAll, onApply, applyLabel, applyBusy, solving = false, applyBlocked, disabled,
  snapshot, progress, result, onFinishApply, onCloseApply, onRetryApply, onStopApply, stopRequested, output, terminalTheme,
  osChange,
}: PlanViewProps) {
  const byName = useMemo(() => new Map(packages.map(p => [p.name, p])), [packages])

  const resolve = (name: string): PackageInfo =>
    byName.get(name) ?? installedOnlyPackage(name, installedPackages.get(name) ?? '')

  const currentVersion = (name: string) =>
    installedVersionOf(name, installedPackages, upgradeCandidates)

  const sorted = (names: Iterable<string>) =>
    [...names].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))

  const impliedUpgrade = snapshot ? snapshot.impliedUpgrade : impliedUpgrades

  const sync = snapshot
    ? snapshot.sync ?? null
    : osChange ? { to: osChange.to, targets: osChange.syncTargets } : null

  const groups: PlanGroup[] = [
    {
      key: 'remove',
      label: 'Remove',
      context: 'installed',
      accent: 'bg-[var(--destructive-surface)]',
      names: snapshot ? snapshot.remove : [...sorted(uninstallQueue), ...sorted(impliedRemovals)],
      onCancel: onRemoveFromUninstall,
    },
    {
      key: 'upgrade',
      label: sync ? `Update for OS ${sync.to}` : 'Upgrade',
      context: 'updates',
      accent: 'bg-[var(--info-surface)]',
      names: snapshot
        ? [...snapshot.upgrade, ...impliedUpgrade.keys()]
        : osChange ? sorted(osChange.sync) : [...sorted(upgradeQueue), ...sorted(impliedUpgrade.keys())],
      onCancel: onRemoveFromUpgrade,
    },
    {
      key: 'install',
      label: 'Install',
      context: 'discover',
      accent: 'bg-[var(--success-surface)]',
      names: snapshot ? snapshot.install : [...sorted(installQueue), ...sorted(impliedPackages)],
      onCancel: (name: string) => questions?.answers.companions.includes(name)
        ? questions.onDropCompanion(name)
        : onRemoveFromInstall(name),
    },
  ]

  const openQuestions = !snapshot ? questions?.open ?? [] : []
  const conflictNotes = !snapshot ? questions?.conflictNotes ?? new Map<string, string>() : new Map<string, string>()
  const pickedVirtual = (name: string) =>
    Object.entries(questions?.answers.providers ?? {}).find(([, provider]) => provider === name)?.[0]
  const isCompanion = (name: string) => !!questions?.answers.companions.includes(name)

  const answered = !snapshot ? (questions?.answers.settledGates ?? []).map(gate => {
    const companion = gateCompanion[gate]
    const withCompanion = isCompanion(companion) || installedPackages.has(companion) || installQueue.has(companion)
    const value = gate === 'oxide'
      ? withCompanion ? 'With launcherctl' : 'Oxide only'
      : withCompanion ? 'With tripletap' : 'Manually'
    return { gate, value }
  }) : []

  const [outputOpen, setOutputOpen] = useState(false)

  const active = groups.filter(g => g.names.length > 0)

  const total = active.reduce((n, g) => n + g.names.length, 0)
  const settled = active.flatMap(g => g.names).filter(n => progress.done.has(n) || progress.failed.has(n)).length

  const isImplied = (name: string) =>
    snapshot
      ? snapshot.implied.includes(name) || snapshot.impliedRemove.includes(name) || snapshot.impliedUpgrade.has(name)
      : impliedPackages.has(name) || impliedRemovals.has(name) || impliedUpgrades.has(name)

  const requested = active.flatMap(g => g.names).filter(n => !isImplied(n))
  const applied = requested.filter(n => progress.done.has(n)).length
  const stillStaged = installQueue.size + upgradeQueue.size + uninstallQueue.size
  const incompatible = !snapshot && osChange ? sorted(osChange.incompatible) : []
  const onlyTheSyncRemains = sync !== null && !snapshot && active.length === 0 && incompatible.length === 0

  const rowState = (name: string) => {
    if (progress.failed.has(name)) return 'failed'
    if (progress.done.has(name)) return 'done'
    if (progress.running.has(name)) return 'running'
    return 'waiting'
  }

  const trailingFor = (name: string) => {
    if (!snapshot) return undefined
    const state = rowState(name)
    if (state === 'failed') return <span className="text-xs text-destructive flex items-center gap-1" role="status"><WarningCircle className="h-4 w-4" />Failed</span>
    if (state === 'waiting' && result) return <span className="text-xs text-muted-foreground whitespace-nowrap" role="status">Not run</span>
    const stateLabel = state === 'done' ? 'Done' : state === 'running' ? 'Running' : 'Waiting'
    const glyph = state === 'done'
      ? <Check className="h-4 w-4 text-muted-foreground" />
      : state === 'running'
        ? <CircleNotch className="h-4 w-4 animate-spin text-muted-foreground" />
        : <Circle className="h-4 w-4 text-muted-foreground/40" />
    return <span className="flex h-9 w-9 sm:h-8 sm:w-8 items-center justify-center" role="status" aria-label={stateLabel}>{glyph}</span>
  }

  const statusFor = (group: PlanGroup, name: string): CardStatus => {
    if (!snapshot && isCompanion(name)) return 'queued'
    if (isImplied(name)) return 'implied'
    if (group.key === 'remove') return 'remove'
    return 'queued'
  }

  const skippedAfter = (phase?: PlanPhase) => {
    if (!phase) return null
    const order: PlanPhase[] = ['remove', 'upgrade', 'install']
    const later = active.some(g => order.indexOf(g.key) > order.indexOf(phase))
      || (phase === 'remove' && sync !== null)
    return later ? ` Nothing after the ${skippedNoun(phase, sync !== null)} ran.` : null
  }

  const appliedOfRequested = requested.length > 0 ? `${applied} of ${changes(requested.length)} applied.` : null

  const resultStatus = !result ? null
    : result.outcome === 'succeeded' ? {
        icon: null,
        title: snapshot?.sync ? `Updated for OS ${snapshot.sync.to}` : `${changes(requested.length)} applied`,
        progressTone: 'bg-[var(--success)]',
      }
    : result.outcome === 'stopped' ? {
        icon: <StopCircle className="h-4 w-4 shrink-0 text-muted-foreground" />,
        title: 'Stopped',
        progressTone: 'bg-muted-foreground',
        detail: appliedOfRequested,
      }
    : {
        icon: null,
        title: result.phase ? failedTitle(result.phase, sync !== null) : 'Apply failed',
        titleTone: 'text-destructive',
        progressTone: 'bg-destructive',
        detail: appliedOfRequested && `${appliedOfRequested}${skippedAfter(result.phase) ?? ''}`,
        error: result.error,
      }

  if (total === 0 && !sync) {
    return (
      <div className="text-center py-12">
        <p className="text-sm font-medium mb-1">Nothing staged</p>
        <p className="text-xs text-muted-foreground">
          Add, upgrade, or remove mods and they collect here before anything runs.
        </p>
      </div>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className={cn('-mx-1 -mt-1 min-h-0 flex-1 space-y-6 overflow-y-auto overscroll-y-contain px-1 pt-1 pb-4', onlyTheSyncRemains && 'flex flex-col justify-center')}>
      {osChange && incompatible.length > 0 && (
        <section>
          <div className="flex items-end justify-between gap-3">
            <GroupLabel count={incompatible.length}>Not compatible with OS {osChange.to}</GroupLabel>
            <Button variant="outline" className="mb-1.5 h-8 min-h-8 py-1 text-xs" onClick={osChange.onStageAllRemovals} disabled={disabled}>
              Remove all
            </Button>
          </div>
          <div className="rounded-[var(--radius)] border border-border bg-[var(--warning-surface)]">
            {incompatible.map(name => (
              <PackageCard
                key={name}
                pkg={resolve(name)}
                status="incompatible"
                context="installed"
                variant="row"
                plan
                onClick={onSelectPackage}
                onAction={osChange.onStageRemoval}
                disabled={disabled}
              />
            ))}
          </div>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">
            Remove these to continue, or use the{' '}
            <button className="underline hover:text-foreground" onClick={osChange.onOpenOSManager}>OS Manager</button>
            {' '}to switch to an OS they support.
          </p>
        </section>
      )}

      {openQuestions.length > 0 && (
        <section>
          <GroupLabel count={openQuestions.length}>Questions</GroupLabel>
          <div className="rounded-[var(--radius)] border border-[var(--warning)] bg-[var(--warning-surface)]">
            {openQuestions.map(question => {
              const { title, hint, action } = questionText(question)
              return (
                <button
                  key={question.kind === 'provider' ? `provider:${question.choice.virtual}` : `gate:${question.gate}`}
                  type="button"
                  onClick={() => questions!.onOpen(question)}
                  disabled={disabled}
                  className="flex w-full items-center gap-4 px-3.5 py-2.5 text-left border-b border-border last:border-b-0 hover:bg-foreground/[0.04] disabled:opacity-50"
                >
                  <span className="flex-1 min-w-0">
                    <span className="block text-sm font-semibold">{title}</span>
                    {hint && <span className="block text-xs text-muted-foreground mt-0.5">{hint}</span>}
                  </span>
                  <span className="text-sm font-semibold text-[var(--warning-text)]">{action}</span>
                </button>
              )
            })}
          </div>
        </section>
      )}

      {resolutionFailure && (
        <div className="space-y-2 rounded-[var(--radius)] border border-destructive/35 bg-[oklch(from_var(--destructive)_l_c_h_/_8%)] px-3.5 py-2.5">
          <div className="flex items-center gap-2.5">
            <WarningCircle className="h-4 w-4 shrink-0 text-destructive" />
            <p className="text-xs leading-[18px]">These changes could not be resolved. Take a mod out and try again.</p>
          </div>
          <OutputDisclosure output={resolutionFailure} theme={terminalTheme} label="Show apk output" />
        </div>
      )}

      {active.map(group => {
        const syncGroup = group.key === 'upgrade' && sync !== null
        return (
          <section key={group.key}>
            <GroupLabel count={group.names.length}>{group.label}</GroupLabel>
            <div className={cn('rounded-[var(--radius)] border border-border', group.accent)}>
              {group.names.map(name => {
                const syncTarget = syncGroup ? sync.targets?.get(name) : undefined
                const pulledUpgrade = group.key === 'upgrade' ? impliedUpgrade.get(name) : undefined
                const virtual = !snapshot && group.key === 'install' ? pickedVirtual(name) : undefined
                const conflictNote = conflictNotes.get(name)
                const waitingNote = !snapshot && group.key === 'install' && questions?.waiting.has(name)
                  ? 'Dependencies show once you answer'
                  : undefined
                const note = conflictNote ?? waitingNote ?? (virtual && <>Provides <span className="font-mono">{virtual}</span></>)
                return (
                  <PackageCard
                    key={name}
                    pkg={resolve(name)}
                    status={statusFor(group, name)}
                    context={syncGroup && !syncTarget ? 'installed' : group.context}
                    variant="row"
                    plan
                    onClick={onSelectPackage}
                    onAction={group.onCancel}
                    hideAction={syncGroup && !snapshot}
                    disabled={disabled}
                    trailing={trailingFor(name) ?? (virtual && (
                      <Button variant="link" className="h-9 px-0 text-sm text-muted-foreground sm:h-8" onClick={() => questions!.onChangeProvider(virtual)} disabled={disabled}>
                        Change
                      </Button>
                    ))}
                    note={note}
                    conflicted={!!conflictNote}
                    muted={!!snapshot && rowState(name) === 'waiting'}
                    applied={!!result && rowState(name) === 'done'}
                    installedVersion={pulledUpgrade?.from ?? (syncTarget?.from || currentVersion(name))}
                    targetVersion={pulledUpgrade?.to ?? (syncGroup ? syncTarget?.to : upgradeCandidates?.get(name)?.to)}
                  />
                )
              })}
            </div>
          </section>
        )
      })}

      {answered.length > 0 && (
        <section>
          <GroupLabel count={answered.length}>Answered</GroupLabel>
          <div className="rounded-[var(--radius)] border border-border">
            {answered.map(({ gate, value }) => (
              <div key={gate} className="flex items-center gap-4 px-3.5 py-2.5 border-b border-border last:border-b-0">
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold">{gateTitle[gate]}</p>
                  <p className="text-xs text-muted-foreground mt-0.5">{value}</p>
                </div>
                <Button variant="link" className="h-9 px-0 text-sm text-muted-foreground sm:h-8" onClick={() => questions!.onChangeGate(gate)} disabled={disabled}>
                  Change
                </Button>
              </div>
            ))}
          </div>
        </section>
      )}

      {onlyTheSyncRemains && (
        <div className="text-center py-12">
          <Check className="h-4 w-4 inline-block text-[var(--success-text)] mb-3" />
          <p className="text-sm font-medium mb-1">All mods are compatible with OS {sync.to}</p>
          <p className="text-xs text-muted-foreground">Press Apply to complete the upgrade.</p>
        </div>
      )}

      </div>

      <StickyActionBar status={snapshot ? (
        <div role={result?.outcome === 'failed' ? 'alert' : result ? 'status' : undefined}>
          <button
            type="button"
            onClick={() => setOutputOpen(o => !o)}
            aria-expanded={outputOpen}
            className="flex w-full items-baseline justify-between gap-2 mb-1.5 text-left touch:hit-target"
          >
            <span className="flex items-center gap-1.5 text-sm font-semibold">
              {resultStatus?.icon && <span className="self-center">{resultStatus.icon}</span>}
              <span className={resultStatus?.titleTone}>
                {resultStatus
                  ? resultStatus.title
                  : snapshot.sync && total === 0 ? `Updating for OS ${snapshot.sync.to}` : `Applying ${changes(total)}`}
              </span>
              <CaretUp className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform', outputOpen && 'rotate-180')} />
            </span>
            {total > 0 && <span className="text-xs text-muted-foreground tabular-nums">{settled} of {total}</span>}
          </button>
          <Progress
            value={total ? Math.round((settled / total) * 100) : result?.outcome === 'succeeded' ? 100 : 0}
            indeterminate={total === 0 && !result}
            className="h-1.5 bg-muted"
            indicatorClassName={cn('transition-[width,background-color] duration-300 ease-out',
                                   resultStatus ? resultStatus.progressTone : progress.failed.size > 0 && 'bg-destructive')}
          />
          {resultStatus?.detail && <p className="mt-1.5 text-xs leading-[18px] text-muted-foreground">{resultStatus.detail}</p>}
          {resultStatus?.error && <p className={cn('line-clamp-4 whitespace-pre-line break-words text-xs leading-[18px]', !resultStatus.detail && 'mt-1.5')}>{resultStatus.error}</p>}
        </div>
      ) : solving && !osChange ? (
        <p role="status" className="flex items-center gap-1.5 text-sm font-semibold">
          <CircleNotch className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
          Checking your changes…
        </p>
      ) : osChange ? (
        <div>
          <p className="flex items-center gap-1.5 text-sm font-semibold">
            <Warning className="h-4 w-4 shrink-0 text-[var(--warning-text)]" />
            OS changed from {osChange.from} to {osChange.to}
          </p>
          {incompatible.length > 0 && (
            <p className="mt-0.5 text-xs leading-[18px] text-[var(--warning-text)]">
              Remove the mods that won&apos;t work on OS {osChange.to} to continue.
            </p>
          )}
        </div>
      ) : conflictNotes.size > 0 ? (
        <p className="text-xs leading-[18px] text-destructive">Remove one of the conflicting mods to apply.</p>
      ) : openQuestions.length > 0 ? (
        <p className="text-xs leading-[18px] text-[var(--warning-text)]">
          Answer {openQuestions.length === 1 ? 'the question' : `the ${openQuestions.length} questions`} above to apply.
        </p>
      ) : undefined}
      panel={snapshot && outputOpen && (
        <div className="h-[min(45vh,360px)] rounded-[var(--radius)] overflow-hidden overscroll-y-contain">
          <TerminalWithCopy output={output.join('\n')} theme={terminalTheme} />
        </div>
      )}>
        {result?.outcome === 'succeeded' ? (
          <Button className="flex-1 min-w-0 sm:flex-none" onClick={onFinishApply}>Done</Button>
        ) : result ? (
          <>
            <Button variant="outline" className="flex-1 min-w-0 sm:flex-none" onClick={onCloseApply}>Close</Button>
            {(osChange || stillStaged > 0) && (
              <Button className="flex-1 min-w-0 sm:flex-none" onClick={onRetryApply} disabled={disabled || applyBlocked}>
                {osChange ? 'Retry' : `Retry ${changes(stillStaged)}`}
              </Button>
            )}
          </>
        ) : applyBusy ? (
          <Button variant="outline" className="flex-1 min-w-0 sm:flex-none text-destructive border-destructive" onClick={onStopApply} disabled={stopRequested}>
            {stopRequested ? 'Stopping…' : 'Stop'}
          </Button>
        ) : (
          <>
            {stillStaged > 0 && (
              <Button variant="outline" className="flex-1 min-w-0 sm:flex-none" onClick={onClearAll}>
                Clear all
              </Button>
            )}
            {applyLabel && (
              <Button className="flex-1 min-w-0 sm:flex-none" onClick={onApply} disabled={disabled || applyBlocked}>
                {applyLabel}
              </Button>
            )}
          </>
        )}
      </StickyActionBar>
    </div>
  )
}
