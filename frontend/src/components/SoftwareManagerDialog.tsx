import { useState, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Dialog,
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
import { RefreshCw, Download, Check, Loader2, X, AlertCircle } from 'lucide-react'
import { toast } from 'sonner'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'
import { CheckOSDialog } from '@/components/CheckOSDialog'

interface PartitionSlot {
  number: number
  version: string
  label: string
  isActive: boolean
  isNextBoot: boolean
}

interface SystemInfo {
  active: PartitionSlot
  fallback: PartitionSlot
  deviceType: string
  encrypted: boolean
  updatePending: boolean
}

interface OSVersionInfo {
  version: string
  isLatest: boolean
  isInstalled: boolean
}

interface InstallProgress {
  phase: string
  percent: number
  message: string
}

interface CompatibilityResult {
  compatible: string[]
  incompatible: string[]
  noConstraint: string[]
  fetchFailed: boolean
}

interface SoftwareManagerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  vellumInstalled: boolean | null
  onSelectPackageForOS: (name: string, targetOS: string, isCompatible: boolean) => void
}

export function SoftwareManagerDialog({ open, onOpenChange, isConnected, vellumInstalled, onSelectPackageForOS }: SoftwareManagerDialogProps) {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [versions, setVersions] = useState<OSVersionInfo[]>([])
  const [versionsFailed, setVersionsFailed] = useState(false)
  const [loading, setLoading] = useState(true)
  const [selectedVersion, setSelectedVersion] = useState('')
  const [installNeedsReboot, setInstallNeedsReboot] = useState(false)

  const [showSwitchConfirm, setShowSwitchConfirm] = useState(false)
  const [showInstallConfirm, setShowInstallConfirm] = useState(false)
  const [showInstallProgress, setShowInstallProgress] = useState(false)
  const [showInstallComplete, setShowInstallComplete] = useState(false)
  const [showRebootConfirm, setShowRebootConfirm] = useState(false)
  const [switching, setSwitching] = useState(false)

  const [installProgress, setInstallProgress] = useState<InstallProgress>({ phase: '', percent: 0, message: 'Downloading...' })
  const [installedVersion, setInstalledVersion] = useState('')
  const [showInstallError, setShowInstallError] = useState(false)
  const [installErrorMessage, setInstallErrorMessage] = useState('')
  const [installErrorOutput, setInstallErrorOutput] = useState('')

  const [compatCache, setCompatCache] = useState<Record<string, CompatibilityResult>>({})
  const [partitionCompatLoading, setPartitionCompatLoading] = useState(false)
  const [versionCompatLoading, setVersionCompatLoading] = useState(false)
  const [checkOSVersion, setCheckOSVersion] = useState<string | null>(null)
  const [showIncompatWarning, setShowIncompatWarning] = useState(false)

  const checkCompatibility = useCallback(async (version: string): Promise<CompatibilityResult | null> => {
    try {
      const res = await (window as any).go.main.App.CheckOSCompatibility(version)
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
    setVersionsFailed(false)
    try {
      const info = await window.go.main.App.GetPartitionInfo()
      setSystemInfo(info)

      try {
        const availableVersions = await window.go.main.App.GetAvailableOSVersions()
        const installed = new Set([info.active.version, info.fallback.version])
        setVersions((availableVersions || []).map(v => ({
          ...v,
          isInstalled: installed.has(v.version),
        })))
      } catch {
        setVersionsFailed(true)
      }
    } catch {
      toast.error('Failed to load partition info')
    } finally {
      setLoading(false)
    }
  }, [isConnected])

  useEffect(() => {
    if (open && isConnected) {
      loadData()
      setInstallNeedsReboot(false)
      setSelectedVersion('')
      setCompatCache({})
    }
  }, [open, isConnected, loadData])

  useEffect(() => {
    if (!open || !systemInfo || !vellumInstalled) return

    const versions = [systemInfo.active.version, systemInfo.fallback.version]
    const unchecked = versions.filter(v => !compatCache[v])
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

  useEffect(() => {
    if (!open) return

    const cleanupProgress = window.runtime.EventsOn('software:install-progress', (...args: unknown[]) => {
      const progress = args[0] as InstallProgress
      setInstallProgress(progress)
    })

    const cleanupComplete = window.runtime.EventsOn('software:install-complete', (...args: unknown[]) => {
      const data = args[0] as { version: string; partition: string }
      setShowInstallProgress(false)
      setInstalledVersion(data.version)
      setShowInstallComplete(true)
      setInstallNeedsReboot(true)
      loadData()
    })

    const cleanupError = window.runtime.EventsOn('software:install-error', (...args: unknown[]) => {
      const data = args[0] as { error: string; output?: string }
      setShowInstallProgress(false)
      if (data.error === 'cancelled') return
      setInstallErrorMessage(data.error)
      setInstallErrorOutput(data.output || '')
      setShowInstallError(true)
    })

    return () => {
      cleanupProgress()
      cleanupComplete()
      cleanupError()
    }
  }, [open, loadData])

  const slotA = systemInfo ? (systemInfo.active.number === 2 ? systemInfo.active : systemInfo.fallback) : null
  const slotB = systemInfo ? (systemInfo.active.number === 3 ? systemInfo.active : systemInfo.fallback) : null

  const standbySlot = systemInfo ? systemInfo.fallback : null
  const bootSwitchPending = systemInfo ? !systemInfo.active.isNextBoot : false
  const needsReboot = bootSwitchPending || installNeedsReboot

  const selectedVersionCompat = selectedVersion ? compatCache[selectedVersion] : null
  const selectedVersionHasIncompat = selectedVersionCompat ? selectedVersionCompat.incompatible.length > 0 : false

  const handleInstallClick = () => {
    if (selectedVersionHasIncompat) {
      setShowIncompatWarning(true)
    } else {
      setShowInstallConfirm(true)
    }
  }

  const handleSwitchConfirm = async () => {
    if (!standbySlot) return
    setSwitching(true)
    try {
      const nextBootTarget = systemInfo!.active.isNextBoot
        ? systemInfo!.fallback.number
        : systemInfo!.active.number
      await window.go.main.App.SwitchBootPartition(nextBootTarget)
      setShowSwitchConfirm(false)
      await loadData()
    } catch (err: any) {
      toast.error(`Failed to switch partition: ${err?.message || err}`)
    } finally {
      setSwitching(false)
    }
  }

  const handleInstallConfirm = () => {
    if (!selectedVersion) return
    setShowInstallConfirm(false)
    setShowIncompatWarning(false)
    setInstallProgress({ phase: 'downloading', percent: 0, message: 'Downloading...' })
    setShowInstallProgress(true)
    window.go.main.App.InstallOSVersion(selectedVersion)
  }

  const handleReboot = async () => {
    try {
      await window.go.main.App.RebootDevice()
      setShowRebootConfirm(false)
      onOpenChange(false)
    } catch (err: any) {
      toast.error(`Failed to reboot: ${err?.message || err}`)
    }
  }

  const getNextBootSlot = () => {
    if (!systemInfo) return null
    return systemInfo.active.isNextBoot ? systemInfo.fallback : systemInfo.active
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
    const result = compatCache[version]
    if (!result) {
      if (partitionCompatLoading) {
        return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      }
      return null
    }
    const hasIncompat = result.incompatible.length > 0
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button onClick={() => setCheckOSVersion(version)}>
            {hasIncompat ? (
              <X className="h-4 w-4 text-destructive" />
            ) : (
              <Check className="h-4 w-4 text-green-500" />
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent>View package compatibility details</TooltipContent>
      </Tooltip>
    )
  }

  const renderSlotCard = (slot: PartitionSlot | null, label: string) => {
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
      <div className={`border rounded-[10px] p-4 transition-colors ${highlighted ? 'border-foreground' : ''}`}>
        <div className="flex items-center justify-between mb-1.5">
          <span className="text-sm font-semibold">Partition {label}</span>
          <Badge variant={badgeVariant} className="text-[11px] px-2 py-0">
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
              <span className="text-[11px] text-muted-foreground">Packages</span>
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
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Checking packages...
        </div>
      )
    }

    const result = compatCache[selectedVersion]
    const hasIncompat = result.incompatible.length > 0
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
              {hasIncompat ? 'Package issues found' : 'Packages compatible'}
            </button>
          </TooltipTrigger>
          <TooltipContent>View package compatibility details</TooltipContent>
        </Tooltip>
      </div>
    )
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="w-[92vw] sm:w-[56vw] sm:max-w-3xl max-h-[85vh] flex flex-col overflow-y-auto p-7 relative">
          <button
            onClick={() => onOpenChange(false)}
            className="absolute top-4 right-4 rounded-sm opacity-70 hover:opacity-100 transition-opacity"
          >
            <X className="h-4 w-4" />
          </button>
          <DialogHeader className="mb-3">
            <DialogTitle className="text-base">OS Manager</DialogTitle>
            <DialogDescription className="text-xs">
              View, install, and switch OS versions.
            </DialogDescription>
          </DialogHeader>

          {loading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : systemInfo ? (
            <>
              <div className="grid grid-cols-2 gap-3">
                {renderSlotCard(slotA, 'A')}
                {renderSlotCard(slotB, 'B')}
              </div>

              <div className="h-px bg-border my-4" />

              <div className="flex flex-col gap-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium">Switch boot partition</div>
                    <div className="text-xs text-muted-foreground">Set the active boot partition.</div>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setShowSwitchConfirm(true)}
                    disabled={installNeedsReboot || systemInfo?.updatePending}
                    className="shrink-0"
                  >
                    <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
                    Switch
                  </Button>
                </div>

                <div className="flex flex-col gap-2.5">
                  <div>
                    <div className="text-sm font-medium">Install a version</div>
                    <div className="text-xs text-muted-foreground">Download and install to the standby partition.</div>
                  </div>
                  <div className="flex gap-2">
                    <Select value={selectedVersion} onValueChange={setSelectedVersion} disabled={needsReboot || versionsFailed}>
                      <SelectTrigger className="flex-1 h-9">
                        <SelectValue placeholder={needsReboot ? "Reboot to install a version" : versionsFailed ? "Could not load versions" : "Choose a version..."}>{selectedVersion || undefined}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {versions.map((v) => (
                          <SelectItem key={v.version} value={v.version} className="pl-3 [&>span:first-child]:hidden [&>span:last-child]:flex-1">
                            <span className="flex items-center w-full">
                              <span>{v.version}</span>
                              {getVersionLabel(v.version) && (
                                <span className="text-[11px] text-muted-foreground ml-auto pl-3">{getVersionLabel(v.version)}</span>
                              )}
                            </span>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Button
                      size="sm"
                      className="h-9"
                      disabled={!selectedVersion || needsReboot || versionsFailed}
                      onClick={handleInstallClick}
                    >
                      Install
                    </Button>
                  </div>
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
              <p>Connect to a device to view partition information</p>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Switch boot confirmation */}
      <Dialog open={showSwitchConfirm} onOpenChange={setShowSwitchConfirm} priority>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-base">Switch boot partition?</DialogTitle>
            <DialogDescription>
              On next restart, your reMarkable will boot from a different partition.
            </DialogDescription>
          </DialogHeader>
          {systemInfo && (
            <div className="flex gap-3 items-center">
              <div className="flex-1 p-2.5 rounded-lg border bg-muted">
                <div className="text-[11px] text-muted-foreground mb-0.5">Current</div>
                <div className="text-[13px] font-semibold">{getCurrentBootSlot()?.version}</div>
                <div className="text-xs text-muted-foreground">Partition {getCurrentBootSlot()?.label}</div>
              </div>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground shrink-0">
                <line x1="5" y1="12" x2="19" y2="12" /><polyline points="12 5 19 12 12 19" />
              </svg>
              <div className="flex-1 p-2.5 rounded-lg border border-foreground">
                <div className="text-[11px] text-muted-foreground mb-0.5">Next boot</div>
                <div className="text-[13px] font-semibold">{getNextBootSlot()?.version}</div>
                <div className="text-xs text-muted-foreground">Partition {getNextBootSlot()?.label}</div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSwitchConfirm(false)}>Cancel</Button>
            <Button onClick={handleSwitchConfirm} disabled={switching}>
              {switching && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Switch
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Install confirmation */}
      <Dialog open={showInstallConfirm} onOpenChange={setShowInstallConfirm} priority>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-base">Install version {selectedVersion}?</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground">
            {getInstallDescription()}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowInstallConfirm(false)}>Cancel</Button>
            <Button onClick={handleInstallConfirm}>
              <Download className="h-3.5 w-3.5 mr-1.5" />
              Download & install
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Incompatibility warning */}
      <Dialog open={showIncompatWarning} onOpenChange={setShowIncompatWarning} priority>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-destructive" />
              Incompatible packages
            </DialogTitle>
            <DialogDescription>
              The following installed packages are not compatible with {selectedVersion}:
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-48 overflow-y-auto space-y-1">
            {selectedVersionCompat?.incompatible.map(pkg => (
              <div key={pkg} className="flex items-center gap-2 text-sm py-1">
                <X className="h-4 w-4 text-destructive flex-shrink-0" />
                {pkg}
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={handleInstallConfirm}>
              Install Anyway
            </Button>
            <Button onClick={() => setShowIncompatWarning(false)}>
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Install progress */}
      <Dialog open={showInstallProgress} onOpenChange={() => {}} closable={false} priority>
        <DialogContent className="sm:min-w-[420px]">
          <DialogHeader>
            <DialogTitle className="text-base">Installing {selectedVersion}</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground mb-3">{installProgress.message}</div>
          <div className="h-[36px] flex items-center">
            {installProgress.phase === 'installing' || installProgress.phase === 'finalizing' ? (
              <div className="flex items-center justify-center w-full">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <div className="w-full">
                <Progress value={installProgress.percent} className="mb-2" />
                <div className="text-xs text-right text-muted-foreground">{Math.round(installProgress.percent)}%</div>
              </div>
            )}
          </div>
          <DialogFooter className="mt-4">
            <Button
              variant="outline"
              disabled={installProgress.phase !== 'downloading' && installProgress.phase !== 'uploading'}
              onClick={() => {
                window.go.main.App.CancelOSInstall()
                setShowInstallProgress(false)
              }}
            >
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Install complete */}
      <Dialog open={showInstallComplete} onOpenChange={setShowInstallComplete} priority>
        <DialogContent className="text-center">
          <div className="w-12 h-12 rounded-full bg-muted inline-flex items-center justify-center mx-auto mb-3">
            <Check className="h-6 w-6" />
          </div>
          <DialogHeader className="items-center">
            <DialogTitle className="text-base">Installation complete</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground mb-2">
            Version <strong className="text-card-foreground">{installedVersion}</strong> has been installed to <strong className="text-card-foreground">Partition {standbySlot?.label}</strong>.
          </div>
          <DialogFooter className="justify-center">
            <Button variant="outline" onClick={() => setShowInstallComplete(false)}>Done</Button>
            <Button variant="destructive" onClick={() => { setShowInstallComplete(false); handleReboot() }}>Reboot</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reboot confirmation */}
      <Dialog open={showRebootConfirm} onOpenChange={setShowRebootConfirm} priority>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-base">Reboot device?</DialogTitle>
          </DialogHeader>
          <div className="text-sm text-muted-foreground">Your reMarkable will restart immediately.</div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRebootConfirm(false)}>Cancel</Button>
            <Button variant="destructive" onClick={handleReboot}>Reboot</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Install error */}
      <Dialog open={showInstallError} onOpenChange={setShowInstallError} priority>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-destructive" />
              Installation failed
            </DialogTitle>
            <DialogDescription>{installErrorMessage}</DialogDescription>
          </DialogHeader>
          {installErrorOutput && (
            <div className="h-[200px]">
              <TerminalWithCopy output={installErrorOutput} />
            </div>
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
        priority
      />
    </>
  )
}
