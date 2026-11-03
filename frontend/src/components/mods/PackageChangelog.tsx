import { CircleNotch } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Badge } from '@/components/ui/badge'
import { MarkdownLink, resolveMarkdownUrl } from '@/components/MarkdownLink'
import { getCachedContent, setCachedContent } from '@/lib/contentCache'
import { formatOsRange } from '@/lib/format'
import { handleError } from '@/lib/errorMessages'
import { compareVersions } from '@/lib/version'
import { PackageRelease } from '@/lib/types'
import { cn } from '@/lib/utils'
import { PackagesService } from '@bindings/reManager'

interface PackageChangelogProps {
  packageName: string
  installedVersion?: string
  latestVersion: string
}

// pkgrel (-rN) is a repackage rather than a code change, so it is stripped from the shown version.
function pkgver(version: string): string {
  return version.replace(/-r\d+$/, '')
}

function formatDate(released: string): string {
  if (!released) return ''
  const date = new Date(released)
  return isNaN(date.getTime()) ? released : date.toLocaleDateString()
}

function reachLabel(release: PackageRelease): string {
  const range = formatOsRange(release)
  return range ? `Needs OS ${range}` : 'Not for this reMarkable'
}

function derivedChanges(release: PackageRelease, previous?: PackageRelease): string[] {
  if (!previous) return ['First release in the catalog.']

  const changes: string[] = []
  const before = formatOsRange(previous)
  const after = formatOsRange(release)
  if (before !== after) {
    changes.push(after
      ? `OS support ${before ? `changed from ${before} to ${after}` : `set to ${after}`}.`
      : 'Now supports any OS version.')
  }

  const addedDeps = (release.depends ?? []).filter(dep => !(previous.depends ?? []).includes(dep))
  const droppedDeps = (previous.depends ?? []).filter(dep => !(release.depends ?? []).includes(dep))
  if (addedDeps.length) changes.push(`Now requires ${addedDeps.join(', ')}.`)
  if (droppedDeps.length) changes.push(`No longer requires ${droppedDeps.join(', ')}.`)

  const addedArch = (release.arch ?? []).filter(arch => !(previous.arch ?? []).includes(arch))
  if (addedArch.length) changes.push(`Added ${addedArch.join(', ')} builds.`)

  const addedDevices = (release.devices ?? []).filter(device => !(previous.devices ?? []).includes(device))
  if (addedDevices.length) changes.push('Added support for another reMarkable model.')

  if (!changes.length) changes.push('No catalog details changed in this release.')
  return changes
}


// Headings vary by project: "## 1.2.3", "## [1.2.3] - 2026-01-01" and "## v1.2.3" all appear in the catalog.
const documentProse = 'text-sm [&_h1]:text-sm [&_h1]:font-semibold [&_h1]:mt-4 [&_h1]:mb-1.5 [&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mt-4 [&_h2]:mb-1.5 [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:mt-3 [&_h3]:mb-1 [&_p]:mb-2.5 [&_ul]:list-disc [&_ul]:pl-5 [&_ul]:mb-2.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_ol]:mb-2.5 [&_li]:mb-0.5 [&_code]:font-mono [&_code]:text-[13px] [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded [&_a]:underline [&_img]:max-w-full [&_img]:h-auto [&_table]:block [&_table]:overflow-x-auto [&_table]:max-w-full [&_pre]:overflow-x-auto break-words'

const notesProse = 'text-sm [&_h1]:text-xs [&_h1]:font-semibold [&_h1]:uppercase [&_h1]:tracking-[0.6px] [&_h1]:text-muted-foreground [&_h1]:mt-3 [&_h1]:mb-1 [&_h2]:text-xs [&_h2]:font-semibold [&_h2]:uppercase [&_h2]:tracking-[0.6px] [&_h2]:text-muted-foreground [&_h2]:mt-3 [&_h2]:mb-1 [&_h3]:text-xs [&_h3]:font-semibold [&_h3]:uppercase [&_h3]:tracking-[0.6px] [&_h3]:text-muted-foreground [&_h3]:mt-3 [&_h3]:mb-1 [&_p]:text-xs [&_p]:text-muted-foreground [&_p]:mb-2 [&_ul]:list-disc [&_ul]:pl-4 [&_ul]:mb-2 [&_ul]:text-xs [&_ul]:text-muted-foreground [&_ol]:list-decimal [&_ol]:pl-4 [&_ol]:mb-2 [&_ol]:text-xs [&_li]:mb-0.5 [&_code]:font-mono [&_code]:text-[11px] [&_code]:bg-muted [&_code]:px-1 [&_code]:rounded [&_a]:underline [&_img]:max-w-full [&_img]:h-auto [&_table]:block [&_table]:overflow-x-auto [&_table]:max-w-full [&_pre]:overflow-x-auto break-words'

const markdownComponents: Components = {
  a: ({ href, children }) => <MarkdownLink href={href}>{children}</MarkdownLink>,
}

function sectionsByVersion(markdown: string): Map<string, string> {
  const sections = new Map<string, string>()
  const lines = markdown.split('\n')
  let current: string | null = null
  let body: string[] = []

  const commit = () => {
    if (current && body.length) sections.set(current, body.join('\n').trim())
  }

  for (const line of lines) {
    const heading = line.match(/^(#{1,6})\s+(.*)$/)
    const version = heading ? heading[2].match(/(\d+(?:\.\d+)+)/) : null
    if (heading && version) {
      commit()
      current = version[1]
      body = []
      continue
    }
    if (current) body.push(line)
  }
  commit()
  return sections
}

export function PackageChangelog({ packageName, installedVersion, latestVersion }: PackageChangelogProps) {
  const [releases, setReleases] = useState<PackageRelease[] | null>(null)
  const [notes, setNotes] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let canceled = false
    setLoading(true)
    setError(null)
    setNotes(null)

    PackagesService.GetPackageReleases(packageName)
      .then(async (loaded) => {
        if (canceled) return
        setReleases(loaded || [])

        // Each release's changelogUrl points at the file as it stood then, so the newest release's file is the whole history.
        const url = loaded?.[0]?.changelogUrl
        if (!url) return
        const cached = getCachedContent(url)
        if (cached !== undefined) {
          if (!canceled) setNotes(cached)
          return
        }
        try {
          const content = await PackagesService.GetReadmeContent(url)
          setCachedContent(url, content)
          if (!canceled) setNotes(content)
        } catch {
          // A missing changelog file falls back to derived history rather than failing the tab.
        }
      })
      .catch((err) => { if (!canceled) setError(handleError(err, 'Could not load release history.')) })
      .finally(() => { if (!canceled) setLoading(false) })

    return () => { canceled = true }
  }, [packageName])

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground py-6">
        <CircleNotch className="h-4 w-4 animate-spin" />
        Loading release history
      </div>
    )
  }

  if (error || !releases || releases.length === 0) {
    return <p className="text-sm text-muted-foreground py-6">No release history is available for this mod.</p>
  }

  const changelogUrl = releases[0].changelogUrl ?? ''
  const resolveChangelogUrl = (src: string) => resolveMarkdownUrl(src, changelogUrl)
  const sections = notes ? sectionsByVersion(notes) : new Map<string, string>()
  const alignsToReleases = sections.size > 0

  // The index prunes old versions, so it is not the full history; the rail is the union of index rows and changelog sections.
  const byVersion = new Map<string, { version: string; released: string; release?: PackageRelease }>()
  for (const release of releases) {
    byVersion.set(pkgver(release.version), { version: pkgver(release.version), released: release.released, release })
  }
  for (const version of sections.keys()) {
    if (!byVersion.has(version)) byVersion.set(version, { version, released: '' })
  }
  const railRows = [...byVersion.values()].sort((a, b) => compareVersions(b.version, a.version))

  const latestReachable = railRows.find(row => !row.release || row.release.compatible)

  const reachable = releases.filter(r => r.compatible)
  const installedIndex = installedVersion ? reachable.findIndex(r => pkgver(r.version) === pkgver(installedVersion)) : -1
  const behind = installedIndex > 0 ? installedIndex : 0

  return (
    <div>
      {behind > 0 && (
        <div className="mb-4 rounded-lg border px-3 py-2.5">
          <p className="text-sm font-semibold">
            {behind} release{behind > 1 ? 's' : ''} since your version
          </p>
          <p className="text-xs text-muted-foreground mt-0.5">
            You have {pkgver(installedVersion!)}. Latest is {pkgver(latestVersion)}.
          </p>
        </div>
      )}

      {notes && !alignsToReleases && (
        <div className={documentProse}>
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents} urlTransform={resolveChangelogUrl}>{notes}</ReactMarkdown>
        </div>
      )}

      {!notes && (
        <p className="text-xs text-muted-foreground mb-4">
          This mod does not publish release notes. Here is what changed in the catalog between releases.
        </p>
      )}

      {notes && !alignsToReleases && (
        <div className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mt-6 mb-3">
          Release history
        </div>
      )}

      <div>
        {railRows.map(row => {
          const release = row.release
          const previous = release ? releases[releases.indexOf(release) + 1] : undefined
          const isInstalled = !!installedVersion && row.version === pkgver(installedVersion)
          const isLatest = row === latestReachable
          const outOfReach = !!release && !release.compatible
          const isUpgradeTarget = isLatest && !!installedVersion && row.version !== pkgver(installedVersion)
          return (
            <div
              key={row.version}
              className={cn(
                'relative pl-[22px] pb-[18px] last:pb-0',
                'before:absolute before:left-1 before:top-4 before:bottom-0 before:w-px before:bg-border last:before:hidden',
                'after:absolute after:left-0 after:top-1.5 after:h-[9px] after:w-[9px] after:rounded-full after:border after:border-border after:bg-background',
                isInstalled && 'after:bg-muted-foreground after:border-muted-foreground',
                isLatest && !isInstalled && 'after:border-foreground',
              )}
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className={cn(
                  'text-sm font-semibold',
                  outOfReach && 'text-muted-foreground',
                  isUpgradeTarget && 'text-[var(--info-text)]',
                )}>{row.version}</span>
                {isLatest && <Badge variant="outline">Latest</Badge>}
                {isInstalled && <Badge variant="outline">Installed</Badge>}
              </div>
              {row.released && <p className="text-xs text-muted-foreground mb-1.5">{formatDate(row.released)}</p>}
              {outOfReach && (
                <p className="text-xs text-muted-foreground mb-1.5">
                  {reachLabel(release!)}
                </p>
              )}
              {alignsToReleases ? (
                sections.has(row.version) ? (
                  <div className={notesProse}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents} urlTransform={resolveChangelogUrl}>{sections.get(row.version)!}</ReactMarkdown>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">No notes for this release.</p>
                )
              ) : !notes && release ? (
                <ul className="text-xs text-muted-foreground list-disc pl-4 space-y-0.5">
                  {derivedChanges(release, previous).map(change => <li key={change}>{change}</li>)}
                </ul>
              ) : null}
            </div>
          )
        })}
      </div>
    </div>
  )
}
