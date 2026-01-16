import { useState, useEffect } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Loader2, AlertTriangle } from 'lucide-react'
import { Terminal } from '@/components/Terminal'

interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  vellumInstalled: boolean | null
  tabVisibility: Record<string, boolean>
  proxyMode: boolean
  onSaveSettings: (tabVisibility: Record<string, boolean>, proxyMode: boolean) => void
  onUninstallVellum: (removeAllPackages: boolean) => void
  uninstalling: boolean
  uninstallOutput: string
  appVersion: string
}

export function SettingsDialog({
  open,
  onOpenChange,
  isConnected,
  vellumInstalled,
  tabVisibility,
  proxyMode,
  onSaveSettings,
  onUninstallVellum,
  uninstalling,
  uninstallOutput,
  appVersion,
}: SettingsDialogProps) {
  const [localTabVisibility, setLocalTabVisibility] = useState(tabVisibility)
  const [localProxyMode, setLocalProxyMode] = useState(proxyMode)
  const [showUninstallConfirm, setShowUninstallConfirm] = useState(false)
  const [removePackages, setRemovePackages] = useState(true)

  useEffect(() => {
    if (open) {
      setLocalTabVisibility(tabVisibility)
      setLocalProxyMode(proxyMode)
    }
  }, [open, tabVisibility, proxyMode])

  const handleCancel = () => {
    setLocalTabVisibility(tabVisibility)
    setLocalProxyMode(proxyMode)
    setShowUninstallConfirm(false)
    onOpenChange(false)
  }

  const handleSave = () => {
    onSaveSettings(localTabVisibility, localProxyMode)
    onOpenChange(false)
  }

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setShowUninstallConfirm(false)
      setLocalTabVisibility(tabVisibility)
      setLocalProxyMode(proxyMode)
    }
    onOpenChange(newOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="w-[80vw] h-[80vh] max-w-none flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle>Settings</DialogTitle>
          <DialogDescription>
            Configure reManager preferences
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 flex-1 overflow-y-auto">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">Tab Visibility</CardTitle>
              <CardDescription className="text-xs">
                Choose which tabs to show in the app
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center justify-between">
                <Label htmlFor="tab-mods" className="font-normal">Mods</Label>
                <Switch
                  id="tab-mods"
                  checked={localTabVisibility.mods}
                  onCheckedChange={(v) => setLocalTabVisibility({ ...localTabVisibility, mods: v })}
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">Proxy Mode</CardTitle>
              <CardDescription className="text-xs">
                Download packages through reManager before installing on the tablet.
                This allows installing packages even if the tablet is not connected to the internet or has connectivity issues.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between">
                <Label htmlFor="proxy-mode" className="font-normal">Enable Proxy Mode</Label>
                <Switch
                  id="proxy-mode"
                  checked={localProxyMode}
                  onCheckedChange={setLocalProxyMode}
                />
              </div>
            </CardContent>
          </Card>

          {isConnected && vellumInstalled && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm">Uninstall</CardTitle>
                <CardDescription className="text-xs">
                  Remove the Vellum package manager from your reMarkable.
                  <br />
                  reManager will no longer be able to install or manage packages unless Vellum is reinstalled.
                </CardDescription>
              </CardHeader>
              <CardContent>
                {uninstalling ? (
                  <div className="space-y-3">
                    <div className="flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span className="text-sm">Uninstalling Vellum...</span>
                    </div>
                    {uninstallOutput && (
                      <div className="h-[200px] rounded-lg overflow-hidden">
                        <Terminal output={uninstallOutput} />
                      </div>
                    )}
                  </div>
                ) : !showUninstallConfirm ? (
                  <Button
                    variant="outline"
                    onClick={() => setShowUninstallConfirm(true)}
                    className="w-full"
                  >
                    Uninstall Vellum
                  </Button>
                ) : (
                  <div className="space-y-3">
                    <div className="flex items-center gap-2 text-sm text-destructive">
                      <AlertTriangle className="h-4 w-4" />
                      <span>This will remove Vellum from the device</span>
                    </div>
                    <div className="flex items-center justify-between">
                      <Label htmlFor="remove-pkgs" className="font-normal text-sm">
                        Also remove all installed packages
                      </Label>
                      <Switch
                        id="remove-pkgs"
                        checked={removePackages}
                        onCheckedChange={setRemovePackages}
                      />
                    </div>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        onClick={() => setShowUninstallConfirm(false)}
                        className="flex-1"
                      >
                        Cancel
                      </Button>
                      <Button
                        variant="destructive"
                        onClick={() => onUninstallVellum(removePackages)}
                        className="flex-1"
                      >
                        Confirm Uninstall
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>

        <DialogFooter className="flex-shrink-0 mt-4">
          <Button variant="outline" onClick={handleCancel}>
            Cancel
          </Button>
          <Button onClick={handleSave}>
            Save
          </Button>
        </DialogFooter>

        <div className="absolute bottom-4 left-0 right-0 text-sm text-muted-foreground text-center">
          <span>reManager {appVersion}</span>
          <span className="mx-2">·</span>
          <button
            onClick={() => window.runtime.BrowserOpenURL('https://github.com/rmitchellscott/remanager')}
            className="hover:underline"
          >
            GitHub
          </button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
