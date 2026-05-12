import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Badge } from '@/components/ui/badge'
import { CheckOSDialog } from '@/components/CheckOSDialog'
import { TimezoneCombobox } from '@/components/TimezoneCombobox'
import { Loader2, AlertCircle, X } from 'lucide-react'
import { useAppContext } from '@/contexts/AppContext'
import { MaintenanceCommandInfo, SystemTaskInfo, HashtabVersionStatus, TimezoneStatus, UpdateServiceStatus } from '@/lib/types'

interface MaintenanceTabProps {
  systemTasks: SystemTaskInfo[]
  maintenanceCommands: Record<string, MaintenanceCommandInfo[]>
  updateServiceStatus: UpdateServiceStatus
  xochitlRunning: boolean
  xoviActive: boolean
  hashtabMismatch: HashtabVersionStatus | null
  hashtabMissing: boolean
  reenableStatus: string
  upgradesAvailable: boolean
  timezoneMismatch: TimezoneStatus | null
  selectedTimezone: string
  deviceTimezone: string
  runningSystemTask: string | null
  currentRunningCommand: { componentId: string; commandId: string } | null
  settingTimezone: boolean
  showStartUIDialog: boolean
  setShowStartUIDialog: (v: boolean) => void
  handleSystemTask: (taskId: string) => Promise<void>
  handleComponentMaintenance: (componentId: string, commandId: string) => Promise<void>
  handleRunReenable: () => Promise<void>
  handleTimezoneChange: (timezone: string) => Promise<void>
  handleSetTimezone: () => void
  handleSelectPackageForOS: (name: string, targetOS: string, isCompatible?: boolean) => Promise<void>
  runningReenable: boolean
}

export function MaintenanceTab({
  systemTasks,
  maintenanceCommands,
  updateServiceStatus,
  xochitlRunning,
  xoviActive,
  hashtabMismatch,
  hashtabMissing,
  reenableStatus,
  upgradesAvailable,
  timezoneMismatch,
  selectedTimezone,
  deviceTimezone,
  runningSystemTask,
  currentRunningCommand,
  settingTimezone,
  showStartUIDialog,
  setShowStartUIDialog,
  handleSystemTask,
  handleComponentMaintenance,
  handleRunReenable,
  handleTimezoneChange,
  handleSetTimezone,
  handleSelectPackageForOS,
  runningReenable,
}: MaintenanceTabProps) {
  const { installedPackages, packages, commandRunning, connectionStatus, writeableRootBusy, vellumInstalled, startMaintenanceOperation } = useAppContext()

  const [showNoUpgradesDialog, setShowNoUpgradesDialog] = useState(false)
  const [pendingPackageUpgrade, setPendingPackageUpgrade] = useState<string[] | null>(null)
  const [showCheckOSDialog, setShowCheckOSDialog] = useState(false)
  const [simulatingUpgrade, setSimulatingUpgrade] = useState(false)

  const handleCheckUpgrades = async () => {
    setSimulatingUpgrade(true)
    try {
      const result = await window.go.main.App.SimulatePackageUpgrade()
      if (result.hasUpgrades) {
        setPendingPackageUpgrade(result.packages)
      } else {
        setShowNoUpgradesDialog(true)
      }
    } catch (err) {
      console.error('Failed to check upgrades:', err)
    } finally {
      setSimulatingUpgrade(false)
    }
  }

  const confirmPackageUpgrade = async () => {
    setPendingPackageUpgrade(null)
    startMaintenanceOperation({ resetProgress: false })
    await window.go.main.App.RunPackageUpgrade()
  }

  return (
    <>
      <TabsContent value="maintenance">
        <div className="space-y-6">
          {/* System Commands Section */}
          <Card>
            <CardHeader>
              <CardTitle>System Commands</CardTitle>
              <CardDescription>Device-level maintenance tasks</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {/* Auto-Update Status */}
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">Auto-Update Status:</span>
                  <Badge variant={updateServiceStatus.enabled ? 'default' : 'secondary'}>
                    {updateServiceStatus.enabled ? 'Enabled' : 'Disabled'}
                  </Badge>
                  <Badge variant={updateServiceStatus.running ? 'default' : 'secondary'}>
                    {updateServiceStatus.running ? 'Running' : 'Stopped'}
                  </Badge>
                </div>

                <div className="grid grid-cols-2 gap-4">
                  {systemTasks.map((task) => {
                    const isEnableDisabled = task.id === 'enable-updates' && updateServiceStatus.enabled && updateServiceStatus.running
                    const isDisableDisabled = task.id === 'disable-updates' && !updateServiceStatus.enabled && !updateServiceStatus.running
                    const shouldHighlight = (task.id === 'disable-updates' && updateServiceStatus.enabled) || (task.id === 'restart-xochitl' && !xochitlRunning)
                    const isRunning = runningSystemTask === task.id
                    return (
                      <Button
                        key={task.id}
                        onClick={() => handleSystemTask(task.id)}
                        disabled={commandRunning || isEnableDisabled || isDisableDisabled || connectionStatus !== 'connected' || (task.needsWriteableRoot && writeableRootBusy)}
                        variant={shouldHighlight ? 'default' : 'outline'}
                      >
                        {isRunning ? (
                          <>
                            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                            {task.label}
                          </>
                        ) : (
                          task.label
                        )}
                      </Button>
                    )
                  })}
                </div>

                {/* Timezone */}
                <div className="grid grid-cols-2 gap-4 pt-2">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium whitespace-nowrap">Timezone:</span>
                    <TimezoneCombobox
                      value={selectedTimezone || timezoneMismatch?.deviceTimezone || ''}
                      onChange={handleTimezoneChange}
                      disabled={connectionStatus !== 'connected'}
                    />
                  </div>
                  <Button
                    onClick={handleSetTimezone}
                    disabled={settingTimezone || connectionStatus !== 'connected' || !selectedTimezone || selectedTimezone === deviceTimezone || writeableRootBusy}
                    variant={timezoneMismatch?.needsUpdate ? 'default' : 'outline'}
                  >
                    {settingTimezone ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Set Timezone
                      </>
                    ) : (
                      'Set Timezone'
                    )}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Package Maintenance Section */}
          {vellumInstalled && (
            <Card>
              <CardHeader>
                <CardTitle>Package Maintenance</CardTitle>
                <CardDescription>Package-specific commands</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* Vellum Commands */}
                <div>
                  <h4 className="font-medium">Vellum</h4>
                  <p className="text-sm text-muted-foreground mb-2">Package manager for reMarkable tablets</p>
                  <div className="grid grid-cols-3 gap-2">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          onClick={handleRunReenable}
                          disabled={commandRunning || runningReenable || connectionStatus !== 'connected' || reenableStatus === 'unneeded' || writeableRootBusy}
                          variant={reenableStatus === 'needed' ? 'default' : 'outline'}
                          size="sm"
                          className="flex-1"
                        >
                          Reenable
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Reenable packages that modify the system partition</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          onClick={handleCheckUpgrades}
                          disabled={commandRunning || simulatingUpgrade || connectionStatus !== 'connected'}
                          variant={upgradesAvailable ? 'default' : 'outline'}
                          size="sm"
                          className="flex-1"
                        >
                          {simulatingUpgrade ? 'Checking...' : 'Upgrade'}
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Check for and install package updates</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          onClick={() => setShowCheckOSDialog(true)}
                          disabled={connectionStatus !== 'connected'}
                          variant="outline"
                          size="sm"
                          className="flex-1"
                        >
                          Check OS
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Check package compatibility with a target OS version</TooltipContent>
                    </Tooltip>
                  </div>
                </div>

                {/* Separator if there are package commands */}
                {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]?.length > 0).length > 0 && (
                  <div className="border-t" />
                )}

                {/* Package-specific Commands */}
                {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]?.length > 0).length > 0 && (
                  <div className="space-y-4">
                    {packages.filter(p => installedPackages.has(p.name) && maintenanceCommands[p.name]).sort((a, b) => a.name.localeCompare(b.name)).map((pkg) => (
                      <div key={pkg.name}>
                        <h4 className="font-medium">{pkg.name}</h4>
                        {pkg.description && <p className="text-sm text-muted-foreground mb-2">{pkg.description}</p>}
                        <div className="grid grid-cols-3 gap-2">
                          {maintenanceCommands[pkg.name]?.map((cmd) => {
                            const isRunning = currentRunningCommand?.componentId === pkg.name &&
                                             currentRunningCommand?.commandId === cmd.id
                            const isHashtabRebuild = pkg.name === 'qt-resource-rebuilder' && cmd.id === 'rebuild_hashtable'
                            const isXoviStart = pkg.name === 'xovi' && cmd.id === 'start'
                            const xoviNeedsStart = isXoviStart && xochitlRunning && !xoviActive
                            const shouldHighlight = (isHashtabRebuild && (hashtabMismatch || hashtabMissing)) || (xoviNeedsStart && !hashtabMismatch && !hashtabMissing)
                            const isDisabledByMismatch = isXoviStart && (!!hashtabMismatch || hashtabMissing)

                            return (
                              <div key={cmd.id} className="flex gap-2">
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      onClick={() => handleComponentMaintenance(pkg.name, cmd.id)}
                                      disabled={isDisabledByMismatch || (commandRunning && !isRunning) || connectionStatus !== 'connected' || writeableRootBusy}
                                      variant={shouldHighlight ? 'default' : 'outline'}
                                      size="sm"
                                      className="flex-1"
                                    >
                                      {cmd.label}
                                    </Button>
                                  </TooltipTrigger>
                                  {cmd.description && (
                                    <TooltipContent>{cmd.description}</TooltipContent>
                                  )}
                                </Tooltip>
                              </div>
                            )
                          })}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </TabsContent>

      {/* Package Upgrade Confirmation Dialog */}
      <Dialog open={pendingPackageUpgrade !== null} onOpenChange={(open) => !open && setPendingPackageUpgrade(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Upgrade Packages</DialogTitle>
            <DialogDescription>
              The following {pendingPackageUpgrade?.length} package{pendingPackageUpgrade?.length !== 1 ? 's' : ''} will be upgraded:
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[40vh] overflow-y-auto">
            <ul className="list-disc list-inside space-y-1 text-sm">
              {pendingPackageUpgrade?.sort().map((pkg) => (
                <li key={pkg}>{pkg}</li>
              ))}
            </ul>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingPackageUpgrade(null)}>
              Cancel
            </Button>
            <Button onClick={confirmPackageUpgrade}>
              Upgrade ({pendingPackageUpgrade?.length})
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* No Updates Available Dialog */}
      <Dialog open={showNoUpgradesDialog} onOpenChange={setShowNoUpgradesDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>No Updates Available</DialogTitle>
            <DialogDescription>
              All packages are up to date.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setShowNoUpgradesDialog(false)}>OK</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Start UI Chooser Dialog */}
      <Dialog open={showStartUIDialog} onOpenChange={setShowStartUIDialog}>
        <DialogContent className="relative">
          <Button
            variant="ghost"
            size="xs"
            className="absolute right-2 top-2"
            onClick={() => setShowStartUIDialog(false)}
          >
            <X className="h-3 w-3" />
          </Button>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-yellow-500" />
              reMarkable UI stopped
            </DialogTitle>
            <DialogDescription>
              The reMarkable interface is currently stopped. How would you like to start it?
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2 pt-2">
            <Button
              className="w-full justify-center gap-2"
              disabled={!!hashtabMismatch || hashtabMissing}
              onClick={() => {
                setShowStartUIDialog(false)
                handleComponentMaintenance('xovi', 'start')
              }}
            >
              Start UI with Mods
            </Button>
            {(hashtabMismatch || hashtabMissing) && (
              <p className="text-xs text-muted-foreground text-center -mt-1">
                {hashtabMissing ? 'Disabled — hashtable not built' : 'Disabled due to hashtable mismatch'}
              </p>
            )}
            <Button
              variant="outline"
              className="w-full justify-center gap-2"
              onClick={() => {
                setShowStartUIDialog(false)
                handleSystemTask('restart-xochitl')
              }}
            >
              Start UI without Mods
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <CheckOSDialog
        open={showCheckOSDialog}
        onOpenChange={setShowCheckOSDialog}
        isConnected={connectionStatus === 'connected'}
        onSelectPackage={handleSelectPackageForOS}
      />
    </>
  )
}
