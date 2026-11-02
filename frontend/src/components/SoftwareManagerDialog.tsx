import { ArrowRight, Check, CircleNotch, DownloadSimple, HardDrives, Power, Swap, WarningCircle, X } from '@phosphor-icons/react'
import { useState, useEffect, useCallback } from 'react'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Dialog,
  DialogCloseButton,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { toast } from 'sonner'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { useAppContext } from '@/contexts/AppContext'
import { CheckOSDialog } from '@/components/CheckOSDialog'
import { CompatibilityResult, ErrorCode, OSInstallStage, type OSInstallProgress, type OSVersionInfo, type PartitionInfo, type PartitionSlot } from '@/lib/types'
import { OSService, OperationsService, PackagesService } from '@bindings/reManager'
import { operationFailure } from '@/lib/operations'

interface SoftwareManagerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  vellumInstalled: boolean | null
  onSelectPackageForOS: (name: string, targetOS: string, isCompatible: boolean) => void
}

export function SoftwareManagerDialog({ open, onOpenChange, isConnected, vellumInstalled, onSelectPackageForOS }: SoftwareManagerDialogProps) {
  const { resolvedTerminalTheme, commandRunning, runningOperation, startOperation } = useAppContext()
  const [systemInfo, setSystemInfo] = useState<PartitionInfo | null>(null)
  const [partitionError, setPartitionError] = useState<{ message: string; detail: string } | null>(null)
  const [lockDirError, setLockDirError] = useState<string | null>(null)
  const [versions, setVersions] = useState<OSVersionInfo[]>([])
  const [versionsFailed, setVersionsFailed] = useState(false)
  const [loading, setLoading] = useState(true)
  const [versionsLoading, setVersionsLoading] = useState(false)
  const [selectedVersion, setSelectedVersion] = useState('')
  const [installNeedsReboot, setInstallNeedsReboot] = useState(false)
  const [installMode, setInstallMode] = useState<'swupdate' | 'legacy'>('swupdate')

  const [showSwitchConfirm, setShowSwitchConfirm] = useState(false)
  const [showInstallConfirm, setShowInstallConfirm] = useState(false)
  const [showInstallProgress, setShowInstallProgress] = useState(false)
  const [showInstallComplete, setShowInstallComplete] = useState(false)
  const [showRebootConfirm, setShowRebootConfirm] = useState(false)
  const switching = runningOperation?.kind === 'switch-partition'
  const rebooting = runningOperation?.kind === 'reboot'
  const [switchError, setSwitchError] = useState<{ message: string; detail: string } | null>(null)
  const [rebootError, setRebootError] = useState<{ message: string; detail: string } | null>(null)

  const [installRun, setInstallRun] = useState<{ id: string; progress: OSInstallProgress } | null>(null)
  const [installedVersion, setInstalledVersion] = useState('')
  const [showInstallError, setShowInstallError] = useState(false)
  const [installErrorMessage, setInstallErrorMessage] = useState('')
  const [installErrorOutput, setInstallErrorOutput] = useState('')
  const [installErrorOutputOpen, setInstallErrorOutputOpen] = useState(false)

  const [compatCache, setCompatCache] = useState<Record<string, CompatibilityResult>>({})
  const [partitionCompatLoading, setPartitionCompatLoading] = useState(false)
  const [versionCompatLoading, setVersionCompatLoading] = useState(false)
  const [checkOSVersion, setCheckOSVersion] = useState<string | null>(null)
  const [showIncompatWarning, setShowIncompatWarning] = useState(false)

  const checkCompatibility = useCallback(async (version: string): Promise<CompatibilityResult | null> => {
    try {
      const res = await PackagesService.CheckOSCompatibility(version)
      if (!res.fetchFailed) {
        setCompatCache(prev => ({ ...prev, [version]: res }))
        return res
      }
    } catch {}
    return null
  }, [])

  const loadData = useCallback(async () => {
    if (!isConnected) return
    setLoading(true)
    setVersionsLoading(true)
    setVersionsFailed(false)
    setPartitionError(null)
    setLockDirError(null)

    try {
      const readiness = await OSService.CheckOSManagerReady()
      if (!readiness.ready && readiness.code === ErrorCode.ErrLockDirMissing) {
        setLockDirError(readiness.message || 'Your reMarkable is not ready for OS updates right now.')
        setLoading(false)
        setVersionsLoading(false)
        return
      }
    } catch {}

    let info: PartitionInfo
    try {
      const partition = await OSService.GetPartitionInfo()
      if (!partition) throw new Error('no partition info returned')
      info = partition
      setSystemInfo(info)
    } catch (err) {
      setPartitionError({ message: handleError(err, ''), detail: errorDetail(err) })
      setVersionsLoading(false)
      toast.error(handleError(err, 'Could not read partition info.'))
      return
    } finally {
      setLoading(false)
    }

    const rebootPending = !info.active.isNextBoot || info.updatePending
    if (rebootPending) {
      setVersions([])
      setVersionsLoading(false)
      return
    }

    try {
      const mode = await OSService.GetOSInstallMode()
      setInstallMode(mode === 'legacy' ? 'legacy' : 'swupdate')
    } catch {
      setInstallMode('swupdate')
    }

    try {
      const availableVersions = await OSService.GetAvailableOSVersions()
      const installed = new Set([info.active.version, info.fallback.version])
      setVersions((availableVersions || []).map(v => ({
        ...v,
        isInstalled: installed.has(v.version),
      })))
    } catch {
      setVersionsFailed(true)
    } finally {
      setVersionsLoading(false)
    }
  }, [isConnected])

  const installRunning = installRun !== null
  const installProgress = installRun?.progress ?? { stage: OSInstallStage.OSInstallDownloading, percent: 0, message: 'Downloading…' }

  useEffect(() => {
    if (open && isConnected) {
      loadData()
      if (installRunning) {
        setShowInstallProgress(true)
        return
      }
      setInstallNeedsReboot(false)
      setSelectedVersion('')
      setCompatCache({})
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isConnected, loadData])

  useEffect(() => {
    if (!open || !systemInfo || !vellumInstalled) return

    const versions = [systemInfo.active.version, systemInfo.fallback.version]
    const unchecked = versions.filter(v => v !== 'unknown' && !compatCache[v])
    if (unchecked.length === 0) return

    setPartitionCompatLoading(true)
    Promise.all(unchecked.map(v => checkCompatibility(v))).finally(() => {
      setPartitionCompatLoading(false)
    })
  }, [open, systemInfo, vellumInstalled, checkCompatibility])

  useEffect(() => {
    if (!selectedVersion || !vellumInstalled || compatCache[selectedVersion]) return

    setVersionCompatLoading(true)
    checkCompatibility(selectedVersion).finally(() => {
      setVersionCompatLoading(false)
    })
  }, [selectedVersion, vellumInstalled, checkCompatibility, compatCache])

  const slotA = systemInfo ? (systemInfo.active.number === 2 ? systemInfo.active : systemInfo.fallback) : null
  const slotB = systemInfo ? (systemInfo.active.number === 3 ? systemInfo.active : systemInfo.fallback) : null

  const standbySlot = systemInfo ? systemInfo.fallback : null
  const bootSwitchPending = systemInfo ? !systemInfo.active.isNextBoot : false
  const needsReboot = bootSwitchPending || !!systemInfo?.updatePending || installNeedsReboot

  const isLegacy = installMode === 'legacy'

  const selectedVersionCompat = selectedVersion ? compatCache[selectedVersion] : null
  const selectedVersionHasIncompat = selectedVersionCompat ? (selectedVersionCompat.incompatible ?? []).length > 0 : false

  const handleInstallClick = () => {
    if (selectedVersionHasIncompat) {
      setShowIncompatWarning(true)
    } else {
      setShowInstallConfirm(true)
    }
  }

  const handleSwitchConfirm = async () => {
    if (!standbySlot) return
    const nextBootTarget = systemInfo!.active.isNextBoot
      ? systemInfo!.fallback.number
      : systemInfo!.active.number
    setSwitchError(null)
    const result = await startOperation({
      kind: 'switch-partition',
      title: 'Switch partition',
      background: true,
      reportsOwnFailure: true,
      run: id => OSService.SwitchBootPartition(id, nextBootTarget),
    })
    if (!result.ok) {
      const failure = operationFailure(result)
      setSwitchError({ message: handleError(failure, 'Could not switch partition.'), detail: errorDetail(failure) })
      return
    }
    setShowSwitchConfirm(false)
    await loadData()
  }

  const handleInstallConfirm = async () => {
    if (!selectedVersion) return
    const version = selectedVersion
    setShowInstallConfirm(false)
    setShowIncompatWarning(false)
    setShowInstallProgress(true)
    let output = ''
    const result = await startOperation({
      kind: 'os-install',
      target: version,
      title: `Install ${version}`,
      background: true,
      reportsOwnFailure: true,
      onOSInstallProgress: progress => setInstallRun(prev => prev && { ...prev, progress }),
      onOutput: text => { output += text },
      run: id => {
        setInstallRun({ id, progress: { stage: OSInstallStage.OSInstallDownloading, percent: 0, message: 'Downloading…' } })
        return OSService.InstallOSVersion(id, version)
      },
    })
    setInstallRun(null)
    setShowInstallProgress(false)
    if (result.ok) {
      setInstalledVersion(version)
      setRebootError(null)
      setShowInstallComplete(true)
      setInstallNeedsReboot(true)
      loadData()
      return
    }
    if (result.canceled) return
    setInstallErrorMessage(result.error ?? '')
    setInstallErrorOutput(output.trim())
    setShowInstallError(true)
  }

  const handleReboot = async () => {
    setRebootError(null)
    const result = await startOperation({
      kind: 'reboot',
      title: 'Reboot',
      background: true,
      reportsOwnFailure: true,
      run: id => OSService.RebootDevice(id),
    })
    if (!result.ok) {
      const failure = operationFailure(result)
      setRebootError({ message: handleError(failure, 'Could not reboot.'), detail: errorDetail(failure) })
      return
    }
    setShowRebootConfirm(false)
    setShowInstallComplete(false)
    onOpenChange(false)
  }

  const getCurrentBootSlot = () => {
    if (!systemInfo) return null
    return systemInfo.active.isNextBoot ? systemInfo.active : systemInfo.fallback
  }

  const getInstallDescription = () => {
    if (!selectedVersion || !systemInfo) return ''
    const standby = systemInfo.fallback
    const active = systemInfo.active

    if (selectedVersion === standby.version) {
      return `Version ${selectedVersion} is already installed on Partition ${standby.label}. Installing again will re-download and overwrite it.`
    }
    if (selectedVersion === active.version) {
      return `Version ${selectedVersion} is already running on Partition ${active.label}. This will install a copy to Partition ${standby.label}.`
    }
    return `This will install ${selectedVersion} to Partition ${standby.label}. Reboot to switch.`
  }

  const getVersionLabel = (version: string) => {
    if (!systemInfo) return ''
    const labels: string[] = []
    if (versions.length > 0 && versions[0].version === version) labels.push('Latest')
    if (version === systemInfo.active.version || version === systemInfo.fallback.version) labels.push('Installed')
    return labels.join(' · ')
  }

  const renderCompatIcon = (version: string) => {
    if (version === 'unknown') {
      return <span className="text-muted-foreground">Unknown</span>
    }
    const result = compatCache[version]
    if (!result) {
      if (partitionCompatLoading) {
        return <CircleNotch className="h-4 w-4 animate-spin text-muted-foreground" />
      }
      return null
    }
    const hasIncompat = (result.incompatible ?? []).length > 0
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button onClick={(e) => { e.stopPropagation(); setCheckOSVersion(version) }}>
            {hasIncompat ? (
              <X className="h-4 w-4 text-destructive" />
            ) : (
              <Check className="h-4 w-4 text-green-500" />
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent>View compatibility details</TooltipContent>
      </Tooltip>
    )
  }

  const renderSlotCard = (slot: PartitionSlot | null, label: string, clickable: boolean) => {
    if (!slot) return null
    const isRunning = slot.isActive
    const isNextBoot = slot.isNextBoot && !slot.isActive

    let badgeLabel = 'Standby'
    let badgeVariant: 'default' | 'secondary' = 'secondary'
    let highlighted = false

    if (isRunning && slot.isNextBoot) {
      badgeLabel = 'Running'
      badgeVariant = 'default'
      highlighted = true
    } else if (isRunning) {
      badgeLabel = 'Running'
    } else if (isNextBoot) {
      badgeLabel = 'Next boot'
      badgeVariant = 'default'
      highlighted = true
    }

    return (
      <div
        className={`border rounded-[10px] p-4 transition-colors ${highlighted ? 'border-foreground' : ''} ${clickable ? 'cursor-pointer hover:border-foreground' : ''}`}
        onClick={clickable ? () => setShowSwitchConfirm(true) : undefined}
      >
        <div className="flex items-center justify-between gap-2 mb-1.5">
          <span className="text-sm font-semibold truncate">Partition {label}</span>
          <Badge variant={badgeVariant} className="text-xs px-2 py-0 shrink-0">
            {badgeLabel}
          </Badge>
        </div>
        <div className="flex items-center justify-between">
          <div>
            <div className="text-xs text-muted-foreground">Version</div>
            <div className="text-sm font-medium">{slot.version}</div>
          </div>
          {vellumInstalled && (
            <div className="flex flex-col items-end gap-0.5">
              <span className="text-xs text-muted-foreground">Compatibility</span>
              {renderCompatIcon(slot.version)}
            </div>
          )}
        </div>
      </div>
    )
  }

  const renderVersionCompatStatus = () => {
    if (!selectedVersion || !vellumInstalled) return null

    if (versionCompatLoading || !compatCache[selectedVersion]) {
      return (
        <div className="flex items-center gap-2 text-xs text-muted-foreground h-5">
          <CircleNotch className="h-3.5 w-3.5 animate-spin" />
          Checking…
        </div>
      )
    }

    const result = compatCache[selectedVersion]
    const hasIncompat = (result.incompatible ?? []).length > 0
    return (
      <div className="flex items-center gap-2 text-xs h-5">
        {hasIncompat ? (
          <X className="h-3.5 w-3.5 text-destructive flex-shrink-0" />
        ) : (
          <Check className="h-3.5 w-3.5 text-green-500 flex-shrink-0" />
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className="text-primary hover:underline"
              onClick={() => setCheckOSVersion(selectedVersion)}
            >
              {hasIncompat ? 'Compatibility issues found' : 'Installed mods are compatible'}
            </button>
          </TooltipTrigger>
          <TooltipContent>View compatibility details</TooltipContent>
        </Tooltip>
      </div>
    )
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="xl" className="overflow-y-auto">
          <DialogCloseButton onClick={() => onOpenChange(false)} />
          <DialogHeader className="mb-3">
            <DialogTitle>OS Manager</DialogTitle>
            <DialogDescription className="text-xs">
              View, install, and switch OS versions.
            </DialogDescription>
          </DialogHeader>

          {loading ? (
            <div className="flex items-center justify-center py-12">
              <CircleNotch className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : lockDirError ? (
            <>
              <div className="flex flex-col items-center text-center py-6">
                <WarningCircle className="h-6 w-6 text-destructive" />
                <div className="mt-3 text-sm font-medium">Cannot install or switch OS versions right now</div>
                <div className="mt-1.5 text-xs text-muted-foreground break-words max-w-md">{lockDirError}</div>
              </div>
              <div className="border-t pt-4 mt-2">
                <Button className="w-full" onClick={() => setShowRebootConfirm(true)}>
                  Reboot
                </Button>
              </div>
            </>
          ) : partitionError ? (
            <div className="flex flex-col items-center gap-3 py-8 text-center">
              <WarningCircle className="h-6 w-6 text-destructive" />
              <div>
                <div className="text-sm font-medium">Could not read partition info</div>
                {partitionError.message && (
                  <div className="mt-1 text-xs text-muted-foreground break-words max-w-md">{partitionError.message}</div>
                )}
              </div>
              <OutputDisclosure output={partitionError.detail} className="w-full max-w-md text-left" />
              <Button variant="outline" size="sm" onClick={() => loadData()}>Try again</Button>
            </div>
          ) : systemInfo ? (
            <>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {renderSlotCard(slotA, 'A', !!(slotA && !slotA.isNextBoot && !installNeedsReboot && !systemInfo?.updatePending))}
                {renderSlotCard(slotB, 'B', !!(slotB && !slotB.isNextBoot && !installNeedsReboot && !systemInfo?.updatePending))}
              </div>

              <div className="h-px bg-border my-4" />

              <div className="flex flex-col gap-4">

                <div className="flex flex-col gap-2.5">
                  <div>
                    <div className="text-sm font-medium">Install a version</div>
                    <div className="text-xs text-muted-foreground">Download and install to the standby partition.</div>
                  </div>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <Select value={selectedVersion} onValueChange={setSelectedVersion} disabled={needsReboot || versionsFailed || versionsLoading || installRunning}>
                      <SelectTrigger className="w-full sm:flex-1 h-9 touch:hit-target">
                        <div className="flex min-w-0 items-center gap-2">
                          {versionsLoading && <CircleNotch className="h-3.5 w-3.5 animate-spin shrink-0 text-muted-foreground" />}
                          <SelectValue className="truncate" placeholder={needsReboot ? "Reboot to install a version" : versionsLoading ? "Loading versions…" : versionsFailed ? "Could not load versions" : "Choose a version…"}>{selectedVersion || undefined}</SelectValue>
                        </div>
                      </SelectTrigger>
                      <SelectContent>
                        {versions.map((v) => (
                          <SelectItem key={v.version} value={v.version} className="pl-3 [&>span:first-child]:hidden [&>span:last-child]:flex-1">
                            <span className="flex items-center w-full">
                              <span>{v.version}</span>
                              {getVersionLabel(v.version) && (
                                <span className="text-xs text-muted-foreground ml-auto pl-3">{getVersionLabel(v.version)}</span>
                              )}
                            </span>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Button
                      size="sm"
                      className="h-9 w-full sm:w-auto touch:hit-target"
                      disabled={!selectedVersion || needsReboot || versionsFailed || versionsLoading || installRunning}
                      onClick={handleInstallClick}
                    >
                      Install
                    </Button>
                  </div>
                  {isLegacy && (
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <WarningCircle className="h-3.5 w-3.5 shrink-0" />
                      <span>Newer versions need 3.11.2.5 first. Install it, then reboot.</span>
                    </div>
                  )}
                  {versionsFailed && (
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <WarningCircle className="h-3.5 w-3.5" />
                      Check your internet connection and reopen.
                    </div>
                  )}
                  {vellumInstalled && <div className="h-5">{renderVersionCompatStatus()}</div>}
                </div>
              </div>

              {needsReboot && (
                <div className="border-t pt-4 mt-2">
                  <Button className="w-full" onClick={() => setShowRebootConfirm(true)}>
                    Reboot
                  </Button>
                </div>
              )}
            </>
          ) : (
            <div className="text-center text-muted-foreground py-8">
              <p>Connect a reMarkable to see its partitions.</p>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={showSwitchConfirm} onOpenChange={setShowSwitchConfirm}>
        <DialogContent>
          <DialogHeader severity="info" icon={Swap}>
            <DialogTitle>Switch boot partition?</DialogTitle>
            <DialogDescription>
              On next restart, your reMarkable will boot from a different partition.
            </DialogDescription>
          </DialogHeader>
          {systemInfo && (() => {
            const aIsCurrent = getCurrentBootSlot()!.label === 'A'
            return (
              <div className="flex gap-3 items-center">
                <div className={`flex-1 p-2.5 rounded-lg border ${aIsCurrent ? 'bg-muted' : 'border-foreground'}`}>
                  <div className="text-xs text-muted-foreground mb-0.5">{aIsCurrent ? 'Current' : 'Next boot'}</div>
                  <div className="text-sm font-semibold">{slotA?.version}</div>
                  <div className="text-xs text-muted-foreground">Partition A</div>
                </div>
                <ArrowRight className={`h-4 w-4 text-muted-foreground shrink-0 ${!aIsCurrent ? 'rotate-180' : ''}`} />
                <div className={`flex-1 p-2.5 rounded-lg border ${aIsCurrent ? 'border-foreground' : 'bg-muted'}`}>
                  <div className="text-xs text-muted-foreground mb-0.5">{aIsCurrent ? 'Next boot' : 'Current'}</div>
                  <div className="text-sm font-semibold">{slotB?.version}</div>
                  <div className="text-xs text-muted-foreground">Partition B</div>
                </div>
              </div>
            )
          })()}
          {switchError && (
            <Alert variant="destructive">
              <WarningCircle className="h-4 w-4" />
              <AlertDescription>{switchError.message}</AlertDescription>
            </Alert>
          )}
          {switchError && <OutputDisclosure output={switchError.detail} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSwitchConfirm(false)}>Cancel</Button>
            <Button onClick={handleSwitchConfirm} disabled={commandRunning || !isConnected}>
              {switching && <CircleNotch className="h-4 w-4 mr-2 animate-spin" />}
              Switch
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showInstallConfirm} onOpenChange={setShowInstallConfirm}>
        <DialogContent>
          <DialogHeader severity="info" icon={HardDrives}>
            <DialogTitle>Install version {selectedVersion}?</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground">
            {getInstallDescription()}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowInstallConfirm(false)}>Cancel</Button>
            <Button onClick={handleInstallConfirm} disabled={commandRunning || !isConnected}>
              <DownloadSimple className="h-3.5 w-3.5 mr-1.5" />
              Download & install
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showIncompatWarning} onOpenChange={setShowIncompatWarning}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={WarningCircle}>
            <DialogTitle>Incompatible mods</DialogTitle>
            <DialogDescription>
              These installed mods are not compatible with {selectedVersion}:
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-48 overflow-y-auto space-y-1">
            {(selectedVersionCompat?.incompatible ?? []).map(pkg => (
              <div key={pkg} className="flex items-center gap-2 text-sm py-1">
                <X className="h-4 w-4 text-destructive flex-shrink-0" />
                {pkg}
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={handleInstallConfirm} disabled={commandRunning || !isConnected}>
              Install anyway
            </Button>
            <Button onClick={() => setShowIncompatWarning(false)}>
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showInstallProgress} onOpenChange={() => {}} closable={false}>
        <DialogContent>
          <DialogHeader severity="info" icon={HardDrives}>
            <DialogTitle>Version {selectedVersion}</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground mb-3">{installProgress.message}</div>
          <div className="h-[36px] flex items-center">
            {installProgress.stage === OSInstallStage.OSInstallInstalling || installProgress.stage === OSInstallStage.OSInstallFinalizing ? (
              <div className="w-full">
                <Progress indeterminate className="mb-2" indicatorClassName="bg-[var(--info)]" aria-label={`Installing ${selectedVersion}`} />
                <div className="h-4" />
              </div>
            ) : (
              <div className="w-full">
                <Progress value={installProgress.percent} className="mb-2" indicatorClassName="bg-[var(--info)]" />
                <div className="text-xs text-right text-muted-foreground tabular-nums">{Math.round(installProgress.percent)}%</div>
              </div>
            )}
          </div>
          <DialogFooter className="mt-4">
            {installProgress.stage === OSInstallStage.OSInstallDownloading || installProgress.stage === OSInstallStage.OSInstallUploading ? (
              <Button
                variant="outline"
                onClick={() => {
                  if (installRun) void OperationsService.CancelOperation(installRun.id)
                  setShowInstallProgress(false)
                }}
              >
                Cancel
              </Button>
            ) : (
              // Past the upload the reMarkable is writing its standby partition, which is not safe to stop.
              <Button variant="outline" onClick={() => setShowInstallProgress(false)}>
                Close
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showInstallComplete} onOpenChange={setShowInstallComplete}>
        <DialogContent>
          <DialogHeader severity="success" icon={Check}>
            <DialogTitle>Installation complete</DialogTitle>
            <DialogDescription>
              Version <strong className="text-card-foreground">{installedVersion}</strong> is installed on <strong className="text-card-foreground">Partition {standbySlot?.label}</strong>.
            </DialogDescription>
          </DialogHeader>
          {rebootError && (
            <Alert variant="destructive">
              <WarningCircle className="h-4 w-4" />
              <AlertDescription>{rebootError.message}</AlertDescription>
            </Alert>
          )}
          {rebootError && <OutputDisclosure output={rebootError.detail} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowInstallComplete(false)}>Done</Button>
            <Button onClick={handleReboot} disabled={commandRunning || !isConnected}>
              {rebooting && <CircleNotch className="h-4 w-4 mr-2 animate-spin" />}
              Reboot
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showRebootConfirm} onOpenChange={setShowRebootConfirm}>
        <DialogContent>
          <DialogHeader severity="info" icon={Power}>
            <DialogTitle>Reboot reMarkable?</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground">Your reMarkable will restart immediately.</div>
          {rebootError && (
            <Alert variant="destructive">
              <WarningCircle className="h-4 w-4" />
              <AlertDescription>{rebootError.message}</AlertDescription>
            </Alert>
          )}
          {rebootError && <OutputDisclosure output={rebootError.detail} />}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRebootConfirm(false)}>Cancel</Button>
            <Button onClick={handleReboot} disabled={commandRunning || !isConnected}>
              {rebooting && <CircleNotch className="h-4 w-4 mr-2 animate-spin" />}
              Reboot
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showInstallError} onOpenChange={setShowInstallError}>
        <DialogContent
          size={installErrorOutputOpen ? '2xl' : 'default'}
          className="transition-[max-width] duration-200 ease-out"
        >
          <DialogHeader severity="destructive" icon={WarningCircle}>
            <DialogTitle>Installation failed</DialogTitle>
            <DialogDescription>{installErrorMessage}</DialogDescription>
          </DialogHeader>
          {installErrorOutput && (
            <OutputDisclosure
              output={installErrorOutput}
              theme={resolvedTerminalTheme as 'dark' | 'light'}
              failed
              onOpenChange={setInstallErrorOutputOpen}
            />
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowInstallError(false)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CheckOSDialog
        open={!!checkOSVersion}
        onOpenChange={(open) => { if (!open) setCheckOSVersion(null) }}
        isConnected={isConnected}
        onSelectPackage={onSelectPackageForOS}
        initialVersion={checkOSVersion ?? undefined}
      />
    </>
  )
}
