import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { ExternalLink, Plus, Trash2, Check, AlertTriangle } from 'lucide-react'

interface PackageInfo {
  name: string
  version: string
  description: string
  upstreamAuthor: string
  categories: string[]
  url: string
  license: string
  devices: string[]
  depends: string[]
  conflicts: string[]
  osMin: string | null
  osMax: string | null
}

interface PackageDetailPanelProps {
  pkg: PackageInfo
  isInstalled: boolean
  installedPackages: Set<string>
  onInstall: () => void
  onUninstall: () => void
  isQueued: boolean
  queueType: 'install' | 'uninstall' | null
  disabled: boolean
  onSelectPackage: (name: string) => void
  firmware?: string
  conflict?: string | null
  isOsCompatible?: boolean
}

const deviceLabels: Record<string, string> = {
  rm1: 'reMarkable 1',
  rm2: 'reMarkable 2',
  rmpp: 'Paper Pro',
  rmppm: 'Paper Pro Move',
}

export function PackageDetailPanel({
  pkg,
  isInstalled,
  installedPackages,
  onInstall,
  onUninstall,
  isQueued,
  queueType,
  disabled,
  onSelectPackage,
  firmware,
  conflict,
  isOsCompatible = true,
}: PackageDetailPanelProps) {
  const formatOsRange = () => {
    if (!pkg.osMin && !pkg.osMax) return null
    if (pkg.osMin && pkg.osMax) {
      return pkg.osMin === pkg.osMax ? pkg.osMin : `${pkg.osMin} – ${pkg.osMax}`
    }
    if (pkg.osMin) return `${pkg.osMin}+`
    return `≤ ${pkg.osMax}`
  }

  const osRange = formatOsRange()

  return (
    <div className="flex flex-col h-full">
      <SheetHeader className="pb-4">
        <SheetTitle className="text-xl pr-8 flex items-center gap-2">
          {pkg.name}
          {isInstalled && <Check className="h-5 w-5 text-green-600" />}
        </SheetTitle>
        <SheetDescription className="mt-1">{pkg.description}</SheetDescription>
      </SheetHeader>

      <div className="flex-1 overflow-y-auto">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <dt className="text-muted-foreground">Version</dt>
          <dd className="font-medium">{pkg.version}</dd>

          {pkg.upstreamAuthor && (
            <>
              <dt className="text-muted-foreground">Author</dt>
              <dd>
                {pkg.url ? (
                  <button
                    onClick={() => window.runtime.BrowserOpenURL(pkg.url)}
                    className="text-primary hover:underline inline-flex items-center gap-1"
                  >
                    {pkg.upstreamAuthor}
                    <ExternalLink className="h-3 w-3" />
                  </button>
                ) : (
                  pkg.upstreamAuthor
                )}
              </dd>
            </>
          )}

          {pkg.categories && pkg.categories.length > 0 && (
            <>
              <dt className="text-muted-foreground">Category</dt>
              <dd className="flex flex-wrap gap-1">
                {pkg.categories.map(cat => <Badge key={cat} variant="outline">{cat}</Badge>)}
              </dd>
            </>
          )}

          {pkg.license && (
            <>
              <dt className="text-muted-foreground">License</dt>
              <dd>{pkg.license}</dd>
            </>
          )}

          {pkg.devices && pkg.devices.length > 0 && (
            <>
              <dt className="text-muted-foreground">Devices</dt>
              <dd className="flex flex-wrap gap-1">
                {pkg.devices.map((device) => (
                  <Badge key={device} variant="secondary" className="text-xs">
                    {deviceLabels[device] || device}
                  </Badge>
                ))}
              </dd>
            </>
          )}

          {osRange && (
            <>
              <dt className="text-muted-foreground">OS Version</dt>
              <dd>{osRange}</dd>
            </>
          )}

          {pkg.depends && pkg.depends.length > 0 && (
            <>
              <dt className="text-muted-foreground col-span-2 mt-2 border-t pt-3">Dependencies</dt>
              <dd className="col-span-2">
                <ul className="space-y-1">
                  {pkg.depends.map((dep) => {
                    const depInstalled = installedPackages.has(dep)
                    return (
                      <li key={dep} className="flex items-center gap-2">
                        <button
                          onClick={() => onSelectPackage(dep)}
                          className="text-primary hover:underline text-left"
                        >
                          {dep}
                        </button>
                        {!depInstalled && !isInstalled && (
                          <span className="text-xs text-muted-foreground">(will be installed)</span>
                        )}
                        {depInstalled && (
                          <Check className="h-3 w-3 text-green-600" />
                        )}
                      </li>
                    )
                  })}
                </ul>
              </dd>
            </>
          )}

          {pkg.conflicts && pkg.conflicts.length > 0 && (
            <>
              <dt className="text-muted-foreground col-span-2 mt-2 border-t pt-3">Conflicts</dt>
              <dd className="col-span-2">
                <ul className="space-y-1">
                  {pkg.conflicts.map((conflict) => {
                    const conflictInstalled = installedPackages.has(conflict)
                    return (
                      <li key={conflict} className="flex items-center gap-2">
                        <button
                          onClick={() => onSelectPackage(conflict)}
                          className="text-primary hover:underline text-left"
                        >
                          {conflict}
                        </button>
                        {conflictInstalled && (
                          <span className="inline-flex items-center gap-1 text-xs text-destructive">
                            <AlertTriangle className="h-3 w-3" />
                            installed
                          </span>
                        )}
                      </li>
                    )
                  })}
                </ul>
              </dd>
            </>
          )}

          {pkg.url && (
            <>
              <dt className="text-muted-foreground col-span-2 mt-2 border-t pt-3">Source</dt>
              <dd className="col-span-2">
                <button
                  onClick={() => window.runtime.BrowserOpenURL(pkg.url)}
                  className="text-primary hover:underline inline-flex items-center gap-1 break-all text-left"
                >
                  {pkg.url}
                  <ExternalLink className="h-3 w-3 flex-shrink-0" />
                </button>
              </dd>
            </>
          )}
        </dl>
      </div>

      <div className="pt-4 mt-4 border-t">
        {isQueued ? (
          <Button variant="outline" className="w-full" disabled>
            <Check className="h-4 w-4 mr-2" />
            Queued for {queueType === 'install' ? 'Installation' : 'Removal'}
          </Button>
        ) : isInstalled ? (
          <Button
            variant="destructive"
            className="w-full"
            onClick={onUninstall}
            disabled={disabled}
          >
            <Trash2 className="h-4 w-4 mr-2" />
            Remove
          </Button>
        ) : !isOsCompatible ? (
          <Button className="w-full" disabled>
            <AlertTriangle className="h-4 w-4 mr-2" />
            Not compatible with {firmware}
          </Button>
        ) : conflict ? (
          <Button className="w-full" disabled>
            <AlertTriangle className="h-4 w-4 mr-2" />
            {conflict}
          </Button>
        ) : (
          <Button className="w-full" onClick={onInstall} disabled={disabled}>
            <Plus className="h-4 w-4 mr-2" />
            Add to Queue
          </Button>
        )}
      </div>
    </div>
  )
}
