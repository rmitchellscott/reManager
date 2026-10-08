import { CaretDown, Cpu, DotsThree, Eye, EyeSlash, Package, Trash, WifiHigh, WifiSlash } from '@phosphor-icons/react'
import { useState, useRef, useMemo, useEffect, type ReactNode } from 'react'
import { toast } from 'sonner'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Switch } from '@/components/ui/switch'
import { Select, SelectGroup, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuRadioGroup, DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu'
import { PageHeader } from '@/components/PageHeader'
import { SectionLabel } from '@/components/Panel'
import { cn } from '@/lib/utils'
import { relativeTime } from '@/lib/format'
import { ShellRcNoisyDialog } from '@/components/ShellRcNoisyDialog'
import type { SettingsSection } from '@/components/SettingsDialog'
import { useSwipeToDismiss } from '@/hooks/useSwipeToDismiss'
import { useBackHandler } from '@/hooks/useBackHandler'
import { ErrorCode, SyncKey, type Step, type SSHKey, type SavedDevice, type ManagedKey, type PlatformCapabilities, type DeviceOutlook } from '@/lib/types'
import { ConnectionService, DeviceService, SystemService } from '@bindings/reManager'
import { onSyncedChange } from '@/lib/syncEvents'

type AuthType = 'password' | 'key' | 'agent'

const headlineBand = 'mb-2.5'

const connectColumn = 'mx-auto w-full max-w-2xl'

const AUTH_CHOICES: { value: AuthType; label: string }[] = [
  { value: 'password', label: 'Password' },
  { value: 'key', label: 'Key' },
  { value: 'agent', label: 'Agent' },
]

const authChipClass = (active: boolean) => cn(
  'inline-flex flex-1 items-center justify-center px-3 py-1.5 sm:flex-none touch:hit-target',
  'rounded-[calc(var(--radius)-2px)] text-sm font-medium border cursor-pointer',
  'transition-colors duration-150 whitespace-nowrap',
  active
    ? 'bg-primary text-primary-foreground border-primary'
    : 'bg-card text-card-foreground border-border hover:bg-accent',
)

function FormRow({ label, detail, htmlFor, control, expansion, inlineControl }: {
  label: string
  detail?: React.ReactNode
  htmlFor?: string
  control?: React.ReactNode
  expansion?: React.ReactNode
  inlineControl?: boolean
}) {
  const detailId = htmlFor && detail ? `${htmlFor}-detail` : undefined
  return (
    <div className="px-3.5 py-3 border-t first:border-t-0">
      <div className={cn(
        'flex gap-4',
        inlineControl ? 'flex-row items-center justify-between' : 'flex-col sm:flex-row sm:items-center sm:justify-between',
      )}>
        <div className="min-w-0">
          {htmlFor
            ? <label htmlFor={htmlFor} className="text-sm font-medium">{label}</label>
            : <p className="text-sm font-medium">{label}</p>}
          {detail && <p id={detailId} className="text-xs leading-5 text-muted-foreground">{detail}</p>}
        </div>
        {control && (
          <div className={cn('shrink-0', !inlineControl && 'sm:w-64')}>{control}</div>
        )}
      </div>
      {expansion && <div className="mt-3">{expansion}</div>}
    </div>
  )
}

const MANAGED_KEY_PREFIX = 'managed:'

function KeyGroupLabel({ children }: { children: ReactNode }) {
  return <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">{children}</div>
}

function RevealButton({ shown, onToggle, label }: { shown: boolean; onToggle: () => void; label: string }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-label={label}
      className="absolute right-0 top-0 flex h-full w-10 items-center justify-center text-muted-foreground hover:text-foreground"
    >
      {shown ? <EyeSlash className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
    </button>
  )
}

const outlookBadge = 'inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-[oklch(from_var(--info)_l_c_h_/_25%)] bg-[var(--info-ground)] text-[var(--info-text)]'

const blockedBadge = 'border-border bg-secondary text-muted-foreground'

interface ConnectPageProps {
  step: Step
  setStep: (step: Step) => void
  savedDevices: SavedDevice[]
  deviceOutlooks?: Map<string, DeviceOutlook>
  setSavedDevices: (devices: SavedDevice[]) => void
  setDevice: (device: string) => void
  deviceInfo: Record<string, string>
  setConnectedDeviceId: (id: string) => void
  onConnected: (deviceType: string, freshDevice: boolean) => Promise<void>
  initialKeys: SSHKey[]
  initialAgentAvailable: boolean
  onSettings: (section?: SettingsSection) => void
  capabilities: PlatformCapabilities
}

export function ConnectPage({
  step,
  setStep,
  savedDevices,
  deviceOutlooks,
  setSavedDevices,
  setDevice,
  deviceInfo,
  setConnectedDeviceId,
  onConnected,
  initialKeys,
  initialAgentAvailable,
  onSettings,
  capabilities,
}: ConnectPageProps) {
  const [host, setHost] = useState('10.11.99.1')
  const [authType, setAuthType] = useState<AuthType>('password')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [keyPassphrase, setKeyPassphrase] = useState('')
  const [showKeyPassphrase, setShowKeyPassphrase] = useState(false)
  const [availableKeys, setAvailableKeys] = useState<SSHKey[]>(initialKeys)
  const showKeyFiles = capabilities.localKeyFiles
  const [managedKeys, setManagedKeys] = useState<ManagedKey[]>([])
  const [selectedKey, setSelectedKey] = useState<string>(initialKeys.length > 0 ? initialKeys[0].path : '')
  const [customKeyName, setCustomKeyName] = useState<string>('')
  const [agentRunning, setAgentRunning] = useState(initialAgentAvailable)
  const sshAgentAvailable = capabilities.sshAgent && agentRunning
  const [connecting, setConnecting] = useState(false)
  const [connectPhase, setConnectPhase] = useState<'dialing' | 'checking' | null>(null)
  const [connectError, setConnectError] = useState<{ message: string; detail: string } | null>(null)
  const reportConnectFailure = (failure: unknown) => setConnectError({
    message: handleError(failure, 'Could not connect to your reMarkable.'),
    detail: errorDetail(failure),
  })
  const connectAttemptRef = useRef(0)
  const [showShellRcNoisy, setShowShellRcNoisy] = useState(false)
  const retryConnectRef = useRef<() => void>(() => {})

  useEffect(() => {
    setAvailableKeys(initialKeys)
    if (initialKeys.length > 0 && !selectedKey) {
      setSelectedKey(initialKeys[0].path)
    }
  }, [initialKeys])

  useEffect(() => {
    setAgentRunning(initialAgentAvailable)
  }, [initialAgentAvailable])

  const [showAddForm, setShowAddForm] = useState(false)

  useEffect(() => {
    if (!showAddForm) return
    const load = () => ConnectionService.GetManagedKeys().then(keys => setManagedKeys(keys ?? [])).catch(() => setManagedKeys([]))
    load()
    return onSyncedChange(SyncKey.KeysSyncKey, load)
  }, [showAddForm])
  const selectedManagedKeyId = selectedKey.startsWith(MANAGED_KEY_PREFIX) ? selectedKey.slice(MANAGED_KEY_PREFIX.length) : ''
  const selectedKeyPath = selectedManagedKeyId ? '' : selectedKey
  const [editingDevice, setEditingDevice] = useState<SavedDevice | null>(null)
  const [deviceName, setDeviceName] = useState('')
  const [rememberDevice, setRememberDevice] = useState(true)
  const [deviceToDelete, setDeviceToDelete] = useState<string | null>(null)
  const [connectingDeviceId, setConnectingDeviceId] = useState<string | null>(null)
  const [deviceSortMode, setDeviceSortMode] = useState<'recent' | 'alpha'>(() => {
    const saved = localStorage.getItem('deviceSortMode')
    return saved === 'alpha' ? 'alpha' : 'recent'
  })

  const sortedDevices = useMemo(() => {
    return [...savedDevices].sort((a, b) => {
      if (deviceSortMode === 'alpha') {
        return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
      }
      return (b.lastConnected || 0) - (a.lastConnected || 0)
    })
  }, [savedDevices, deviceSortMode])

  const resetFormFields = () => {
    setHost('10.11.99.1')
    setAuthType('password')
    setPassword('')
    setKeyPassphrase('')
    setDeviceName('')
    setRememberDevice(true)
    if (showKeyFiles && availableKeys.length > 0) {
      setSelectedKey(availableKeys[0].path)
    } else if (managedKeys.length > 0) {
      setSelectedKey(MANAGED_KEY_PREFIX + managedKeys[0].id)
    } else {
      setSelectedKey('')
    }
  }

  const handleKeySelect = async (value: string) => {
    if (value === '__manage__') {
      onSettings('connection')
      return
    }
    if (value === '__other__') {
      const path = await ConnectionService.SelectKeyFile()
      if (path) {
        setSelectedKey(path)
        const fileName = path.split('/').pop() || path
        setCustomKeyName(fileName)
      } else if (!selectedKey) {
        setCustomKeyName('')
      }
    } else {
      setSelectedKey(value)
      setCustomKeyName('')
    }
  }

  const handleConnect = async (saveAfterConnect: boolean) => {
    retryConnectRef.current = () => handleConnect(saveAfterConnect)
    const thisAttempt = ++connectAttemptRef.current
    let sessionOpened = false
    setConnecting(true)
    setConnectError(null)
    setConnectPhase('dialing')

    try {
      let result
      if (authType === 'agent') {
        result = await ConnectionService.ConnectWithAgent(host)
      } else if (authType === 'key') {
        result = selectedManagedKeyId
          ? await ConnectionService.ConnectWithManagedKey(host, selectedManagedKeyId, keyPassphrase)
          : await ConnectionService.ConnectWithKey(host, selectedKeyPath, keyPassphrase)
      } else {
        result = await ConnectionService.Connect(host, password)
      }

      if (thisAttempt !== connectAttemptRef.current) {
        return
      }

      if (result.success) {
        sessionOpened = true
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        setConnectPhase('checking')
        await onConnected(deviceType, !!result.freshDevice)
        if (thisAttempt !== connectAttemptRef.current) return

        if (saveAfterConnect) {
          const info = await DeviceService.GetDeviceInfo()
          const name = deviceName.trim() || info?.machine || host
          try {
            await ConnectionService.SaveDevice(
              '', name, host, authType,
              authType === 'password' ? password : '',
              authType === 'key' ? selectedKeyPath : '',
              authType === 'key' ? keyPassphrase : '',
              authType === 'key' ? selectedManagedKeyId : '',
            )
            setSavedDevices(await ConnectionService.GetSavedDevices() || [])
          } catch (err) {
            toast.error(handleError(err, 'Could not save this reMarkable.'))
          }
        }
        setStep('select')
        setShowAddForm(false)
      } else if (result.code === ErrorCode.ErrShellRCNoisy) {
        setShowShellRcNoisy(true)
      } else {
        reportConnectFailure(result.code ? result : result.message)
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) {
        return
      }
      if (sessionOpened) void ConnectionService.Disconnect().catch(() => {})
      reportConnectFailure(err)
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
        setConnectPhase(null)
      }
    }
  }

  const handleConnectToSavedDevice = async (id: string) => {
    retryConnectRef.current = () => handleConnectToSavedDevice(id)
    const thisAttempt = ++connectAttemptRef.current
    let sessionOpened = false
    setConnecting(true)
    setConnectError(null)
    setConnectPhase('dialing')
    setConnectingDeviceId(id)

    try {
      const result = await ConnectionService.ConnectToSavedDevice(id)

      if (thisAttempt !== connectAttemptRef.current) return

      if (result.success) {
        sessionOpened = true
        setConnectedDeviceId(id)
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        setConnectPhase('checking')
        await onConnected(deviceType, !!result.freshDevice)
        if (thisAttempt !== connectAttemptRef.current) return
        setStep('select')
      } else if (result.code === ErrorCode.ErrShellRCNoisy) {
        setShowShellRcNoisy(true)
      } else if (result.code === ErrorCode.ErrKeyNotChosen) {
        const saved = savedDevices.find(d => d.id === id)
        toast.info(result.message)
        if (saved) handleEditDevice({ ...saved, authType: 'key' })
      } else {
        reportConnectFailure(result.code ? result : result.message)
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) return
      if (sessionOpened) void ConnectionService.Disconnect().catch(() => {})
      reportConnectFailure(err)
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
        setConnectPhase(null)
        setConnectingDeviceId(null)
      }
    }
  }

  const connectingDeviceLabel = (() => {
    const saved = connectingDeviceId
      ? savedDevices.find(d => d.id === connectingDeviceId)
      : null
    const name = saved ? saved.name : deviceName.trim()
    const target = saved ? saved.host : host
    return name ? `${name} · ${target}` : target
  })()

  const handleCancelConnect = async () => {
    connectAttemptRef.current++
    setConnecting(false)
    setConnectPhase(null)
    setConnectingDeviceId(null)
    setDevice('')
    setConnectedDeviceId('')
    await ConnectionService.CancelConnect()
  }

  const handleDeleteClick = (id: string) => {
    setDeviceToDelete(id)
  }

  const handleConfirmDelete = async () => {
    if (!deviceToDelete) return
    try {
      await ConnectionService.DeleteSavedDevice(deviceToDelete)
      setSavedDevices(savedDevices.filter(d => d.id !== deviceToDelete))
    } catch (err) {
      toast.error(handleError(err, 'Could not remove this reMarkable.'))
    }
    setDeviceToDelete(null)
  }

  const handleEditDevice = (device: SavedDevice) => {
    setEditingDevice(device)
    setDeviceName(device.name)
    setHost(device.host)
    setAuthType(device.authType as AuthType)
    if (device.authType === 'key') {
      if (showKeyFiles && device.keyPath) setSelectedKey(device.keyPath)
      else if (device.keyId) setSelectedKey(MANAGED_KEY_PREFIX + device.keyId)
      else setSelectedKey('')
    }
    setShowAddForm(true)
  }

  const handleSaveEditedDevice = async () => {
    if (!editingDevice) return
    try {
      const pw = authType === 'password' ? password : ''
      const kp = authType === 'key' ? selectedKeyPath : ''
      const kpp = authType === 'key' ? keyPassphrase : ''
      const kid = authType === 'key' ? selectedManagedKeyId : ''

      await ConnectionService.SaveDevice(editingDevice.id, deviceName, host, authType, pw, kp, kpp, kid)

      const devices = await ConnectionService.GetSavedDevices()
      setSavedDevices(devices || [])

      setEditingDevice(null)
      setShowAddForm(false)
      resetFormFields()
    } catch (err) {
      toast.error(handleError(err, 'Could not save this reMarkable.'))
    }
  }

  const handleCancelForm = () => {
    setShowAddForm(false)
    setEditingDevice(null)
    resetFormFields()
  }

  // The store reads an empty secret as "keep what is there", so editing never requires the credential again.
  const credentialReady = authType === 'agent' ? true
    : authType === 'key' ? (!!selectedKey && selectedKey !== '__other__' && selectedKey !== '__manage__')
    : (!!editingDevice || !!password)

  const canSubmit = editingDevice
    ? !!deviceName.trim() && credentialReady
    : credentialReady

  const canLeaveForm = showAddForm && savedDevices.length > 0 && !connecting
  const swipeBack = useSwipeToDismiss({
    direction: 'right',
    onDismiss: handleCancelForm,
    enabled: canLeaveForm,
  })
  useBackHandler(handleCancelForm, step === 'connect' && canLeaveForm)

  const submitOnEnter = () => {
    if (connecting || !canSubmit) return
    if (editingDevice) handleSaveEditedDevice()
    else handleConnect(rememberDevice)
  }

  if (step !== 'connect' && !showShellRcNoisy) return null

  const screenTitle = savedDevices.length > 0 && !showAddForm
    ? 'Choose a reMarkable'
    : editingDevice ? 'Edit reMarkable' : 'Add a reMarkable'

  return (
    <>
      {step === 'connect' && (
        <PageHeader
          device=""
          narrowTitle={screenTitle}
          onSettings={onSettings}
          onDisconnect={() => {}}
        />
      )}
      {step !== 'connect' ? null : savedDevices.length > 0 && !showAddForm ? (
        // Keyed so React never reuses one branch's DOM node for the other: the swipe leaves an inline transform that a reused node would inherit, rendering the list shoved off-screen.
        <div key="device-list" className={connectColumn}>
          <div className={cn(headlineBand, 'flex items-center justify-between gap-3')}>
            <div className="hidden md:block [&_h3]:mb-0">
              <SectionLabel>Choose a reMarkable</SectionLabel>
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="ml-auto max-md:h-10 md:h-9 px-2.5 text-xs gap-1 shrink-0">
                  {deviceSortMode === 'recent' ? 'Recent' : 'A-Z'}
                  <CaretDown className="h-4 w-4 shrink-0" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-[11rem]">
                <DropdownMenuRadioGroup
                  value={deviceSortMode}
                  onValueChange={(v) => {
                    localStorage.setItem('deviceSortMode', v)
                    setDeviceSortMode(v as 'recent' | 'alpha')
                  }}
                >
                  <DropdownMenuRadioItem value="recent">Recently connected</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="alpha">Name, A to Z</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          <div className="space-y-3">
          <div className="rounded-[var(--radius)] border overflow-hidden">
            {sortedDevices.map((savedDevice, i) => {
              return (
                <div
                  key={savedDevice.id}
                  className={cn(
                    'flex items-center gap-3 px-3.5 py-2.5',
                    i > 0 && 'border-t',
                  )}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <span className="font-semibold truncate">{savedDevice.name}</span>
                      {(() => {
                        const outlook = deviceOutlooks?.get(savedDevice.id)
                        if (!outlook) return null
                        return (
                          <>
                            {outlook.upgrades > 0 && (
                              <span className={outlookBadge} title="Upgrades available">
                                <Package className="h-3 w-3" weight="fill" />
                              </span>
                            )}
                            {outlook.osUpdate && (() => {
                              const blocked = !!outlook.osBlocked
                              return (
                                <span
                                  className={cn(outlookBadge, blocked && blockedBadge)}
                                  title={blocked
                                    ? 'reMarkable OS update available, blocked by installed mods'
                                    : 'reMarkable OS update available'}
                                >
                                  <Cpu className="h-3 w-3" weight={blocked ? 'regular' : 'fill'} />
                                </span>
                              )
                            })()}
                          </>
                        )
                      })()}
                    </div>
                    <div className="text-xs text-muted-foreground truncate">
                      {`${savedDevice.host} · ${relativeTime(savedDevice.lastConnected || 0)}`}
                    </div>
                  </div>

                  {(
                    <>
                      <Button
                        variant="outline"
                        className="hidden sm:inline-flex shrink-0"
                        disabled={connecting}
                        aria-label={`Edit ${savedDevice.name}`}
                        onClick={() => handleEditDevice(savedDevice)}
                      >
                        Edit
                      </Button>
                      <Button
                        variant="outline"
                        className="hidden sm:inline-flex shrink-0"
                        disabled={connecting}
                        aria-label={`Remove ${savedDevice.name}`}
                        onClick={() => handleDeleteClick(savedDevice.id)}
                      >
                        Remove
                      </Button>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="sm:hidden shrink-0 text-muted-foreground hit-target"
                            disabled={connecting}
                            aria-label={`More options for ${savedDevice.name}`}
                          >
                            <DotsThree className="h-5 w-5" weight="bold" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="min-w-[9rem]">
                          <DropdownMenuItem onSelect={() => handleEditDevice(savedDevice)}>
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onSelect={() => handleDeleteClick(savedDevice.id)}
                          >
                            Remove
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                      <Button
                        className="shrink-0 hit-target"
                        disabled={connecting}
                        aria-label={`Connect to ${savedDevice.name}`}
                        onClick={() => handleConnectToSavedDevice(savedDevice.id)}
                      >
                        Connect
                      </Button>
                    </>
                  )}
                </div>
              )
            })}
          </div>

          <Button
            variant="outline"
            className="w-full"
            onClick={() => setShowAddForm(true)}
            disabled={connecting}
          >
            Add reMarkable
          </Button>

          <button
            className="w-full text-center text-xs text-muted-foreground hover:text-foreground touch:hit-target"
            onClick={() => setStep('browse')}
            disabled={connecting}
          >
            Browse mods
          </button>
          </div>
        </div>
      ) : (
        <div
          key="device-form"
          ref={swipeBack.ref}
          {...swipeBack.swipeHandlers}
          className={cn('flex flex-1 flex-col -ml-gutter-left -mr-gutter-right pl-gutter-left pr-gutter-right', swipeBack.touchAction)}
        >
          <div className={connectColumn}>
          <div className={headlineBand}>
          <div className="hidden md:block [&_h3]:mb-0">
            <SectionLabel>{editingDevice ? 'Edit reMarkable' : 'Add a reMarkable'}</SectionLabel>
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Paper Pro, Paper Pro Move, and Paper Pure need{' '}
            <button
              onClick={() => SystemService.OpenExternalURL('https://support.remarkable.com/s/article/Developer-mode')}
              className="underline hover:text-foreground">
              developer mode
            </button>{' '}
            turned on first.
          </p>

          </div>

          <div className="space-y-3">
          <div className="rounded-[var(--radius)] border">
            {editingDevice ? (
              <FormRow
                label="Name"
                htmlFor="deviceName"
                detail="What this reMarkable is called in your list."
                control={
                  <Input
                    id="deviceName"
                    value={deviceName}
                    onChange={(e) => setDeviceName(e.target.value)}
                    placeholder={deviceInfo.machine || 'My reMarkable'}
                  />
                }
              />
            ) : (
              <FormRow
                label="Remember this reMarkable"
                htmlFor="remember-device"
                detail="Saves it to your list, with credentials in your system keychain."
                inlineControl
                control={
                  <Switch
                    id="remember-device"
                    className="touch:hit-target"
                    checked={rememberDevice}
                    onCheckedChange={setRememberDevice}
                  />
                }
                expansion={rememberDevice ? (
                  <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                    <div className="min-w-0">
                      <label htmlFor="deviceName" className="text-sm font-medium">Name</label>
                      <p className="text-xs leading-5 text-muted-foreground">
                        Leave blank to use the name your reMarkable reports.
                      </p>
                    </div>
                    <div className="shrink-0 sm:w-64">
                      <Input
                        id="deviceName"
                        value={deviceName}
                        onChange={(e) => setDeviceName(e.target.value)}
                        placeholder={deviceInfo.machine || 'My reMarkable'}
                      />
                    </div>
                  </div>
                ) : undefined}
              />
            )}

            <FormRow
              label="IP address"
              htmlFor="host"
              detail="Settings › General › Help › Copyrights and licenses."
              control={
                <Input
                  id="host"
                  value={host}
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  inputMode="url"
                  onChange={(e) => setHost(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') submitOnEnter() }}
                  placeholder="10.11.99.1"
                />
              }
            />

            <FormRow
              label="Sign in with"
              control={
                <div className="flex gap-1.5" role="group" aria-label="Authentication method">
                  {AUTH_CHOICES.map(choice => (
                    (choice.value !== 'agent' || sshAgentAvailable) && (
                      <button
                        key={choice.value}
                        type="button"
                        aria-pressed={authType === choice.value}
                        onClick={() => setAuthType(choice.value)}
                        className={authChipClass(authType === choice.value)}
                      >
                        {choice.label}
                      </button>
                    )
                  ))}
                </div>
              }
              expansion={
                authType === 'agent' ? (
                  <p className="text-xs leading-5 text-muted-foreground">
                    reManager will try every key your running SSH agent holds.
                  </p>
                ) : authType === 'password' ? (
                  <div>
                    <div className="relative">
                      <Input
                        id="password"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        autoCapitalize="none"
                        autoCorrect="off"
                        spellCheck={false}
                        onChange={(e) => setPassword(e.target.value)}
                        onKeyDown={(e) => { if (e.key === 'Enter') submitOnEnter() }}
                        placeholder={editingDevice ? 'Saved. Leave blank to keep it.' : 'SSH password'}
                        aria-label="SSH password"
                        className="pr-10"
                      />
                      <RevealButton
                        shown={showPassword}
                        onToggle={() => setShowPassword(!showPassword)}
                        label={showPassword ? 'Hide password' : 'Show password'}
                      />
                    </div>
                  </div>
                ) : (
                  <div className="space-y-2.5">
                    <Select value={selectedKey} onValueChange={handleKeySelect}>
                      <SelectTrigger aria-label="SSH key">
                        <SelectValue placeholder="Select a key">
                          {selectedManagedKeyId
                            ? managedKeys.find(k => k.id === selectedManagedKeyId)?.name || 'Select a key'
                            : customKeyName || availableKeys.find(k => k.path === selectedKey)?.name || 'Select a key'}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {showKeyFiles && (
                          <SelectGroup>
                            {managedKeys.length > 0 && <KeyGroupLabel>On this computer</KeyGroupLabel>}
                            {availableKeys.map((key) => (
                              <SelectItem key={key.path} value={key.path}>
                                {key.name}
                              </SelectItem>
                            ))}
                            <SelectItem value="__other__">Other…</SelectItem>
                          </SelectGroup>
                        )}
                        <SelectGroup>
                          {showKeyFiles && managedKeys.length > 0 && <KeyGroupLabel>Stored in reManager</KeyGroupLabel>}
                          {managedKeys.map((key) => (
                            <SelectItem key={key.id} value={MANAGED_KEY_PREFIX + key.id}>
                              {key.name}
                            </SelectItem>
                          ))}
                          <SelectItem value="__manage__">Add a key…</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <div>
                      <div className="relative">
                        <Input
                          id="keyPassphrase"
                          type={showKeyPassphrase ? 'text' : 'password'}
                          value={keyPassphrase}
                          onChange={(e) => setKeyPassphrase(e.target.value)}
                          onKeyDown={(e) => { if (e.key === 'Enter') submitOnEnter() }}
                          placeholder={editingDevice ? 'Saved. Leave blank to keep it.' : 'Passphrase'}
                          aria-label="Key passphrase"
                          className="pr-10"
                        />
                        <RevealButton
                          shown={showKeyPassphrase}
                          onToggle={() => setShowKeyPassphrase(!showKeyPassphrase)}
                          label={showKeyPassphrase ? 'Hide passphrase' : 'Show passphrase'}
                        />
                      </div>
                      {!editingDevice && (
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          Leave blank if the key is not encrypted.
                        </p>
                      )}
                    </div>
                  </div>
                )
              }
            />
          </div>

          <div className={cn(
            'flex flex-col-reverse gap-2 sm:flex-row sm:justify-between',
            '[&>button]:w-full sm:[&>button]:w-auto',
          )}>
            {savedDevices.length === 0 ? <span className="hidden sm:block" /> : (
              <Button variant="outline" onClick={handleCancelForm}>
                Cancel
              </Button>
            )}
            {editingDevice ? (
              <Button onClick={handleSaveEditedDevice} disabled={!canSubmit}>
                Save
              </Button>
            ) : (
              <Button onClick={() => handleConnect(rememberDevice)} disabled={!canSubmit}>
                Connect
              </Button>
            )}
          </div>

          {savedDevices.length === 0 && (
            <button
              className="w-full text-center text-xs text-muted-foreground hover:text-foreground touch:hit-target"
              onClick={() => setStep('browse')}
              disabled={connecting}
            >
              Browse mods
            </button>
          )}
          </div>
          </div>
        </div>
      )}

      <Dialog
        open={connectPhase !== null || connectError !== null}
        onOpenChange={(open) => { if (!open) setConnectError(null) }}
        closable={connectError !== null}
      >
        <DialogContent>
          {connectError ? (
            <>
              <DialogHeader severity="destructive" icon={WifiSlash}>
                <DialogTitle>Connection failed</DialogTitle>
                <DialogDescription>{connectError.message}</DialogDescription>
              </DialogHeader>
              <OutputDisclosure output={connectError.detail} />
              <DialogFooter>
                <Button variant="outline" onClick={() => setConnectError(null)}>
                  Close
                </Button>
                <Button onClick={() => { setConnectError(null); retryConnectRef.current() }}>
                  Try again
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader severity="info" icon={WifiHigh}>
                <DialogTitle>Connecting</DialogTitle>
                <DialogDescription>
                  {connectPhase === 'checking'
                    ? 'Loading from your reMarkable'
                    : connectingDeviceLabel}
                </DialogDescription>
              </DialogHeader>

              <Progress
                indeterminate
                className="h-1 bg-muted"
                indicatorClassName="bg-[var(--info)]"
                aria-label="Connecting"
              />
              <DialogFooter>
                <Button variant="outline" onClick={handleCancelConnect}>
                  Cancel
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={deviceToDelete !== null} onOpenChange={(open) => !open && setDeviceToDelete(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Trash}>
            <DialogTitle>Remove "{savedDevices.find(d => d.id === deviceToDelete)?.name}"?</DialogTitle>
            <DialogDescription>
              This will remove the saved connection and credentials.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeviceToDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleConfirmDelete}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ShellRcNoisyDialog
        open={showShellRcNoisy}
        isRetrying={connecting}
        onClose={() => setShowShellRcNoisy(false)}
        onRetry={() => { setShowShellRcNoisy(false); retryConnectRef.current() }}
      />
    </>
  )
}
