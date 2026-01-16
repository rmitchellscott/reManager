import { AlertTriangle, Loader2, Trash2, Check, X } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'

interface UpgradeChecklistProps {
  storedOsVersion: string
  currentOsVersion: string
  compatiblePackages: string[]
  incompatiblePackages: string[]
  loading?: boolean
  fetchFailed?: boolean
  uninstallQueue: Set<string>
  onAddToUninstallQueue: (pkg: string) => void
  onRemoveFromUninstallQueue: (pkg: string) => void
  onRunUpgrade: () => void
  autoUpdatesEnabled?: boolean
  hashtabMismatch?: boolean
  onGoToMaintenance: () => void
}

export function UpgradeChecklist({
  storedOsVersion,
  currentOsVersion,
  compatiblePackages,
  incompatiblePackages,
  loading = false,
  fetchFailed = false,
  uninstallQueue,
  onAddToUninstallQueue,
  onRemoveFromUninstallQueue,
  onRunUpgrade,
  autoUpdatesEnabled = false,
  hashtabMismatch = false,
  onGoToMaintenance,
}: UpgradeChecklistProps) {
  const canUpgrade = incompatiblePackages.length === 0

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <AlertTriangle className="h-5 w-5 text-yellow-500" />
          <CardTitle>OS Change Detected</CardTitle>
        </div>
        <CardDescription>
          Your reMarkable OS has changed from {storedOsVersion} to {currentOsVersion}.
          Review package compatibility and complete the upgrade process.
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-6">
        <div className="bg-muted p-4 rounded-md flex items-center justify-between">
          <ul className="text-sm space-y-1">
            {autoUpdatesEnabled && (
              <li>• Auto-updates are enabled</li>
            )}
            <li>• Run Vellum Reenable to restore system modifications</li>
            {hashtabMismatch && (
              <li>• Rebuild hashtable after OS upgrade</li>
            )}
          </ul>
          <Button variant="link" size="sm" className="h-auto p-0" onClick={onGoToMaintenance}>
            Go to Maintenance →
          </Button>
        </div>

        {fetchFailed && (
          <div className="text-sm text-muted-foreground bg-muted p-3 rounded-md">
            Could not verify package compatibility (offline?). Showing all installed packages.
          </div>
        )}

        <Accordion type="multiple" defaultValue={["incompatible"]}>
          {compatiblePackages.length > 0 && (
            <AccordionItem value="compatible" className="border-none">
              <AccordionTrigger className="py-2 text-sm font-medium hover:no-underline">
                Compatible Packages ({compatiblePackages.length})
              </AccordionTrigger>
              <AccordionContent>
                <div className="border rounded-md divide-y">
                  {compatiblePackages.map(pkg => (
                    <div key={pkg} className="px-3 py-2 text-sm flex items-center gap-2">
                      <Check className="h-4 w-4 text-green-500" />
                      <span>{pkg}</span>
                    </div>
                  ))}
                </div>
              </AccordionContent>
            </AccordionItem>
          )}

          {incompatiblePackages.length > 0 && (
            <AccordionItem value="incompatible" className="border-none">
              <AccordionTrigger className="py-2 text-sm font-medium hover:no-underline">
                Incompatible Packages ({incompatiblePackages.length})
              </AccordionTrigger>
              <AccordionContent>
                <div className="border rounded-md divide-y overflow-hidden">
                  {incompatiblePackages.map((pkg, index) => {
                    const isQueued = uninstallQueue.has(pkg)
                    const prevQueued = index > 0 && uninstallQueue.has(incompatiblePackages[index - 1])
                    const nextQueued = index < incompatiblePackages.length - 1 && uninstallQueue.has(incompatiblePackages[index + 1])
                    return (
                      <div
                        key={pkg}
                        className={`px-3 py-2 text-sm flex items-center justify-between transition-colors ${
                          isQueued
                            ? `border-l-4 border-destructive ${!prevQueued ? 'border-t border-t-destructive' : ''} ${!nextQueued ? 'border-b border-b-destructive' : ''}`
                            : ''
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <X className="h-4 w-4 text-destructive" />
                          <span>{pkg}</span>
                        </div>
                        {isQueued ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => onRemoveFromUninstallQueue(pkg)}
                          >
                            <Check className="h-4 w-4 mr-1" />
                            Queued
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => onAddToUninstallQueue(pkg)}
                          >
                            <Trash2 className="h-4 w-4 mr-1" />
                            Uninstall
                          </Button>
                        )}
                      </div>
                    )
                  })}
                </div>
              </AccordionContent>
            </AccordionItem>
          )}
        </Accordion>

        <div className="space-y-3 pt-4">
          {!canUpgrade && (
            <p className="text-sm text-muted-foreground">
              Either wait for incompatible packages to be updated, or uninstall them before upgrading packages.
            </p>
          )}
          <Button
            className="w-full"
            onClick={onRunUpgrade}
            disabled={!canUpgrade || loading}
          >
            {loading ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Running...
              </>
            ) : (
              'Upgrade Packages'
            )}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
