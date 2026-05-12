import { useState, useRef, useMemo } from 'react'
import { toast } from 'sonner'
import { handleError, getUserFriendlyMessage } from '@/lib/errorMessages'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Loader2, Eye, EyeOff } from 'lucide-react'
import type { Step, SSHKey, SavedDevice } from '@/lib/types'

export interface ConnectPageProps {
  step: Step
  setStep: (step: Step) => void
  savedDevices: SavedDevice[]
  setSavedDevices: (devices: SavedDevice[]) => void
  setDevice: (device: string) => void
  deviceInfo: Record<string, string>
  setConnectedDeviceId: (id: string) => void
  onConnected: (deviceType: string) => Promise<void>
  initialKeys: SSHKey[]
  initialAgentAvailable: boolean
}

export function ConnectPage({
  step,
  setStep,
  savedDevices,
  setSavedDevices,
  setDevice,
  deviceInfo,
  setConnectedDeviceId,
  onConnected,
  initialKeys,
  initialAgentAvailable,
}: ConnectPageProps) {
  const [host, setHost] = useState('10.11.99.1')
  const [authType, setAuthType] = useState<'password' | 'key' | 'agent'>('password')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [keyPassphrase, setKeyPassphrase] = useState('')
  const [showKeyPassphrase, setShowKeyPassphrase] = useState(false)
  const [availableKeys] = useState<SSHKey[]>(initialKeys)
  const [selectedKey, setSelectedKey] = useState<string>(initialKeys.length > 0 ? initialKeys[0].path : '')
  const [customKeyName, setCustomKeyName] = useState<string>('')
  const [sshAgentAvailable] = useState(initialAgentAvailable)
  const [connecting, setConnecting] = useState(false)
  const connectAttemptRef = useRef(0)

  const [showAddForm, setShowAddForm] = useState(false)
  const [editingDevice, setEditingDevice] = useState<SavedDevice | null>(null)
  const [deviceName, setDeviceName] = useState('')
  const [deviceToDelete, setDeviceToDelete] = useState<string | null>(null)
  const [connectingDeviceId, setConnectingDeviceId] = useState<string | null>(null)
  const [showSaveDeviceDialog, setShowSaveDeviceDialog] = useState(false)
  const [saveDeviceError, setSaveDeviceError] = useState('')
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
    if (availableKeys.length > 0) {
      setSelectedKey(availableKeys[0].path)
    } else {
      setSelectedKey('')
    }
  }

  const handleKeySelect = async (value: string) => {
    if (value === '__other__') {
      const path = await window.go.main.App.SelectKeyFile()
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
    const thisAttempt = ++connectAttemptRef.current
    setConnecting(true)

    try {
      let result
      if (authType === 'agent') {
        result = await window.go.main.App.ConnectWithAgent(host)
      } else if (authType === 'key') {
        result = await window.go.main.App.ConnectWithKey(host, selectedKey, keyPassphrase)
      } else {
        result = await window.go.main.App.Connect(host, password)
      }

      if (thisAttempt !== connectAttemptRef.current) {
        return
      }

      if (result.success) {
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        await onConnected(deviceType)

        if (saveAfterConnect) {
          const info = await window.go.main.App.GetDeviceInfo()
          const displayName = info.machine || ''
          setDeviceName(displayName || host)
          setShowSaveDeviceDialog(true)
        } else {
          setStep('select')
        }
        setShowAddForm(false)
      } else {
        toast.error(result.code ? getUserFriendlyMessage(result) : handleError(result.message, 'Connection'))
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) {
        return
      }
      toast.error(handleError(err, 'Connection'))
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
      }
    }
  }

  const handleConnectToSavedDevice = async (id: string) => {
    const thisAttempt = ++connectAttemptRef.current
    setConnecting(true)
    setConnectingDeviceId(id)

    try {
      const result = await window.go.main.App.ConnectToSavedDevice(id)

      if (thisAttempt !== connectAttemptRef.current) return

      if (result.success) {
        setConnectedDeviceId(id)
        const deviceType = result.device || 'unknown'
        setDevice(deviceType)
        await onConnected(deviceType)
        setStep('select')
      } else {
        toast.error(result.code ? getUserFriendlyMessage(result) : handleError(result.message, 'Connection'))
      }
    } catch (err) {
      if (thisAttempt !== connectAttemptRef.current) return
      toast.error(handleError(err, 'Connection'))
    } finally {
      if (thisAttempt === connectAttemptRef.current) {
        setConnecting(false)
        setConnectingDeviceId(null)
      }
    }
  }

  const handleCancelConnect = async () => {
    connectAttemptRef.current++
    await window.go.main.App.CancelConnect()
    setConnecting(false)
  }

  const handleDeleteClick = (id: string) => {
    setDeviceToDelete(id)
  }

  const handleConfirmDelete = async () => {
    if (!deviceToDelete) return
    try {
      await window.go.main.App.DeleteSavedDevice(deviceToDelete)
      setSavedDevices(savedDevices.filter(d => d.id !== deviceToDelete))
    } catch (err) {
      console.error('Failed to delete saved device:', err)
    }
    setDeviceToDelete(null)
  }

  const handleEditDevice = (device: SavedDevice) => {
    setEditingDevice(device)
    setDeviceName(device.name)
    setHost(device.host)
    setAuthType(device.authType as 'password' | 'key' | 'agent')
    if (device.authType === 'key' && device.keyPath) {
      setSelectedKey(device.keyPath)
    }
    setShowAddForm(true)
  }

  const handleSaveEditedDevice = async () => {
    if (!editingDevice) return
    try {
      const pw = authType === 'password' ? password : ''
      const kp = authType === 'key' ? selectedKey : ''
      const kpp = authType === 'key' ? keyPassphrase : ''

      await window.go.main.App.SaveDevice(editingDevice.id, deviceName, host, authType, pw, kp, kpp)

      const devices = await window.go.main.App.GetSavedDevices()
      setSavedDevices(devices || [])

      setEditingDevice(null)
      setShowAddForm(false)
      resetFormFields()
    } catch (err) {
      console.error('Failed to save device:', err)
    }
  }

  const handleCancelForm = () => {
    setShowAddForm(false)
    setEditingDevice(null)
    resetFormFields()
  }

  const handleSaveDevice = async () => {
    setSaveDeviceError('')
    try {
      const pw = authType === 'password' ? password : ''
      const kp = authType === 'key' ? selectedKey : ''
      const kpp = authType === 'key' ? keyPassphrase : ''

      await window.go.main.App.SaveDevice('', deviceName, host, authType, pw, kp, kpp)

      const devices = await window.go.main.App.GetSavedDevices()
      setSavedDevices(devices || [])

      setShowSaveDeviceDialog(false)
      setStep('select')
    } catch (err) {
      const friendlyMessage = handleError(err, 'Save device')
      if (friendlyMessage.includes('keyring')) {
        const devices = await window.go.main.App.GetSavedDevices()
        setSavedDevices(devices || [])
      }
      setSaveDeviceError(friendlyMessage)
    }
  }

  if (step !== 'connect' && !showSaveDeviceDialog) return null

  return (
    <>
      {step !== 'connect' ? null : savedDevices.length > 0 && !showAddForm ? (
        <div className="space-y-4">
          <div className="flex justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setDeviceSortMode(m => {
                    const next = m === 'recent' ? 'alpha' : 'recent'
                    localStorage.setItem('deviceSortMode', next)
                    return next
                  })}
                >
                  {deviceSortMode === 'recent' ? 'Recent' : 'A-Z'}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {deviceSortMode === 'recent' ? 'Sort alphabetically' : 'Sort by recent'}
              </TooltipContent>
            </Tooltip>
          </div>
          {sortedDevices.map((savedDevice) => (
            <Card key={savedDevice.id}>
              <CardContent className="pt-6">
                <div className="flex items-center justify-between">
                  <div className="space-y-1">
                    <h3 className="font-semibold text-lg">{savedDevice.name}</h3>
                    <p className="text-sm text-muted-foreground">{savedDevice.host}</p>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      onClick={() => handleEditDevice(savedDevice)}
                      disabled={connecting}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => handleDeleteClick(savedDevice.id)}
                      disabled={connecting}
                    >
                      Remove
                    </Button>
                    {connecting && connectingDeviceId === savedDevice.id ? (
                      <Button onClick={handleCancelConnect} variant="outline">
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Cancel
                      </Button>
                    ) : (
                      <Button
                        onClick={() => handleConnectToSavedDevice(savedDevice.id)}
                        disabled={connecting}
                      >
                        Connect
                      </Button>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
          <Button
            variant="outline"
            className="w-full"
            onClick={() => setShowAddForm(true)}
          >
            Add reMarkable
          </Button>
        </div>
      ) : (
        <Card>
          <CardHeader>
            {editingDevice ? (
              <Input
                value={deviceName}
                onChange={(e) => setDeviceName(e.target.value)}
                placeholder="Device Name"
                className="text-2xl font-semibold h-auto py-1 px-2 -mx-2"
              />
            ) : (
              <CardTitle>Connect to reMarkable</CardTitle>
            )}
            <CardDescription>
              Find your IP and password in Settings - General - Help - Copyrights and licenses.
              <br />
              Paper Pro, Paper Pro Move, and Paper Pure require{' '}
              <button
                onClick={() => window.runtime.BrowserOpenURL('https://support.remarkable.com/s/article/Developer-mode')}
                className="underline hover:text-foreground">
                developer mode
              </button>{' '}
              to be enabled.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="host">IP Address</Label>
              <Input
                id="host"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !connecting) {
                    const canConnect = authType === 'agent' ? true : authType === 'password' ? !!password : (!!selectedKey && selectedKey !== '__other__')
                    if (canConnect) handleConnect(true)
                  }
                }}
                placeholder="10.11.99.1"
              />
            </div>

            <div className="space-y-2">
              <Label>Authentication</Label>
              <RadioGroup
                value={authType}
                onValueChange={(value) => setAuthType(value as 'password' | 'key' | 'agent')}
                className="flex gap-4"
              >
                <div className="flex items-center gap-2">
                  <RadioGroupItem value="password" id="auth-password" />
                  <Label htmlFor="auth-password" className="cursor-pointer font-normal">Password</Label>
                </div>
                <div className="flex items-center gap-2">
                  <RadioGroupItem value="key" id="auth-key" />
                  <Label htmlFor="auth-key" className="cursor-pointer font-normal">
                    SSH Key
                  </Label>
                </div>
                {sshAgentAvailable && (
                  <div className="flex items-center gap-2">
                    <RadioGroupItem value="agent" id="auth-agent" />
                    <Label htmlFor="auth-agent" className="cursor-pointer font-normal">
                      SSH Agent
                    </Label>
                  </div>
                )}
              </RadioGroup>
            </div>

            {authType === 'agent' ? (
              <p className="text-sm text-muted-foreground">
                Authentication will use keys from your running SSH agent.
              </p>
            ) : authType === 'password' ? (
              <div className="space-y-2">
                <Label htmlFor="password">SSH Password</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && !connecting && password) {
                        handleConnect(true)
                      }
                    }}
                    placeholder="Enter SSH password"
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <Label htmlFor="sshKey">SSH Key</Label>
                  <Select value={selectedKey} onValueChange={handleKeySelect}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select a key">
                        {customKeyName || availableKeys.find(k => k.path === selectedKey)?.name || 'Select a key'}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {availableKeys.map((key) => (
                        <SelectItem key={key.path} value={key.path}>
                          {key.name}
                        </SelectItem>
                      ))}
                      <SelectItem value="__other__">Other...</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="keyPassphrase">Key Passphrase (if encrypted)</Label>
                  <div className="relative">
                    <Input
                      id="keyPassphrase"
                      type={showKeyPassphrase ? "text" : "password"}
                      value={keyPassphrase}
                      onChange={(e) => setKeyPassphrase(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && !connecting) {
                          handleConnect(true)
                        }
                      }}
                      placeholder="Leave empty if not encrypted"
                      className="pr-10"
                    />
                    <button
                      type="button"
                      onClick={() => setShowKeyPassphrase(!showKeyPassphrase)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      {showKeyPassphrase ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                </div>
              </>
            )}

            <div className="flex justify-between">
              <div>
                {savedDevices.length > 0 && (
                  <Button variant="outline" onClick={handleCancelForm} disabled={connecting}>
                    Cancel
                  </Button>
                )}
              </div>
              <div className="flex gap-2">
                {connecting ? (
                  <Button onClick={handleCancelConnect} variant="outline">
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Cancel
                  </Button>
                ) : editingDevice ? (
                  <Button
                    onClick={handleSaveEditedDevice}
                    disabled={!deviceName.trim() || (authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey)}
                  >
                    Save
                  </Button>
                ) : (
                  <>
                    <Button
                      variant="outline"
                      onClick={() => handleConnect(false)}
                      disabled={authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey}
                    >
                      Connect
                    </Button>
                    <Button
                      onClick={() => handleConnect(true)}
                      disabled={authType === 'agent' ? false : authType === 'password' ? !password : !selectedKey}
                    >
                      Save and Connect
                    </Button>
                  </>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Remove Confirmation Dialog */}
      <Dialog open={deviceToDelete !== null} onOpenChange={(open) => !open && setDeviceToDelete(null)}>
        <DialogContent>
          <DialogHeader>
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

      {/* Save Device Dialog */}
      <Dialog open={showSaveDeviceDialog} onOpenChange={(open) => {
        setShowSaveDeviceDialog(open)
        if (!open) {
          setSaveDeviceError('')
          setStep('select')
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Device</DialogTitle>
            <DialogDescription>
              Save this device for quick reconnection in the future. Your credentials will be stored securely.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="deviceName">Device Name</Label>
              <Input
                id="deviceName"
                value={deviceName}
                onChange={(e) => setDeviceName(e.target.value)}
                placeholder={deviceInfo.machine || 'My reMarkable'}
              />
            </div>
            {saveDeviceError && (
              <p className="text-sm text-destructive">{saveDeviceError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => { setShowSaveDeviceDialog(false); setStep('select') }}
            >
              {saveDeviceError ? 'Close' : 'Skip'}
            </Button>
            {!saveDeviceError && (
              <Button onClick={handleSaveDevice} disabled={!deviceName.trim()}>
                Save Device
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
