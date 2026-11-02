import { CircleNotch, Key, Plus, Trash } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { panelAction, PanelGroup, PanelRow } from '@/components/Panel'
import type { ManagedKey } from '@/lib/types'
import { cn } from '@/lib/utils'
import { handleError } from '@/lib/errorMessages'
import { ConnectionService } from '@bindings/reManager'

const keyTextClass = 'flex min-h-28 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs leading-5 touch:no-focus-zoom ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'

function keyDetail(key: ManagedKey): string {
  if (key.fingerprint) {
    return key.type ? `${key.type} · ${key.fingerprint}` : key.fingerprint
  }
  return 'Encrypted. reManager asks for the passphrase when it connects.'
}

export function SSHKeyManager({ keys, onKeysChange, canChooseFile }: {
  keys: ManagedKey[]
  onKeysChange: (keys: ManagedKey[]) => void
  canChooseFile: boolean
}) {
  const [importing, setImporting] = useState(false)
  const [name, setName] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [saving, setSaving] = useState(false)
  const [keyToRemove, setKeyToRemove] = useState<string | null>(null)
  const [keyToRename, setKeyToRename] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [renaming, setRenaming] = useState(false)

  useEffect(() => {
    if (importing) return
    setName('')
    setPrivateKey('')
    setPassphrase('')
  }, [importing])

  const refresh = async () => {
    onKeysChange(await ConnectionService.GetManagedKeys() ?? [])
  }

  const handleImport = async () => {
    setSaving(true)
    try {
      const result = await ConnectionService.ImportManagedKey(name, privateKey, passphrase)
      if (!result.success) {
        toast(result.message || 'That does not look like a private key.')
        return
      }
      await refresh()
      setImporting(false)
    } catch (err) {
      toast(handleError(err, 'Could not import key.'))
    } finally {
      setSaving(false)
    }
  }

  const handleChooseFile = async () => {
    try {
      const chosen = await ConnectionService.ChooseKeyFileToImport()
      if (!chosen.privateKey) return
      setPrivateKey(chosen.privateKey)
      if (!name.trim()) setName(chosen.name)
    } catch (err) {
      toast(handleError(err, 'Could not read that key file.'))
    }
  }

  const handleRemove = async () => {
    if (!keyToRemove) return
    try {
      await ConnectionService.DeleteManagedKey(keyToRemove)
      await refresh()
      setKeyToRemove(null)
    } catch (err) {
      toast(handleError(err, 'Could not remove key.'))
    }
  }

  const openRename = (key: ManagedKey) => {
    setNewName(key.name)
    setKeyToRename(key.id)
  }

  const handleRename = async () => {
    if (!keyToRename || !newName.trim()) return
    setRenaming(true)
    try {
      await ConnectionService.RenameManagedKey(keyToRename, newName)
      await refresh()
      setKeyToRename(null)
    } catch (err) {
      toast(handleError(err, 'Could not rename key.'))
    } finally {
      setRenaming(false)
    }
  }

  if (importing) {
    return (
      <div className="space-y-5">
        <div className="space-y-1.5">
          <label htmlFor="key-name" className="text-xs font-medium text-muted-foreground">Name</label>
          <Input
            id="key-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Laptop key"
          />
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label htmlFor="key-text" className="text-xs font-medium text-muted-foreground">Private key</label>
            {canChooseFile && (
              <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={handleChooseFile}>
                Choose a file…
              </Button>
            )}
          </div>
          <textarea
            id="key-text"
            value={privateKey}
            onChange={(e) => setPrivateKey(e.target.value)}
            spellCheck={false}
            autoCapitalize="none"
            autoCorrect="off"
            placeholder={'-----BEGIN OPENSSH PRIVATE KEY-----'}
            className={keyTextClass}
          />
          <p className="text-xs leading-5 text-muted-foreground">
            {canChooseFile ? 'Paste the private key or choose its file.' : 'Paste the private key.'} Its public half must already be in your reMarkable's authorized_keys.
          </p>
        </div>

        <div className="space-y-1.5">
          <label htmlFor="key-passphrase-import" className="text-xs font-medium text-muted-foreground">Passphrase</label>
          <Input
            id="key-passphrase-import"
            type="password"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            placeholder="Leave blank if the key is not encrypted"
          />
        </div>

        <div className={cn(
          'flex flex-col-reverse gap-2 sm:flex-row sm:justify-end',
          '[&>button]:w-full sm:[&>button]:w-auto',
        )}>
          <Button variant="outline" onClick={() => setImporting(false)}>Cancel</Button>
          <Button onClick={handleImport} disabled={saving || !privateKey.trim()}>
            {saving && <CircleNotch className="mr-1.5 h-4 w-4 animate-spin" />}
            Add key
          </Button>
        </div>
      </div>
    )
  }

  if (keys.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex flex-col items-center gap-2 py-6 text-center">
          <Key className="h-7 w-7 text-muted-foreground" aria-hidden />
          <p className="text-sm text-muted-foreground">
            No keys yet. Add one to connect with a key instead of a password.
          </p>
        </div>
        <Button className="w-full touch:hit-target" variant="outline" onClick={() => setImporting(true)}>
          <Plus className="mr-1.5 h-4 w-4" />
          Add a key
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <PanelGroup divided={false}>
        {keys.map(key => (
          <PanelRow key={key.id} label={key.name} detail={keyDetail(key)}>
            <div className="flex gap-2">
              <Button className={panelAction} variant="outline" onClick={() => openRename(key)}>
                Rename
              </Button>
              <Button className={panelAction} variant="outline" onClick={() => setKeyToRemove(key.id)}>
                Remove
              </Button>
            </div>
          </PanelRow>
        ))}
      </PanelGroup>

      <Button className="w-full touch:hit-target" variant="outline" onClick={() => setImporting(true)}>
        <Plus className="mr-1.5 h-4 w-4" />
        Add a key
      </Button>

      <Dialog open={keyToRename !== null} onOpenChange={(open) => !open && setKeyToRename(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename key</DialogTitle>
            <DialogDescription>
              Saved reMarkables that use this key keep using it.
            </DialogDescription>
          </DialogHeader>
          <form
            id="rename-key-form"
            onSubmit={(e) => {
              e.preventDefault()
              handleRename()
            }}
          >
            <Input
              aria-label="Name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              autoFocus
            />
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setKeyToRename(null)}>
              Cancel
            </Button>
            <Button type="submit" form="rename-key-form" disabled={renaming || !newName.trim()}>
              {renaming && <CircleNotch className="mr-1.5 h-4 w-4 animate-spin" />}
              Rename
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={keyToRemove !== null} onOpenChange={(open) => !open && setKeyToRemove(null)}>
        <DialogContent>
          <DialogHeader severity="destructive" icon={Trash}>
            <DialogTitle>Remove "{keys.find(k => k.id === keyToRemove)?.name}"?</DialogTitle>
            <DialogDescription>
              Any saved reMarkable using this key will need another way to sign in.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setKeyToRemove(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRemove}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
