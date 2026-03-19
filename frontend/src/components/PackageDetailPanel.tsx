import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { ExternalLink, Plus, Trash2, Check, AlertTriangle, ArrowRight, X } from 'lucide-react'

interface OSConstraint {
  version: string
  operator: '>=' | '<' | '>' | '<=' | '='
}

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
  osConstraints: OSConstraint[] | null
  compatible: boolean
}

interface PackageDetailPanelProps {
  pkg: PackageInfo
  isInstalled: boolean
  installedPackages: Map<string, string>
  installedVersion?: string
  onInstall: () => void
  onUninstall: () => void
  isQueued: boolean
  queueType: 'install' | 'uninstall' | null
  disabled: boolean
  onSelectPackage: (name: string) => void
  allPackages?: PackageInfo[]
  firmware?: string
  conflict?: string | null
  isOsCompatible?: boolean
  viewOnly?: boolean
  showIncompatible?: boolean
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
  installedVersion,
  onInstall,
  onUninstall,
  isQueued,
  queueType,
  disabled,
  onSelectPackage,
  allPackages = [],
  firmware,
  conflict,
  isOsCompatible = true,
  viewOnly = false,
  showIncompatible = false,
}: PackageDetailPanelProps) {
  const formatOsRangeFor = (p: PackageInfo) => {
    if (p.osConstraints && p.osConstraints.length > 0) {
      const minC = p.osConstraints.find(c => c.operator === '>=')
      const maxC = p.osConstraints.find(c => c.operator === '<')
      const exactC = p.osConstraints.find(c => c.operator === '=')

      if (exactC) return exactC.version

      if (minC && maxC) {
        const maxInclusive = (parseFloat(maxC.version) - 0.01).toFixed(2)
        return minC.version === maxInclusive ? minC.version : `${minC.version} – ${maxInclusive}`
      }

      if (minC) return `${minC.version}+`
      if (maxC) {
        const maxInclusive = (parseFloat(maxC.version) - 0.01).toFixed(2)
        return `≤ ${maxInclusive}`
      }
    }

    if (!p.osMin && !p.osMax) return null
    if (p.osMin && p.osMax) {
      const maxInclusive = (parseFloat(p.osMax) - 0.01).toFixed(2)
      return p.osMin === maxInclusive ? p.osMin : `${p.osMin} – ${maxInclusive}`
    }
    if (p.osMin) return `${p.osMin}+`
    if (p.osMax) {
      const maxInclusive = (parseFloat(p.osMax) - 0.01).toFixed(2)
      return `≤ ${maxInclusive}`
    }
    return null
  }

  const osRange = formatOsRangeFor(pkg)

  return (
    <div className="flex flex-col h-full">
      <SheetHeader className="pb-4">
        <SheetTitle className="text-xl pr-8 flex items-center gap-2">
          {pkg.name}
          {showIncompatible && isInstalled ? (
            <X className="h-5 w-5 text-destructive" />
          ) : isInstalled ? (
            <Check className="h-5 w-5 text-green-600" />
          ) : null}
        </SheetTitle>
        <SheetDescription className="mt-1">{pkg.description}</SheetDescription>
      </SheetHeader>

      <div className="flex-1 overflow-y-auto">
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <dt className="text-muted-foreground">Version</dt>
          <dd className="font-medium">
            {viewOnly && installedVersion && installedVersion !== pkg.version ? (
              <span className="flex items-center gap-2">
                <span className="text-muted-foreground">{installedVersion}</span>
                <ArrowRight className="h-3 w-3" />
                <span className="text-green-600">{pkg.version}</span>
              </span>
            ) : (
              isInstalled && installedVersion ? installedVersion : pkg.version
            )}
          </dd>

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
              <dd className={showIncompatible || !pkg.compatible ? 'text-destructive' : ''}>{osRange}</dd>
            </>
          )}

          {pkg.depends && pkg.depends.length > 0 && (
            <>
              <dt className="text-muted-foreground col-span-2 mt-2 border-t pt-3">Dependencies</dt>
              <dd className="col-span-2">
                <ul className="space-y-1">
                  {pkg.depends.map((dep) => {
                    const depInstalled = installedPackages.has(dep)
                    const depPkg = allPackages.find(p => p.name === dep)
                    const depOsRange = depPkg ? formatOsRangeFor(depPkg) : null
                    const depIncompatible = depPkg && !depPkg.compatible
                    return (
                      <li key={dep} className="flex items-center gap-2">
                        <button
                          onClick={() => onSelectPackage(dep)}
                          className={`hover:underline text-left ${depIncompatible ? 'text-destructive' : 'text-primary'}`}
                        >
                          {dep}
                        </button>
                        {depOsRange && (
                          <span className={`text-xs ${depIncompatible ? 'text-destructive' : 'text-muted-foreground'}`}>
                            (OS {depOsRange})
                          </span>
                        )}
                        {!depInstalled && !isInstalled && !depIncompatible && (
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
              <dt className="text-muted-foreground col-span-2 mt-2 border-t pt-3">Project</dt>
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

      {(!viewOnly || (showIncompatible && isInstalled)) && (
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
      )}
    </div>
  )
}
