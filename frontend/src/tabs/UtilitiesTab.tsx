import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { InteractiveTerminal } from '@/components/InteractiveTerminal'
import { FileBrowser } from '@/components/FileBrowser'
import { ConfigEditor } from '@/components/ConfigEditor'
import { BackupRestoreDialog } from '@/components/BackupRestore'
import { ImportPDFDialog } from '@/components/ImportPDFDialog'
import { SoftwareManagerDialog } from '@/components/SoftwareManagerDialog'
import { PageHeader } from '@/components/PageHeader'
import { X } from 'lucide-react'
import { useAppContext } from '@/contexts/AppContext'

interface UtilitiesTabProps {
  activeTab: string
  handleSelectPackageForOS: (name: string, targetOS: string, isCompatible?: boolean) => Promise<void>
  onSettings: () => void
  onDisconnect: () => Promise<void>
}

export function UtilitiesTab({ activeTab, handleSelectPackageForOS, onSettings, onDisconnect }: UtilitiesTabProps) {
  const ctx = useAppContext()
  const { connectionStatus, device, deviceInfo, installedPackages, vellumInstalled, suppressSystemFileWarnings } = ctx
  const resolvedTerminalTheme = ctx.resolvedTerminalTheme as 'dark' | 'light'
  const resolvedEditorTheme = ctx.resolvedEditorTheme as 'dark' | 'light'

  const [isTerminalRunning, setIsTerminalRunning] = useState(false)
  const [showFileBrowser, setShowFileBrowser] = useState(false)
  const [showConfigEditor, setShowConfigEditor] = useState(false)
  const [showImportPDF, setShowImportPDF] = useState(false)
  const [showSoftwareManager, setShowSoftwareManager] = useState(false)
  const [backupDialogMode, setBackupDialogMode] = useState<'backup' | 'restore' | null>(null)

  return (
    <>
      <TabsContent value="utilities" forceMount className={activeTab === 'utilities' ? '' : 'hidden'}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Card className={isTerminalRunning ? 'md:col-span-2' : ''}>
            <CardHeader>
              <CardTitle>Terminal</CardTitle>
              <CardDescription>Interactive SSH shell</CardDescription>
            </CardHeader>
            <CardContent>
              <InteractiveTerminal
                isConnected={connectionStatus === 'connected'}
                visible={activeTab === 'utilities'}
                onRunningChange={setIsTerminalRunning}
                theme={resolvedTerminalTheme}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>File Browser</CardTitle>
              <CardDescription>Browse and transfer files</CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => setShowFileBrowser(true)}
                disabled={connectionStatus !== 'connected'}
              >
                Open File Browser
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Configuration Editor</CardTitle>
              <CardDescription>Edit settings</CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => setShowConfigEditor(true)}
                disabled={connectionStatus !== 'connected'}
              >
                xochitl.conf
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Backup & Restore</CardTitle>
              <CardDescription>Manage device backups</CardDescription>
            </CardHeader>
            <CardContent className="flex gap-2">
              <Button
                className="flex-1"
                variant="outline"
                onClick={() => setBackupDialogMode('backup')}
                disabled={connectionStatus !== 'connected'}
              >
                Backup
              </Button>
              <Button
                className="flex-1"
                variant="outline"
                onClick={() => setBackupDialogMode('restore')}
                disabled={connectionStatus !== 'connected'}
              >
                Restore
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Document Import</CardTitle>
              <CardDescription>Upload documents to your reMarkable library</CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => setShowImportPDF(true)}
                disabled={connectionStatus !== 'connected'}
              >
                Import
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>OS Manager</CardTitle>
              <CardDescription>View, install, and switch OS versions.</CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                className="w-full"
                variant="outline"
                onClick={() => setShowSoftwareManager(true)}
                disabled={connectionStatus !== 'connected'}
              >
                Manage
              </Button>
            </CardContent>
          </Card>
        </div>
      </TabsContent>

      <ImportPDFDialog
        open={showImportPDF}
        isConnected={connectionStatus === 'connected'}
        hasLibrarian={installedPackages.has('librarian')}
        onOpenChange={setShowImportPDF}
      />

      <SoftwareManagerDialog
        open={showSoftwareManager}
        onOpenChange={setShowSoftwareManager}
        isConnected={connectionStatus === 'connected'}
        vellumInstalled={vellumInstalled}
        onSelectPackageForOS={handleSelectPackageForOS}
      />

      <BackupRestoreDialog
        mode={backupDialogMode}
        onClose={() => setBackupDialogMode(null)}
      />

      {showFileBrowser && (
        <div className="fixed inset-0 z-50 bg-background overflow-auto">
          <div className="min-h-screen p-6">
            <div className="space-y-6">
              <PageHeader
                device={device}
                deviceMachine={deviceInfo.machine}
                firmware={deviceInfo.firmware}
                onSettings={onSettings}
                onDisconnect={onDisconnect}
              />
              <hr className="border-border" />
              <div className="flex items-center justify-between">
                <h2 className="text-xl font-semibold">File Browser</h2>
                <Button variant="ghost" size="sm" onClick={() => setShowFileBrowser(false)}>
                  <X className="h-4 w-4 mr-2" />
                  Close
                </Button>
              </div>
              <FileBrowser
                isConnected={connectionStatus === 'connected'}
                suppressSystemFileWarnings={suppressSystemFileWarnings}
                isVisible={showFileBrowser}
              />
            </div>
          </div>
        </div>
      )}

      {showConfigEditor && (
        <div className="fixed inset-0 z-50 bg-background overflow-auto">
          <div className="min-h-screen p-6">
            <div className="space-y-6">
              <PageHeader
                device={device}
                deviceMachine={deviceInfo.machine}
                firmware={deviceInfo.firmware}
                onSettings={onSettings}
                onDisconnect={onDisconnect}
              />

              <hr className="border-border" />

              <div className="flex items-center justify-between">
                <h2 className="text-xl font-semibold">xochitl.conf Editor</h2>
                <Button variant="ghost" size="sm" onClick={() => setShowConfigEditor(false)}>
                  <X className="h-4 w-4 mr-2" />
                  Close
                </Button>
              </div>

              <ConfigEditor isConnected={connectionStatus === 'connected'} theme={resolvedEditorTheme} />
            </div>
          </div>
        </div>
      )}
    </>
  )
}
