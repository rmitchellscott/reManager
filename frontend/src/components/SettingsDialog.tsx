import { useState, useEffect } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Loader2, AlertTriangle, AlertCircle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'

interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  vellumInstalled: boolean | null
  tabVisibility: Record<string, boolean>
  proxyMode: boolean
  suppressSystemFileWarnings: boolean
  preventSleep: boolean
  theme: string
  terminalTheme: string
  editorTheme: string
  checkForUpdates: boolean
  onSaveSettings: (tabVisibility: Record<string, boolean>, proxyMode: boolean, suppressSystemFileWarnings: boolean, preventSleep: boolean, theme: string, terminalTheme: string, editorTheme: string, checkForUpdates: boolean) => void
  onUninstallVellum: (removeAllPackages: boolean) => void
  uninstalling: boolean
  uninstallOutput: string
  uninstallError: string | null
  appVersion: string
}

export function SettingsDialog({
  open,
  onOpenChange,
  isConnected,
  vellumInstalled,
  tabVisibility,
  proxyMode,
  suppressSystemFileWarnings,
  preventSleep,
  theme,
  terminalTheme,
  editorTheme,
  checkForUpdates,
  onSaveSettings,
  onUninstallVellum,
  uninstalling,
  uninstallOutput,
  uninstallError,
  appVersion,
}: SettingsDialogProps) {
  const defaultTabVisibility = { mods: true, maintenance: true, utilities: true }

  const [localTabVisibility, setLocalTabVisibility] = useState({ ...defaultTabVisibility, ...tabVisibility })
  const [localProxyMode, setLocalProxyMode] = useState(proxyMode)
  const [localSuppressSystemFileWarnings, setLocalSuppressSystemFileWarnings] = useState(suppressSystemFileWarnings)
  const [localPreventSleep, setLocalPreventSleep] = useState(preventSleep)
  const [localTheme, setLocalTheme] = useState(theme)
  const [localTerminalTheme, setLocalTerminalTheme] = useState(terminalTheme)
  const [localEditorTheme, setLocalEditorTheme] = useState(editorTheme)
  const [localCheckForUpdates, setLocalCheckForUpdates] = useState(checkForUpdates)
  const [showUninstallConfirm, setShowUninstallConfirm] = useState(false)
  const [removePackages, setRemovePackages] = useState(true)

  const hasChanges =
    localProxyMode !== proxyMode ||
    localSuppressSystemFileWarnings !== suppressSystemFileWarnings ||
    localPreventSleep !== preventSleep ||
    localTheme !== theme ||
    localTerminalTheme !== terminalTheme ||
    localEditorTheme !== editorTheme ||
    localCheckForUpdates !== checkForUpdates ||
    JSON.stringify(localTabVisibility) !== JSON.stringify({ ...defaultTabVisibility, ...tabVisibility })

  useEffect(() => {
    if (open) {
      setLocalTabVisibility({ ...defaultTabVisibility, ...tabVisibility })
      setLocalProxyMode(proxyMode)
      setLocalSuppressSystemFileWarnings(suppressSystemFileWarnings)
      setLocalPreventSleep(preventSleep)
      setLocalTheme(theme)
      setLocalTerminalTheme(terminalTheme)
      setLocalEditorTheme(editorTheme)
      setLocalCheckForUpdates(checkForUpdates)
    }
  }, [open, tabVisibility, proxyMode, suppressSystemFileWarnings, preventSleep, theme, terminalTheme, editorTheme, checkForUpdates])

  const handleCancel = () => {
    setLocalTabVisibility({ ...defaultTabVisibility, ...tabVisibility })
    setLocalProxyMode(proxyMode)
    setLocalSuppressSystemFileWarnings(suppressSystemFileWarnings)
    setLocalPreventSleep(preventSleep)
    setLocalTheme(theme)
    setLocalTerminalTheme(terminalTheme)
    setLocalEditorTheme(editorTheme)
    setLocalCheckForUpdates(checkForUpdates)
    setShowUninstallConfirm(false)
    onOpenChange(false)
  }

  const handleSave = () => {
    onSaveSettings(localTabVisibility, localProxyMode, localSuppressSystemFileWarnings, localPreventSleep, localTheme, localTerminalTheme, localEditorTheme, localCheckForUpdates)
    onOpenChange(false)
  }

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setShowUninstallConfirm(false)
      setLocalTabVisibility({ ...defaultTabVisibility, ...tabVisibility })
      setLocalProxyMode(proxyMode)
      setLocalSuppressSystemFileWarnings(suppressSystemFileWarnings)
      setLocalPreventSleep(preventSleep)
      setLocalTheme(theme)
      setLocalTerminalTheme(terminalTheme)
      setLocalEditorTheme(editorTheme)
      setLocalCheckForUpdates(checkForUpdates)
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
              <CardTitle className="text-base">Appearance</CardTitle>
              <CardDescription className="text-xs">
                Customize the look of reManager
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between gap-4">
                <Label htmlFor="theme-select" className="font-normal">Theme</Label>
                <Select value={localTheme} onValueChange={setLocalTheme}>
                  <SelectTrigger id="theme-select" className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="light">Light</SelectItem>
                    <SelectItem value="dark">Dark</SelectItem>
                    <SelectItem value="system">System</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center justify-between gap-4">
                <Label htmlFor="terminal-theme-select" className="font-normal">Terminal Theme</Label>
                <Select value={localTerminalTheme} onValueChange={setLocalTerminalTheme}>
                  <SelectTrigger id="terminal-theme-select" className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="match">Match reManager</SelectItem>
                    <SelectItem value="dark">Dark</SelectItem>
                    <SelectItem value="light">Light</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center justify-between gap-4">
                <Label htmlFor="editor-theme-select" className="font-normal">Editor Theme</Label>
                <Select value={localEditorTheme} onValueChange={setLocalEditorTheme}>
                  <SelectTrigger id="editor-theme-select" className="w-40">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="match">Match reManager</SelectItem>
                    <SelectItem value="dark">Dark</SelectItem>
                    <SelectItem value="light">Light</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center justify-between">
                <Label htmlFor="tab-mods" className="font-normal">Show Mods Tab</Label>
                <Switch
                  id="tab-mods"
                  checked={localTabVisibility.mods}
                  onCheckedChange={(v) => setLocalTabVisibility({ ...localTabVisibility, mods: v })}
                />
              </div>
              <div className="flex items-center justify-between">
                <Label htmlFor="tab-utilities" className="font-normal">Show Utilities Tab</Label>
                <Switch
                  id="tab-utilities"
                  checked={localTabVisibility.utilities}
                  onCheckedChange={(v) => setLocalTabVisibility({ ...localTabVisibility, utilities: v })}
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Behavior</CardTitle>
              <CardDescription className="text-xs">
                Configure how reManager operates
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Label htmlFor="proxy-mode" className="font-normal">Proxy Mode</Label>
                  <p className="text-xs text-muted-foreground mt-1">
                    Download packages through reManager before installing on the tablet.
                    Useful if the tablet has limited internet connectivity.
                  </p>
                </div>
                <Switch
                  id="proxy-mode"
                  checked={localProxyMode}
                  onCheckedChange={setLocalProxyMode}
                />
              </div>
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Label htmlFor="prevent-sleep" className="font-normal">Prevent Sleep</Label>
                  <p className="text-xs text-muted-foreground mt-1">
                    Keep the device awake while connected by simulating periodic input.
                  </p>
                </div>
                <Switch
                  id="prevent-sleep"
                  checked={localPreventSleep}
                  onCheckedChange={setLocalPreventSleep}
                />
              </div>
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Label htmlFor="check-updates" className="font-normal">Check for Updates</Label>
                  <p className="text-xs text-muted-foreground mt-1">
                    Notify when a new version of reManager is available.
                  </p>
                </div>
                <Switch
                  id="check-updates"
                  checked={localCheckForUpdates}
                  onCheckedChange={setLocalCheckForUpdates}
                />
              </div>
              <div className="flex items-center justify-between gap-4">
                <div>
                  <Label htmlFor="suppress-sys-warnings" className="font-normal">Suppress Filesystem Warnings</Label>
                  <p className="text-xs text-muted-foreground mt-1">
                    Skip confirmation dialogs when modifying system partition files.
                    Only suppress if you are experienced with Linux systems.
                  </p>
                </div>
                <Switch
                  id="suppress-sys-warnings"
                  checked={localSuppressSystemFileWarnings}
                  onCheckedChange={setLocalSuppressSystemFileWarnings}
                />
              </div>
            </CardContent>
          </Card>

          {isConnected && vellumInstalled && (
            <Card className="border-destructive/50">
              <CardHeader className="pb-3">
                <CardTitle className="text-base">Uninstall</CardTitle>
                <CardDescription className="text-xs">
                  Remove the Vellum package manager from your reMarkable.
                  <br />
                  reManager will no longer be able to install or manage packages unless Vellum is reinstalled.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {uninstalling && (
                  <div className="flex items-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    <span className="text-sm">Uninstalling Vellum...</span>
                  </div>
                )}

                {uninstallError && (
                  <Alert variant="destructive">
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>Uninstall failed: {uninstallError}</AlertDescription>
                  </Alert>
                )}

                {(uninstalling || uninstallError) && uninstallOutput && (
                  <div className="h-[200px] rounded-lg overflow-hidden">
                    <TerminalWithCopy output={uninstallOutput} />
                  </div>
                )}

                {uninstallError && (
                  <Button
                    variant="outline"
                    onClick={() => onUninstallVellum(removePackages)}
                    className="w-full"
                  >
                    Retry Uninstall
                  </Button>
                )}

                {!uninstalling && !uninstallError && !showUninstallConfirm && (
                  <Button
                    variant="outline"
                    onClick={() => setShowUninstallConfirm(true)}
                    className="w-full"
                  >
                    Uninstall Vellum
                  </Button>
                )}

                {!uninstalling && !uninstallError && showUninstallConfirm && (
                  <>
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
                  </>
                )}
              </CardContent>
            </Card>
          )}
        </div>

        <DialogFooter className="flex-shrink-0 mt-4">
          <Button variant="outline" onClick={handleCancel}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={!hasChanges}>
            Save
          </Button>
        </DialogFooter>

        <div className="flex-shrink-0 text-sm text-muted-foreground text-center pt-2">
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
