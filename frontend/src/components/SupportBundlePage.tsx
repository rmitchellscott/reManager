import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Copy, Check, Plus, Trash2, RefreshCw, Loader2, AlertCircle, AlertTriangle } from 'lucide-react'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { SupportBundleDialog } from '@/components/SupportBundleDialog'

interface BundleRecord {
  id: string
  remoteId: string
  url: string
  deviceName: string
  deviceId: string
  uploadedAt: number
  lastAppended?: number
  expiresAt?: number
}

interface SupportBundlePageProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  savedDevices: { id: string; name: string }[]
  connectedDeviceId: string
}

function formatDate(ts: number): string {
  return new Date(ts * 1000).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: 'numeric', minute: '2-digit',
  })
}

function expiryText(expiresAt: number): { text: string; expired: boolean } {
  const now = Date.now() / 1000
  const diff = expiresAt - now
  if (diff <= 0) return { text: 'Expired', expired: true }
  const days = Math.ceil(diff / 86400)
  if (days === 1) return { text: 'Expires tomorrow', expired: false }
  return { text: `Expires in ${days} days`, expired: false }
}

export function SupportBundlePage({ open, onOpenChange, isConnected, savedDevices, connectedDeviceId }: SupportBundlePageProps) {
  const [bundles, setBundles] = useState<BundleRecord[]>([])
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [appendingId, setAppendingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [confirmDeleteBundle, setConfirmDeleteBundle] = useState<BundleRecord | null>(null)
  const [showNewBundle, setShowNewBundle] = useState(false)

  const loadBundles = useCallback(async () => {
    const records = await window.go.main.App.GetBundleHistory()
    setBundles(records || [])
  }, [])

  useEffect(() => {
    if (open) {
      loadBundles()
      setConfirmDeleteBundle(null)
    }
  }, [open, loadBundles])

  const handleAppendComplete = useCallback(() => {
    setAppendingId(null)
    loadBundles()
  }, [loadBundles])

  const handleAppendError = useCallback((...args: unknown[]) => {
    setAppendingId(null)
    const data = args[0] as { message: string }
    toast.error(data.message)
  }, [])

  useEffect(() => {
    if (!open) return
    const unsub1 = window.runtime.EventsOn('support-bundle:append-complete', handleAppendComplete)
    const unsub2 = window.runtime.EventsOn('support-bundle:append-error', handleAppendError)
    return () => {
      unsub1()
      unsub2()
    }
  }, [open, handleAppendComplete, handleAppendError])

  const handleCopy = (remoteId: string, url: string) => {
    window.go.main.App.CopyToClipboard(url)
    setCopiedId(remoteId)
    setTimeout(() => setCopiedId(null), 2000)
  }

  const handleAppend = (remoteId: string) => {
    setAppendingId(remoteId)
    window.go.main.App.AppendSupportBundle(remoteId, connectedDeviceId)
  }

  const handleDelete = async (remoteId: string) => {
    setDeletingId(remoteId)
    try {
      await window.go.main.App.DeleteSupportBundle(remoteId)
      setConfirmDeleteBundle(null)
      loadBundles()
    } catch (err) {
      toast.error('Failed to delete bundle: ' + (err instanceof Error ? err.message : String(err)))
    } finally {
      setDeletingId(null)
    }
  }

  const handleNewBundleClose = (v: boolean) => {
    setShowNewBundle(v)
    if (!v) loadBundles()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[80vw] max-w-2xl h-[70vh] max-h-[600px] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle>Support</DialogTitle>
          <DialogDescription>
            Need help? <button onClick={() => window.runtime.BrowserOpenURL('https://github.com/rmitchellscott/reManager/issues')} className="underline hover:text-foreground">Report issues on GitHub</button> or upload a diagnostic bundle for troubleshooting.
            <span className="block text-xs mt-1">Anyone with the link can view the uploaded data.</span>
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto">
          {bundles.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
              <AlertCircle className="h-8 w-8 mb-3" />
              <p className="text-sm">No uploaded bundles</p>
              <p className="text-xs mt-1">Upload a support bundle to share for support</p>
            </div>
          ) : (
            <div className="space-y-3">
              {bundles.map((bundle) => {
                const expiry = bundle.expiresAt ? expiryText(bundle.expiresAt) : null
                const isExpired = expiry?.expired ?? false

                return (
                  <div key={bundle.remoteId} className="border rounded-lg p-3 space-y-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium truncate">
                            {bundle.deviceName || 'Unknown device'}
                          </span>
                          {expiry && (
                            <span className={`text-xs ${isExpired ? 'text-destructive' : 'text-muted-foreground'}`}>
                              {expiry.text}
                            </span>
                          )}
                        </div>
                        <div className="text-xs text-muted-foreground mt-0.5">
                          Uploaded {formatDate(bundle.uploadedAt)}
                          {bundle.lastAppended ? ` · Updated ${formatDate(bundle.lastAppended)}` : ''}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <code className="text-xs bg-muted px-2 py-1 rounded break-all flex-1 text-muted-foreground">
                        {bundle.url}
                      </code>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 flex-shrink-0"
                            onClick={() => handleCopy(bundle.remoteId, bundle.url)}
                          >
                            {copiedId === bundle.remoteId ? (
                              <Check className="h-3.5 w-3.5" />
                            ) : (
                              <Copy className="h-3.5 w-3.5" />
                            )}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Copy URL</TooltipContent>
                      </Tooltip>
                    </div>
                    <div className="flex items-center justify-end gap-2 pt-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            size="sm"
                            onClick={() => handleAppend(bundle.remoteId)}
                            disabled={isExpired || appendingId === bundle.remoteId}
                          >
                            {appendingId === bundle.remoteId ? (
                              <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                            ) : (
                              <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
                            )}
                            Update
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Send an updated bundle to the server</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setConfirmDeleteBundle(bundle)}
                            disabled={isExpired}
                          >
                            <Trash2 className="h-3.5 w-3.5 mr-1.5" />
                            Delete
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Delete this bundle from the server</TooltipContent>
                      </Tooltip>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        <DialogFooter className="flex-shrink-0">
          <Button variant="outline" onClick={() => onOpenChange(false)} className="mr-auto">
            Close
          </Button>
          <Button variant={bundles.length > 0 ? 'outline' : 'default'} onClick={() => setShowNewBundle(true)}>
            <Plus className="h-4 w-4 mr-1.5" />
            New Bundle
          </Button>
        </DialogFooter>
      </DialogContent>

      <Dialog open={confirmDeleteBundle !== null} onOpenChange={(v) => { if (!v) setConfirmDeleteBundle(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-destructive" />
              Delete Bundle
            </DialogTitle>
            <DialogDescription>
              This will permanently delete this support bundle from the server.
            </DialogDescription>
          </DialogHeader>
          {confirmDeleteBundle && (
            <div className="text-sm text-muted-foreground">
              <p><span className="font-medium text-foreground">{confirmDeleteBundle.deviceName || 'Unknown device'}</span></p>
              <p className="text-xs mt-1">Uploaded {formatDate(confirmDeleteBundle.uploadedAt)}</p>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDeleteBundle(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => confirmDeleteBundle && handleDelete(confirmDeleteBundle.remoteId)}
              disabled={deletingId === confirmDeleteBundle?.remoteId}
            >
              {deletingId === confirmDeleteBundle?.remoteId ? (
                <Loader2 className="h-4 w-4 animate-spin mr-1.5" />
              ) : null}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SupportBundleDialog
        open={showNewBundle}
        onOpenChange={handleNewBundleClose}
        isConnected={isConnected}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
      />
    </Dialog>
  )
}
