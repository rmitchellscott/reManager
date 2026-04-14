import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Copy, Check, Plus, Trash2, RefreshCw, Loader2, AlertCircle, AlertTriangle, ChevronRight, ChevronLeft, CheckCircle2, MessageSquare, Package, Info } from 'lucide-react'
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

type Screen = 'main' | 'report' | 'report-sent' | 'chat' | 'bundles'

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
  const [screen, setScreen] = useState<Screen>('main')
  const [bundles, setBundles] = useState<BundleRecord[]>([])
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [appendingId, setAppendingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [confirmDeleteBundle, setConfirmDeleteBundle] = useState<BundleRecord | null>(null)
  const [showNewBundle, setShowNewBundle] = useState(false)
  const [bundlesExpanded, setBundlesExpanded] = useState(false)


  const [reportDescription, setReportDescription] = useState('')
  const [reportEmail, setReportEmail] = useState('')
  const [reportBundleURL, setReportBundleURL] = useState('')
  const [reportSubmitting, setReportSubmitting] = useState(false)
  const [showReportBundleDialog, setShowReportBundleDialog] = useState(false)


  const [showChatBundleDialog, setShowChatBundleDialog] = useState(false)
  const [chatBundleURL, setChatBundleURL] = useState('')

  const loadBundles = useCallback(async () => {
    const records = await window.go.main.App.GetBundleHistory()
    setBundles(records || [])
  }, [])

  useEffect(() => {
    if (open) {
      loadBundles()
      setScreen('main')
      setConfirmDeleteBundle(null)
      setBundlesExpanded(false)
      setReportDescription('')
      setReportEmail('')
      setReportBundleURL('')
      setChatBundleURL('')
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


  const handleReportBundleComplete = useCallback((...args: unknown[]) => {
    const data = args[0] as { url: string }
    setReportBundleURL(data.url)
    setShowReportBundleDialog(false)
    loadBundles()
  }, [loadBundles])

  useEffect(() => {
    if (!showReportBundleDialog) return
    const unsub = window.runtime.EventsOn('support-bundle:upload-complete', handleReportBundleComplete)
    return () => { unsub() }
  }, [showReportBundleDialog, handleReportBundleComplete])

  const handleSubmitReport = async () => {
    if (!reportDescription.trim()) return
    setReportSubmitting(true)
    try {
      await window.go.main.App.SubmitProblemReport(
        reportDescription.trim(),
        reportEmail.trim(),
        reportBundleURL,
      )
      setScreen('report-sent')
    } catch (err) {
      toast.error('Failed to submit report: ' + (err instanceof Error ? err.message : String(err)))
    } finally {
      setReportSubmitting(false)
    }
  }


  const handleChatBundleComplete = useCallback((...args: unknown[]) => {
    const data = args[0] as { url: string }
    setChatBundleURL(data.url)
    setShowChatBundleDialog(false)
    loadBundles()
  }, [loadBundles])

  useEffect(() => {
    if (!showChatBundleDialog) return
    const unsub = window.runtime.EventsOn('support-bundle:upload-complete', handleChatBundleComplete)
    return () => { unsub() }
  }, [showChatBundleDialog, handleChatBundleComplete])

  const handleOpenDiscord = () => {
    if (chatBundleURL) {
      window.go.main.App.CopyToClipboard(chatBundleURL)
      toast.success('Bundle URL copied to clipboard')
    }
    window.runtime.BrowserOpenURL('https://discord.gg/remanager')
  }

  const bundleList = (
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
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[80vw] max-w-2xl max-h-[80vh] flex flex-col">

        {screen === 'main' && (
          <>
            <DialogHeader className="flex-shrink-0">
              <DialogTitle>Support</DialogTitle>
              <DialogDescription>
                Get help, report issues, or connect with the community.
              </DialogDescription>
            </DialogHeader>

            <div className="flex-1 overflow-y-auto">
              <div className="grid grid-cols-3 gap-3">
                <button
                  className="border rounded-lg py-5 px-4 flex flex-col items-center text-center gap-3 hover:bg-accent hover:border-ring transition-colors"
                  onClick={() => setScreen('report')}
                >
                  <Info className="h-8 w-8 text-muted-foreground" />
                  <span className="text-sm font-medium">Report a Problem</span>
                  <span className="text-xs text-muted-foreground">Let the developer know what went wrong</span>
                </button>

                <button
                  className="border rounded-lg py-5 px-4 flex flex-col items-center text-center gap-3 hover:bg-accent hover:border-ring transition-colors"
                  onClick={() => setScreen('chat')}
                >
                  <MessageSquare className="h-8 w-8 text-muted-foreground" />
                  <span className="text-sm font-medium">Community Chat</span>
                  <span className="text-xs text-muted-foreground">Get help from other users</span>
                </button>

                <button
                  className="border rounded-lg py-5 px-4 flex flex-col items-center text-center gap-3 hover:bg-accent hover:border-ring transition-colors"
                  onClick={() => setScreen('bundles')}
                >
                  <Package className="h-8 w-8 text-muted-foreground" />
                  <span className="text-sm font-medium">Support Bundles</span>
                  <span className="text-xs text-muted-foreground">Generate & manage diagnostics</span>
                </button>
              </div>

              {bundles.length > 0 && (
                <div className="border-t mt-4 pt-3">
                  <button
                    className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
                    onClick={() => setBundlesExpanded(!bundlesExpanded)}
                  >
                    <ChevronRight className={`h-4 w-4 transition-transform ${bundlesExpanded ? 'rotate-90' : ''}`} />
                    Uploaded Bundles ({bundles.length})
                  </button>
                  {bundlesExpanded && <div className="mt-3">{bundleList}</div>}
                </div>
              )}
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => onOpenChange(false)} className="mr-auto">
                Close
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'report' && (
          <>
            <DialogHeader className="flex-shrink-0">
              <button
                className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors mb-3 self-start"
                onClick={() => setScreen('main')}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                Back
              </button>
              <DialogTitle>Report a Problem</DialogTitle>
              <DialogDescription>
                Describe what happened so the developer can look into it.
              </DialogDescription>
            </DialogHeader>

            <div className="flex-1 overflow-y-auto space-y-5 px-1">
              <div className="space-y-2">
                <label className="text-[0.8125rem] font-medium">What went wrong?</label>
                <textarea
                  className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 min-h-[6rem] resize-y"
                  placeholder="Describe the issue you experienced..."
                  value={reportDescription}
                  onChange={(e) => setReportDescription(e.target.value)}
                />
              </div>

              <div className="space-y-2">
                <label className="text-[0.8125rem] font-medium">
                  Email <span className="font-normal text-muted-foreground">(optional)</span>
                </label>
                <Input
                  type="email"
                  placeholder="your@email.com"
                  value={reportEmail}
                  onChange={(e) => setReportEmail(e.target.value)}
                />
              </div>

              <div className="border rounded-lg p-3.5 bg-muted/50 flex items-start gap-3">
                <Package className="h-5 w-5 text-muted-foreground flex-shrink-0 mt-0.5" />
                <div className="flex-1 min-w-0">
                  <div className="text-[0.8125rem] font-medium">Include a diagnostic bundle?</div>
                  <div className="text-xs text-muted-foreground">Helps investigate the issue faster. No personal data is included.</div>
                </div>
                {reportBundleURL ? (
                  <div className="flex items-center gap-1.5 flex-shrink-0">
                    <CheckCircle2 className="h-4 w-4 text-green-600" />
                    <span className="text-xs text-muted-foreground">Attached</span>
                  </div>
                ) : (
                  <Button variant="outline" size="sm" className="flex-shrink-0" onClick={() => setShowReportBundleDialog(true)}>
                    Attach Bundle
                  </Button>
                )}
              </div>
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => setScreen('main')}>
                Cancel
              </Button>
              <Button
                onClick={handleSubmitReport}
                disabled={!reportDescription.trim() || reportSubmitting}
              >
                {reportSubmitting && <Loader2 className="h-4 w-4 animate-spin mr-1.5" />}
                Submit Report
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'report-sent' && (
          <>
            <DialogHeader className="flex-shrink-0">
              <DialogTitle>Report a Problem</DialogTitle>
              <DialogDescription>Your report has been submitted.</DialogDescription>
            </DialogHeader>

            <div className="flex-1 flex items-center gap-3 py-4">
              <CheckCircle2 className="h-6 w-6 text-green-600 flex-shrink-0" />
              <div>
                <div className="text-sm font-medium">Thank you for your report</div>
                <div className="text-xs text-muted-foreground mt-1">
                  Your report has been received. If you provided an email, you may be contacted for follow-up.
                </div>
              </div>
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button onClick={() => { setScreen('main'); setReportDescription(''); setReportEmail(''); setReportBundleURL('') }}>
                Done
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'chat' && (
          <>
            <DialogHeader className="flex-shrink-0">
              <button
                className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors mb-3 self-start"
                onClick={() => setScreen('main')}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                Back
              </button>
              <DialogTitle>Community Chat</DialogTitle>
              <DialogDescription>
                Join the Discord to get help from other users and the developer.
              </DialogDescription>
            </DialogHeader>

            <div className="flex-1 overflow-y-auto space-y-4">
              <div className="border rounded-lg p-3.5 bg-muted/50 flex items-start gap-3">
                <Package className="h-5 w-5 text-muted-foreground flex-shrink-0 mt-0.5" />
                <div className="flex-1 min-w-0">
                  <div className="text-[0.8125rem] font-medium">Generate a support bundle first?</div>
                  <div className="text-xs text-muted-foreground">You can share the link in Discord so others can help diagnose your issue.</div>
                </div>
                {chatBundleURL ? (
                  <div className="flex items-center gap-1.5 flex-shrink-0">
                    <CheckCircle2 className="h-4 w-4 text-green-600" />
                    <span className="text-xs text-muted-foreground">Ready</span>
                  </div>
                ) : (
                  <Button variant="outline" size="sm" className="flex-shrink-0" onClick={() => setShowChatBundleDialog(true)}>
                    Generate
                  </Button>
                )}
              </div>

              <p className="text-xs text-muted-foreground">
                {chatBundleURL
                  ? 'The bundle URL will be copied to your clipboard when you open Discord.'
                  : "You'll be taken to Discord in your browser."
                }
              </p>
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => setScreen('main')}>
                Cancel
              </Button>
              <Button onClick={handleOpenDiscord}>
                Open Discord
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'bundles' && (
          <>
            <DialogHeader className="flex-shrink-0">
              <button
                className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors mb-3 self-start"
                onClick={() => setScreen('main')}
              >
                <ChevronLeft className="h-3.5 w-3.5" />
                Back
              </button>
              <DialogTitle>Support Bundles</DialogTitle>
              <DialogDescription>
                Generate and share diagnostic data for troubleshooting.
                <span className="block text-xs mt-1">Anyone with the link can view the uploaded data.</span>
              </DialogDescription>
            </DialogHeader>

            <div className="flex-1 overflow-y-auto">
              {bundles.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-muted-foreground py-8">
                  <AlertCircle className="h-8 w-8 mb-3" />
                  <p className="text-sm">No uploaded bundles</p>
                  <p className="text-xs mt-1">Upload a support bundle to share for support</p>
                </div>
              ) : bundleList}
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => setScreen('main')} className="mr-auto">
                Close
              </Button>
              <Button onClick={() => setShowNewBundle(true)}>
                <Plus className="h-4 w-4 mr-1.5" />
                New Bundle
              </Button>
            </DialogFooter>
          </>
        )}
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

      <SupportBundleDialog
        open={showReportBundleDialog}
        onOpenChange={(v) => { if (!v) setShowReportBundleDialog(false) }}
        isConnected={isConnected}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
      />

      <SupportBundleDialog
        open={showChatBundleDialog}
        onOpenChange={(v) => { if (!v) setShowChatBundleDialog(false) }}
        isConnected={isConnected}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
      />
    </Dialog>
  )
}
