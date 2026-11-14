import { Check, Package, Warning } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { formatOsRange } from '@/lib/format'
import type { VirtualChoice } from '@/lib/types'

export type Gate = 'oxide' | 'xovi-extensions'

export const gateCompanion: Record<Gate, string> = {
  oxide: 'launcherctl-oxide',
  'xovi-extensions': 'tripletap',
}

export const companionGate: Record<string, Gate> = {
  'launcherctl-oxide': 'oxide',
  tripletap: 'xovi-extensions',
}

export type InstallQuestion =
  | { kind: 'provider'; choice: VirtualChoice }
  | { kind: 'gate'; gate: Gate }

export type InstallAnswer =
  | { kind: 'provider'; virtual: string; provider: string }
  | { kind: 'gate'; gate: Gate; companion: boolean }

export interface InstallAnswers {
  providers: Record<string, string>
  companions: string[]
  settledGates: Gate[]
}

interface InstallQuestionDialogProps {
  question: InstallQuestion | null
  current: string | boolean | null
  installedPackages: Map<string, string>
  installQueue: Set<string>
  firmware: string
  isConnected: boolean
  onFindCompanionConflict: (companion: string) => Promise<string | null>
  onAnswer: (answer: InstallAnswer) => void
  onCancel: () => void
  onPackageInfo: (name: string, osVersion: string, compatible: boolean) => void
}

const cardClass = 'block rounded-md border p-3 cursor-pointer transition-colors hover:bg-accent'
const selectedCardClass = 'border-primary bg-accent'

export function InstallQuestionDialog({
  question,
  current,
  installedPackages,
  installQueue,
  firmware,
  isConnected,
  onFindCompanionConflict,
  onAnswer,
  onCancel,
  onPackageInfo,
}: InstallQuestionDialogProps) {
  const [providerSelection, setProviderSelection] = useState<string | null>(null)
  const [decision, setDecision] = useState<'yes' | 'no' | null>(null)
  const [acknowledged, setAcknowledged] = useState(false)
  const [companionConflict, setCompanionConflict] = useState<string | null | undefined>(undefined)

  useEffect(() => {
    if (question === null) return
    setAcknowledged(false)
    if (question.kind === 'provider') {
      const { choice } = question
      const preselect = typeof current === 'string' ? current : choice.default
      const option = choice.providers?.find(p => p.name === preselect && p.compatible)
      setProviderSelection(option?.name ?? null)
      return
    }
    setDecision(current === true ? 'yes' : current === false ? 'no' : null)
  }, [question, current])

  useEffect(() => {
    if (question === null || question.kind !== 'gate') return
    let stale = false
    setCompanionConflict(undefined)
    onFindCompanionConflict(gateCompanion[question.gate])
      .then(blocks => {
        if (stale) return
        setCompanionConflict(blocks)
        if (blocks) setDecision(prev => (prev === 'yes' ? null : prev))
      })
    return () => { stale = true }
  }, [question])

  if (question === null) return null

  const cancelButton = (
    <Button variant="outline" onClick={onCancel}>
      Cancel
    </Button>
  )

  const providerStep = (choice: VirtualChoice) => {
    const requiredBy = choice.requiredBy ?? []
    return (
      <>
        <DialogHeader severity="neutral" icon={Package}>
          <DialogTitle>
            {requiredBy.join(', ')} require{requiredBy.length === 1 ? 's' : ''}{' '}
            <span className="font-mono">{choice.virtual}</span>
          </DialogTitle>
          <DialogDescription>Choose which one to install.</DialogDescription>
        </DialogHeader>

        <RadioGroup value={providerSelection ?? ''} onValueChange={setProviderSelection} className="gap-2">
          {(choice.providers ?? []).map((provider) => {
            const installed = installedPackages.has(provider.name)
            const osRange = formatOsRange(provider)
            const details = (
              <>
                <div className="flex items-center justify-between gap-2 min-h-[24px]">
                  <span className="text-sm font-semibold">{provider.name}</span>
                  <div className="flex items-center gap-2">
                    {installed && <Check className="h-4 w-4 text-green-600" />}
                    {provider.name === choice.default && <Badge variant="outline">Recommended</Badge>}
                  </div>
                </div>
                {provider.description && (
                  <p className="text-sm text-muted-foreground mt-1">{provider.description}</p>
                )}
                {!provider.compatible && (
                  <p className="text-sm text-destructive mt-1">
                    {provider.incompatibleReason === 'device'
                      ? 'Not for this reMarkable'
                      : (osRange ? `Needs OS ${osRange}` : 'Needs a different OS version')}
                  </p>
                )}
                <button
                  type="button"
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    onPackageInfo(provider.name, firmware, provider.compatible)
                  }}
                  className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground mt-2"
                >
                  More info
                </button>
              </>
            )

            if (!provider.compatible) {
              return (
                <div key={provider.name} className="block rounded-md border border-dashed p-3 opacity-70">
                  {details}
                </div>
              )
            }

            return (
              <label
                key={provider.name}
                htmlFor={`provider-${provider.name}`}
                className={`${cardClass} ${providerSelection === provider.name ? selectedCardClass : ''}`}
              >
                <RadioGroupItem value={provider.name} id={`provider-${provider.name}`} className="sr-only" />
                {details}
              </label>
            )
          })}
        </RadioGroup>

        <DialogFooter>
          {cancelButton}
          <Button
            disabled={providerSelection === null || !isConnected}
            onClick={() => onAnswer({ kind: 'provider', virtual: choice.virtual, provider: providerSelection! })}
          >
            Continue
          </Button>
        </DialogFooter>
      </>
    )
  }

  const companionYesOption = (id: string, title: string, description: string) => {
    if (companionConflict) {
      return (
        <div className="block rounded-md border border-dashed p-3 opacity-70">
          <div className="flex items-center min-h-[24px]">
            <span className="text-sm font-semibold">{title}</span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">{description}</p>
          <p className="text-sm text-destructive mt-1">
            Conflicts with {companionConflict}, which {installedPackages.has(companionConflict) ? 'is installed' : "you're installing"}
          </p>
        </div>
      )
    }
    return (
      <label htmlFor={id} className={`${cardClass} ${decision === 'yes' ? selectedCardClass : ''}`}>
        <RadioGroupItem value="yes" id={id} className="sr-only" />
        <div className="flex items-center justify-between gap-2 min-h-[24px]">
          <span className="text-sm font-semibold">{title}</span>
          <Badge variant="outline">Recommended</Badge>
        </div>
        <p className="text-sm text-muted-foreground mt-1">{description}</p>
      </label>
    )
  }

  const decisionStep = (gate: Gate, prompt: string, yes: { id: string; title: string; description: string }, no: { id: string; title: string; description: string }) => (
    <div className="space-y-3">
      <p className="text-sm font-medium">{prompt}</p>
      <RadioGroup value={decision ?? ''} onValueChange={(v) => setDecision(v as 'yes' | 'no')} className="gap-2">
        {companionYesOption(yes.id, yes.title, yes.description)}
        <label htmlFor={no.id} className={`${cardClass} ${decision === 'no' ? selectedCardClass : ''}`}>
          <RadioGroupItem value="no" id={no.id} className="sr-only" />
          <div className="flex items-center min-h-[24px]">
            <span className="text-sm font-semibold">{no.title}</span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">{no.description}</p>
        </label>
      </RadioGroup>

      <DialogFooter>
        {cancelButton}
        <Button
          disabled={decision === null || companionConflict === undefined || !isConnected}
          onClick={() => onAnswer({ kind: 'gate', gate, companion: decision === 'yes' })}
        >
          Continue
        </Button>
      </DialogFooter>
    </div>
  )

  const acknowledgeStep = (gate: Gate, heading: string, name: string, installed: boolean, description: string, instruction: string, promise: string) => (
    <div className="space-y-3">
      <p className="text-sm font-medium">{heading}</p>
      <div className="rounded-md border bg-accent p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-semibold">{name}</span>
          <Badge variant="outline" className="gap-1">
            <Check className="h-3 w-3" />
            {installed ? 'Installed' : 'Added'}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground mt-1">{description}</p>
      </div>
      <p className="border-t pt-2.5 text-sm text-muted-foreground">{instruction}</p>

      <label className="flex items-start gap-3 rounded-md border bg-muted p-3 cursor-pointer">
        <Checkbox checked={acknowledged} onCheckedChange={(v) => setAcknowledged(v === true)} className="mt-0.5" />
        <span className="text-sm">{promise}</span>
      </label>

      <DialogFooter>
        {cancelButton}
        <Button disabled={!acknowledged || !isConnected} onClick={() => onAnswer({ kind: 'gate', gate, companion: false })}>
          Continue
        </Button>
      </DialogFooter>
    </div>
  )

  const oxideStep = () => {
    const installed = installedPackages.has('launcherctl-oxide')
    return (
      <>
        <DialogHeader severity="warning" icon={Warning}>
          <DialogTitle>Oxide needs launcherctl to start</DialogTitle>
          <DialogDescription>
            Oxide is a full launcher. It takes over the screen instead of running inside the reMarkable UI.
          </DialogDescription>
        </DialogHeader>
        {installed || installQueue.has('launcherctl-oxide')
          ? acknowledgeStep(
            'oxide', 'How to change launchers', 'launcherctl', installed,
            'Switch between Oxide, other launchers, and the reMarkable UI.',
            'In reManager, go to System › Advanced › launcherctl › Switch.',
            "I'll manage my launcher from the System tab.",
          )
          : decisionStep(
            'oxide', 'Would you like to install launcherctl?',
            { id: 'lc-yes', title: 'Yes, install launcherctl', description: 'Switch between Oxide, other launchers, and the reMarkable UI from the System tab.' },
            { id: 'lc-no', title: 'No, just Oxide', description: "Oxide will be installed but won't run." },
          )}
      </>
    )
  }

  const xoviStep = () => {
    const installed = installedPackages.has('tripletap')
    return (
      <>
        <DialogHeader severity="warning" icon={Warning}>
          <DialogTitle>Mods require a manual start after every reboot</DialogTitle>
          <DialogDescription>
            UI mods don't auto-start after reboot. Forcing auto-start can prevent your reMarkable from booting.
          </DialogDescription>
        </DialogHeader>
        {installed || installQueue.has('tripletap')
          ? acknowledgeStep(
            'xovi-extensions', 'How to start mods after reboot', 'tripletap', installed,
            'Triple-press the power button after every boot to start mods.',
            'You can also start them from the System tab in reManager.',
            "I'll start my mods manually after every reboot.",
          )
          : decisionStep(
            'xovi-extensions', 'Would you like to install tripletap?',
            { id: 'tt-yes', title: 'Yes, install tripletap', description: 'Triple-press the power button after every boot to start mods.' },
            { id: 'tt-no', title: "No, I'll start mods from reManager", description: 'Open reManager and start them from the System tab after every boot, or start them over SSH.' },
          )}
      </>
    )
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onCancel() }}>
      <DialogContent>
        {question.kind === 'provider' ? providerStep(question.choice)
          : question.gate === 'oxide' ? oxideStep()
            : xoviStep()}
      </DialogContent>
    </Dialog>
  )
}
