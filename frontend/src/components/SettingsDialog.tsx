import { CaretLeft, CaretRight, CheckCircle, CircleNotch, Trash, X } from '@phosphor-icons/react'
import { useState, useEffect, useRef } from 'react'
import { handleError } from '@/lib/errorMessages'
import { toast } from 'sonner'
import { Dialog, DialogCloseButton, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useMediaQuery, NARROW_VIEWPORT } from '@/hooks/useMediaQuery'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { useBackHandler } from '@/hooks/useBackHandler'
import { panelAction, PanelGroup, PanelRow, SectionLabel } from '@/components/Panel'
import { pageTitle } from '@/components/PageHeader'
import { SupportHome, type SupportScreen } from '@/components/SupportHome'
import { SSHKeyManager, useSSHKeyDrafts } from '@/components/SSHKeyManager'
import { SyncKey, type ManagedKey, type PlatformCapabilities, type SettingsInfo } from '@/lib/types'
import { cn } from '@/lib/utils'
import { ConnectionService, SupportService, SystemService } from '@bindings/reManager'
import { onSyncedChange } from '@/lib/syncEvents'

interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings: SettingsInfo
  onUpdate: (patch: Partial<SettingsInfo>) => void
  onOpenSupport: (screen: SupportScreen) => void
  supportPageOpen: boolean
  connectedDeviceId: string
  appVersion: string
  capabilities: PlatformCapabilities
  initialSection?: SettingsSection
}

export type SettingsSection = 'connection' | 'appearance' | 'prompts' | 'storage' | 'help'

type SectionId = SettingsSection

interface SectionDefinition {
  id: SectionId
  label: string
}

const SECTIONS: SectionDefinition[] = [
  { id: 'connection', label: 'Connection' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'prompts', label: 'Prompts & warnings' },
  { id: 'storage', label: 'Storage' },
  { id: 'help', label: 'Get help' },
]

const THEME_CHOICES = [
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
  { value: 'system', label: 'System' },
]

const MATCH_CHOICES = [
  { value: 'match', label: 'Match reManager' },
  { value: 'dark', label: 'Dark' },
  { value: 'light', label: 'Light' },
]

const switchHitTarget = 'touch:hit-target'

export function SettingsDialog({
  open,
  onOpenChange,
  settings,
  onUpdate,
  onOpenSupport,
  supportPageOpen,
  connectedDeviceId,
  appVersion,
  capabilities,
  initialSection,
}: SettingsDialogProps) {
  const narrow = useMediaQuery(NARROW_VIEWPORT)

  const [section, setSection] = useState<SectionId | null>(null)
  const [confirmDeleteLogs, setConfirmDeleteLogs] = useState(false)
  const [confirmClearCache, setConfirmClearCache] = useState(false)
  const [clearingCache, setClearingCache] = useState(false)
  const [cachedDevices, setCachedDevices] = useState(0)
  const [managedKeys, setManagedKeys] = useState<ManagedKey[]>([])
  const keyDrafts = useSSHKeyDrafts()
  const [deletingLogs, setDeletingLogs] = useState(false)
  const [enteringBack, setEnteringBack] = useState(false)

  const [agentSocketMode, setAgentSocketMode] = useState<'auto' | 'custom'>(settings.sshAgentSocketPath ? 'custom' : 'auto')
  const [agentPathDraft, setAgentPathDraft] = useState(settings.sshAgentSocketPath)

  const headingRef = useRef<HTMLHeadingElement>(null)
  const navRefs = useRef<Partial<Record<SectionId, HTMLButtonElement | null>>>({})
  const lastSection = useRef<SectionId | null>(null)

  // Read via ref so the open-reset effect does not list settings as a dep and throw an open pane back to the list on every commit.
  const latestSettings = useRef(settings)
  latestSettings.current = settings

  const latestInitialSection = useRef(initialSection)
  latestInitialSection.current = initialSection

  useEffect(() => {
    if (!open || !capabilities.managedKeys) return
    const load = () => ConnectionService.GetManagedKeys().then(keys => setManagedKeys(keys ?? [])).catch(() => setManagedKeys([]))
    load()
    return onSyncedChange(SyncKey.KeysSyncKey, load)
  }, [open, capabilities.managedKeys])

  useEffect(() => {
    if (!open) return
    setSection(latestInitialSection.current ?? null)
    setConfirmDeleteLogs(false)
    setEnteringBack(false)
    lastSection.current = latestInitialSection.current ?? null
    setAgentSocketMode(latestSettings.current.sshAgentSocketPath ? 'custom' : 'auto')
    setAgentPathDraft(latestSettings.current.sshAgentSocketPath)
  }, [open])

  const activeSection: SectionId = section ?? 'connection'
  const activeDefinition = SECTIONS.find(s => s.id === activeSection) as SectionDefinition

  useEffect(() => {
    if (!open || !narrow) return
    if (section !== null) {
      headingRef.current?.focus()
    } else if (lastSection.current) {
      navRefs.current[lastSection.current]?.focus()
    }
  }, [open, narrow, section])

  const openSection = (id: SectionId) => {
    lastSection.current = id
    setEnteringBack(false)
    setSection(id)
  }

  const backToList = () => {
    setEnteringBack(true)
    setSection(null)
  }

  const { ref: paneRef, swipeHandlers: paneSwipe, touchAction } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: backToList,
    enabled: narrow && section !== null,
  })

  const { ref: shellRef, swipeHandlers: shellSwipe } = useSwipeToDismiss({
    direction: 'right',
    onDismiss: () => onOpenChange(false),
    enabled: narrow && section === null,
  })

  useBackHandler(section !== null ? backToList : () => onOpenChange(false), open && narrow)

  useEffect(() => {
    if (!open) return
    ConnectionService.CachedDeviceCount()
      .then(setCachedDevices)
      .catch(() => setCachedDevices(0))
  }, [open, clearingCache])

  const handleClearCache = async () => {
    setClearingCache(true)
    try {
      await ConnectionService.ClearDeviceInfoCache()
      toast('Cached device information cleared', { icon: <CheckCircle className="h-4 w-4" /> })
      setConfirmClearCache(false)
    } catch (err) {
      toast(handleError(err, 'Could not clear cache.'))
    } finally {
      setClearingCache(false)
    }
  }

  const handleDeleteLogs = async () => {
    setDeletingLogs(true)
    try {
      await SupportService.DeleteAllLogs()
      toast('All command logs deleted', { icon: <CheckCircle className="h-4 w-4" /> })
      setConfirmDeleteLogs(false)
    } catch (err) {
      toast(handleError(err, 'Could not delete logs.'))
    } finally {
      setDeletingLogs(false)
    }
  }

  const commitAgentPath = () => {
    if (agentPathDraft !== settings.sshAgentSocketPath) onUpdate({ sshAgentSocketPath: agentPathDraft })
  }

  const agentPathField = (
    <div>
      <label htmlFor="agent-path" className="text-xs font-medium text-muted-foreground">
        Socket path
      </label>
      <div className="mt-1.5 flex gap-2">
        <Input
          id="agent-path"
          value={agentPathDraft}
          onChange={(e) => setAgentPathDraft(e.target.value)}
          onBlur={commitAgentPath}
          onKeyDown={(e) => { if (e.key === 'Enter') commitAgentPath() }}
          placeholder="/path/to/agent.sock"
          className="flex-1"
        />
        <Button
          variant="outline"
          type="button"
          className="shrink-0 touch:hit-target"
          onClick={async () => {
            const path = await ConnectionService.SelectKeyFile()
            if (path) {
              setAgentPathDraft(path)
              onUpdate({ sshAgentSocketPath: path })
            }
          }}
        >
          Browse
        </Button>
      </div>
    </div>
  )

  const renderSection = () => {
    switch (activeSection) {
      case 'connection':
        return (
          <div className="space-y-5">
          <PanelGroup controlColumn divided={false}>
            <PanelRow
              label="Download through reManager"
              htmlFor="proxy-mode"
              detail="Useful if your reMarkable has limited internet connectivity."
            >
              <Switch id="proxy-mode" className={switchHitTarget} checked={settings.proxyMode} onCheckedChange={(v) => onUpdate({ proxyMode: v })} />
            </PanelRow>
            <PanelRow
              label="Keep your reMarkable awake"
              htmlFor="prevent-sleep"
              detail="Simulates periodic input while connected."
            >
              <Switch id="prevent-sleep" className={switchHitTarget} checked={settings.preventSleep} onCheckedChange={(v) => onUpdate({ preventSleep: v })} />
            </PanelRow>
            {capabilities.sshAgent && (
            <PanelRow
              label="SSH agent"
              htmlFor="agent-mode"
              detail={agentSocketMode === 'auto'
                ? 'Uses the SSH_AUTH_SOCK environment variable.'
                : 'Point reManager at a specific agent socket.'}
              expansion={agentSocketMode === 'custom' ? agentPathField : undefined}
            >
              <Select
                value={agentSocketMode}
                onValueChange={(value) => {
                  const mode = value as 'auto' | 'custom'
                  setAgentSocketMode(mode)
                  if (mode === 'auto') {
                    setAgentPathDraft('')
                    onUpdate({ sshAgentSocketPath: '' })
                  }
                }}
              >
                <SelectTrigger id="agent-mode" className="w-40" aria-describedby="agent-mode-detail">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">Auto-detect</SelectItem>
                  <SelectItem value="custom">Custom path</SelectItem>
                </SelectContent>
              </Select>
            </PanelRow>
            )}
          </PanelGroup>

          {capabilities.managedKeys && (
            <div className="border-t pt-5">
              <SectionLabel>SSH keys</SectionLabel>
              <SSHKeyManager keys={managedKeys} onKeysChange={setManagedKeys} canChooseFile={capabilities.localKeyFiles} drafts={keyDrafts} />
            </div>
          )}
          </div>
        )

      case 'appearance':
        return (
          <PanelGroup controlColumn divided={false}>
            <PanelRow label="Theme" htmlFor="theme-select" detail="Applies to reManager itself.">
              <Select value={settings.theme} onValueChange={(v) => onUpdate({ theme: v })}>
                <SelectTrigger id="theme-select" className="w-40"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {THEME_CHOICES.map(c => <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </PanelRow>
            <PanelRow label="Terminal theme" htmlFor="terminal-theme-select" detail="Applies to the terminal and command output.">
              <Select value={settings.terminalTheme} onValueChange={(v) => onUpdate({ terminalTheme: v })}>
                <SelectTrigger id="terminal-theme-select" className="w-40"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {MATCH_CHOICES.map(c => <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </PanelRow>
            <PanelRow label="Editor theme" htmlFor="editor-theme-select" detail="Applies to the file editor.">
              <Select value={settings.editorTheme} onValueChange={(v) => onUpdate({ editorTheme: v })}>
                <SelectTrigger id="editor-theme-select" className="w-40"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {MATCH_CHOICES.map(c => <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>)}
                </SelectContent>
              </Select>
            </PanelRow>
          </PanelGroup>
        )

      case 'prompts':
        return (
          <PanelGroup controlColumn divided={false}>
            {/* The stored setting suppresses warnings, so the switch shows its inverse. */}
            <PanelRow
              label="Warn before changing system files"
              htmlFor="warn-sys-files"
              detail="Only turn this off if you are comfortable with Linux systems."
            >
              <Switch
                id="warn-sys-files"
                className={switchHitTarget}
                checked={!settings.suppressSystemFileWarnings}
                onCheckedChange={(v) => onUpdate({ suppressSystemFileWarnings: !v })}
              />
            </PanelRow>
            {capabilities.updateCheck && (
              <PanelRow
                label="Notify me about reManager updates"
                htmlFor="check-updates"
                detail="Checks for a new release when reManager starts."
              >
                <Switch id="check-updates" className={switchHitTarget} checked={settings.checkForUpdates} onCheckedChange={(v) => onUpdate({ checkForUpdates: v })} />
              </PanelRow>
            )}
          </PanelGroup>
        )

      case 'storage':
        return (
          <div className="space-y-5">
            <PanelGroup controlColumn>
              <PanelRow
                label="Cached device information"
                detail="Shows what each reMarkable has waiting before you connect."

              >
                <Button
                  className={panelAction}
                  variant="outline"
                  disabled={cachedDevices === 0}
                  onClick={() => setConfirmClearCache(true)}
                >
                  Clear
                </Button>
              </PanelRow>
              <PanelRow label="Command logs" detail="Cleaned up automatically after 30 days.">
                <Button className={panelAction} variant="outline" onClick={() => setConfirmDeleteLogs(true)}>
                  Delete
                </Button>
              </PanelRow>
            </PanelGroup>
          </div>
        )
      case 'help':
        return (
          <div className="space-y-5">
            <SupportHome
              active={activeSection === 'help'}
              supportPageOpen={supportPageOpen}
              connectedDeviceId={connectedDeviceId}
              supportUpload={capabilities.supportUpload}
              onOpen={onOpenSupport}
            />
          </div>
        )
    }
  }

  const clearCacheDialog = (
    <Dialog open={confirmClearCache} onOpenChange={(open) => !open && setConfirmClearCache(false)}>
      <DialogContent>
        <DialogHeader severity="destructive" icon={Trash}>
          <DialogTitle>Clear cached device information?</DialogTitle>
          <DialogDescription>
            reManager forgets what each reMarkable has installed until the next time you connect
            to it.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setConfirmClearCache(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={handleClearCache} disabled={clearingCache}>
            {clearingCache && <CircleNotch className="mr-1.5 h-4 w-4 animate-spin" />}
            Clear
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  const deleteLogsDialog = (
    <Dialog open={confirmDeleteLogs} onOpenChange={(open) => !open && setConfirmDeleteLogs(false)}>
      <DialogContent>
        <DialogHeader severity="destructive" icon={Trash}>
          <DialogTitle>Delete all command logs?</DialogTitle>
          <DialogDescription>
            Support bundles are built from these logs, so a bundle made afterward will carry
            nothing from before now.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setConfirmDeleteLogs(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={handleDeleteLogs} disabled={deletingLogs}>
            {deletingLogs && <CircleNotch className="mr-1.5 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )

  const showList = narrow && section === null

  useEffect(() => {
    if (!open || !narrow) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onOpenChange(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, narrow, onOpenChange])

  const closeButton = (
    <Button
      variant="ghost"
      size="xs"
      className="-mr-2 touch:hit-target"
      onClick={() => onOpenChange(false)}
      aria-label="Close settings"
    >
      <X className="h-4 w-4" />
    </Button>
  )

  const navRowClass = 'flex w-full items-center gap-3 text-left text-sm font-medium transition-colors touch:hit-target'

  const navList = narrow ? (
    <nav className="-ml-gutter-left -mr-gutter-right" aria-label="Settings sections">
      {SECTIONS.map(s => (
        <button
          key={s.id}
          ref={(el) => { navRefs.current[s.id] = el }}
          type="button"
          onClick={() => openSection(s.id)}
          className={cn(navRowClass, 'justify-between pl-gutter-left pr-gutter-right py-3 hover:bg-accent active:bg-accent')}
        >
          <span className="truncate">{s.label}</span>
          <CaretRight className="h-4 w-4 shrink-0 opacity-50" aria-hidden />
        </button>
      ))}
    </nav>
  ) : (
    <nav className="space-y-0.5" aria-label="Settings sections">
      {SECTIONS.map(s => (
        <button
          key={s.id}
          type="button"
          onClick={() => setSection(s.id)}
          aria-current={activeSection === s.id ? 'page' : undefined}
          className={cn(
            navRowClass,
            'rounded-md px-3 py-2',
            // --accent equals --muted here, so an accent fill on the sidebar would vanish; the active row takes the content surface instead.
            activeSection === s.id
              ? 'bg-card text-foreground shadow-sm'
              : 'text-muted-foreground hover:bg-card/60 hover:text-foreground',
          )}
        >
          {s.label}
        </button>
      ))}
    </nav>
  )

  const footer = (stacked: boolean) => (
    <div className={cn(
      'flex shrink-0 text-xs text-muted-foreground',
      stacked ? 'flex-col items-start gap-0.5' : 'items-center justify-center gap-2',
    )}>
      <span>reManager {appVersion}</span>
      {!stacked && <span aria-hidden>·</span>}
      <button
        onClick={() => SystemService.OpenExternalURL('https://github.com/rmitchellscott/remanager')}
        className="hover:text-foreground hover:underline"
      >
        GitHub
      </button>
      {!stacked && <span aria-hidden>·</span>}
      <button
        onClick={() => SystemService.OpenExternalURL('https://remanager.io/privacy')}
        className="hover:text-foreground hover:underline"
      >
        Privacy policy
      </button>
    </div>
  )

  if (narrow) {
    if (!open) return null
    return (
      <div
        ref={shellRef}
        {...shellSwipe}
        className="screen-edge fixed inset-0 z-50 flex touch-pan-y flex-col bg-background duration-150 animate-in fade-in-0"
      >
        <div className="flex shrink-0 items-center justify-between gap-2 border-b pl-gutter-left pr-gutter-right pb-3 pt-[calc(var(--safe-area-top)+12px)]">
          <div className="flex min-w-0 items-center gap-1">
            {section !== null && (
              <button
                type="button"
                onClick={backToList}
                aria-label="Back to settings"
                className="-ml-2 shrink-0 rounded-md text-muted-foreground touch:hit-target hover:text-foreground"
              >
                <CaretLeft className="h-5 w-5" aria-hidden />
              </button>
            )}
            <h2 ref={headingRef} tabIndex={-1} className={cn(pageTitle, 'truncate outline-none')}>
              {section === null ? 'Settings' : activeDefinition.label}
            </h2>
          </div>
          {closeButton}
        </div>

        <div
          key={section ?? 'list'}
          ref={paneRef}
          {...paneSwipe}
          className={cn(
            'min-h-0 flex-1 overflow-y-auto pl-gutter-left pr-gutter-right pb-4 duration-200 animate-in fade-in-0',
            enteringBack ? 'slide-in-from-left-4' : 'slide-in-from-right-4',
            section === null ? 'pt-2' : 'pt-4 pb-[calc(var(--safe-area-bottom)+16px)]',
            touchAction,
          )}
        >
          {showList ? navList : renderSection()}
        </div>

        {section === null && (
          <div className="pl-gutter-left pr-gutter-right pb-[calc(var(--safe-area-bottom)+12px)]">
            {footer(false)}
          </div>
        )}

        {clearCacheDialog}
      {deleteLogsDialog}
      </div>
    )
  }

  // Width lives on Dialog's own box; DialogContent size only caps the inner one, so a size above max-w-lg overflows the wrapper rather than widening it.
  return (
    <Dialog open={open} onOpenChange={onOpenChange} className="max-w-3xl">
      <DialogContent size="xl" className="h-[min(74dvh,640px)] flex-row gap-0 overflow-hidden p-0">
        <aside className="flex w-52 shrink-0 flex-col bg-muted px-3 pb-3 pt-4">
          {/* Radix requires a DialogTitle for the dialog's accessible name, even though it is hidden here. */}
          <DialogHeader className="sr-only">
            <DialogTitle>Settings</DialogTitle>
          </DialogHeader>

          {navList}

          <div className="mt-auto px-3 pt-4">{footer(true)}</div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          <div className="shrink-0 pl-6 pr-10 pb-3 pt-5">
            <h2 className="truncate text-lg font-semibold tracking-tight">{activeDefinition.label}</h2>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-6 pt-1 pb-6">
            {renderSection()}
          </div>
        </div>
        <DialogCloseButton onClick={() => onOpenChange(false)} aria-label="Close settings" />
      </DialogContent>

      {clearCacheDialog}
      {deleteLogsDialog}
    </Dialog>
  )
}
