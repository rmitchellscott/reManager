import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { ExternalLink, Plus, Trash2, Check, AlertTriangle, ArrowLeft, ArrowRight, X, Heart, BookOpen } from 'lucide-react'
import { StatusBadge } from '@/components/StatusBadge'
import { PackageInfo } from '@/lib/types'
import { formatOsRange } from '@/lib/format'

function conflictName(entry: string): string {
  const match = entry.match(/^(.+?)(>=|<=|=|>|<)/)
  return match ? match[1] : entry
}

interface PackageDetailPanelProps {
  pkg: PackageInfo
  isInstalled: boolean
  detailsUnavailable?: boolean
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
  onViewReadme?: (url: string) => void
  onBack?: () => void
}

const deviceLabels: Record<string, string> = {
  rm1: 'reMarkable 1',
  rm2: 'reMarkable 2',
  rmpp: 'Paper Pro',
  rmppmove: 'Paper Pro Move',
  rmppure: 'Paper Pure',
}

export function PackageDetailPanel({
  pkg,
  isInstalled,
  detailsUnavailable = false,
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
  onViewReadme,
  onBack,
}: PackageDetailPanelProps) {
  const osRange = formatOsRange(pkg)

  return (
    <div className="flex flex-col h-full">
      <SheetHeader className="pb-4">
        <SheetTitle className="text-xl pr-8 flex items-center gap-2">
          {onBack && (
            <button onClick={onBack} className="text-muted-foreground hover:text-foreground transition-colors">
              <ArrowLeft className="h-5 w-5" />
            </button>
          )}
          {pkg.name}
          {showIncompatible && isInstalled ? (
            <X className="h-5 w-5 text-destructive" />
          ) : isInstalled ? (
            <Check className="h-5 w-5 text-green-600" />
          ) : null}
          <StatusBadge status={pkg.status} />
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
                    const depPkg = allPackages.find(p => p.name === dep)
                    const providers = depPkg ? [] : allPackages.filter(p => (p.provides || []).includes(dep))
                    if (providers.length > 0) {
                      const satisfiedBy = providers.find(p => installedPackages.has(p.name))
                      return (
                        <li key={dep} className="flex items-center gap-2">
                          <span>{dep}</span>
                          <span className="text-xs text-muted-foreground">
                            ({satisfiedBy ? satisfiedBy.name : providers.map(p => p.name).join(' or ')})
                          </span>
                          {satisfiedBy && <Check className="h-3 w-3 text-green-600" />}
                        </li>
                      )
                    }
                    const depInstalled = installedPackages.has(dep)
                    const depOsRange = depPkg ? formatOsRange(depPkg) : null
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
                  {pkg.conflicts.map((entry) => {
                    const name = conflictName(entry)
                    const constraint = entry.slice(name.length)
                    return (
                      <li key={entry} className="flex items-center gap-2">
                        <button
                          onClick={() => onSelectPackage(name)}
                          className="text-primary hover:underline text-left"
                        >
                          {name}
                        </button>
                        {constraint && (
                          <span className="text-xs text-muted-foreground font-mono">{constraint}</span>
                        )}
                        {installedPackages.has(name) && (
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

          {(pkg.donateUrl || pkg.readmeUrl) && (
            <dd className="col-span-2 mt-2 border-t pt-3 flex flex-wrap gap-2">
              {pkg.readmeUrl && onViewReadme && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onViewReadme(pkg.readmeUrl!)}
                  className="inline-flex items-center gap-1"
                >
                  <BookOpen className="h-3 w-3" />
                  View Readme
                </Button>
              )}
              {pkg.donateUrl && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => window.runtime.BrowserOpenURL(pkg.donateUrl!)}
                  className="inline-flex items-center gap-1"
                >
                  <Heart className="h-3 w-3" />
                  Donate
                  <ExternalLink className="h-3 w-3" />
                </Button>
              )}
            </dd>
          )}
        </dl>

        {detailsUnavailable && (
          <p className="text-sm text-muted-foreground mt-4 border-t pt-3">
            Detailed package information requires an internet connection.
          </p>
        )}
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
              {pkg.incompatibleReason === 'device' ? 'Does not support your reMarkable model' : `Not compatible with ${firmware}`}
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
