import { ArrowsClockwise, Bug, Chat, Check, CheckCircle, CircleNotch, Copy, Package, Trash, Warning, WarningCircle } from '@phosphor-icons/react'
import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { Label } from '@/components/ui/label'
import { SupportBundleDialog } from '@/components/SupportBundleDialog'
import { formatDate } from '@/lib/format'
import { errorDetail, handleError } from '@/lib/errorMessages'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import type { SupportScreen } from '@/components/SupportHome'
import { SupportService, SystemService } from '@bindings/reManager'
import type { BundleRecord } from '@/lib/types'
import { operationFailure, runOperation } from '@/lib/operations'

interface SupportBundlePageProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialScreen: SupportScreen
  isConnected: boolean
  savedDevices: { id: string; name: string }[]
  connectedDeviceId: string
  supportUpload: boolean
}

type Screen = SupportScreen | 'report-sent'

function expiryText(expiresAt: number): { text: string; expired: boolean } {
  const now = Date.now() / 1000
  const diff = expiresAt - now
  if (diff <= 0) return { text: 'Expired', expired: true }
  const days = Math.ceil(diff / 86400)
  if (days === 1) return { text: 'Expires tomorrow', expired: false }
  return { text: `Expires in ${days} days`, expired: false }
}

export function SupportBundlePage({ open, onOpenChange, initialScreen, isConnected, savedDevices, connectedDeviceId, supportUpload }: SupportBundlePageProps) {
  const [screen, setScreen] = useState<Screen>(initialScreen)
  // Synced during render, not an effect: an effect lands after the first paint, so the dialog would show the previous screen for a frame and visibly resize.
  const [wasOpen, setWasOpen] = useState(open)
  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setScreen(initialScreen)
  }
  const [bundles, setBundles] = useState<BundleRecord[]>([])
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [appendingId, setAppendingId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<{ message: string; detail: string } | null>(null)
  const [appendError, setAppendError] = useState<{ remoteId: string; message: string; detail: string } | null>(null)
  const [confirmDeleteBundle, setConfirmDeleteBundle] = useState<BundleRecord | null>(null)
  const [showNewBundle, setShowNewBundle] = useState(false)



  const [reportDescription, setReportDescription] = useState('')
  const [reportEmail, setReportEmail] = useState('')
  const [reportBundleURL, setReportBundleURL] = useState('')
  const [reportDeviceId, setReportDeviceId] = useState('')
  const [reportSubmitting, setReportSubmitting] = useState(false)
  const [reportError, setReportError] = useState<{ message: string; detail: string } | null>(null)
  const [showReportBundleDialog, setShowReportBundleDialog] = useState(false)


  const [showChatBundleDialog, setShowChatBundleDialog] = useState(false)
  const [chatBundleURL, setChatBundleURL] = useState('')

  const loadBundles = useCallback(async () => {
    const records = await SupportService.GetBundleHistory()
    setBundles(records || [])
  }, [])

  useEffect(() => {
    if (open) {
      loadBundles()
      setConfirmDeleteBundle(null)
      setReportDescription('')
      setReportEmail('')
      setReportBundleURL('')
      setReportDeviceId(connectedDeviceId || '')
      setChatBundleURL('')
    }
  }, [open, initialScreen, loadBundles])

  const handleCopy = (remoteId: string, url: string) => {
    SupportService.CopyToClipboard(url)
    setCopiedId(remoteId)
    setTimeout(() => setCopiedId(null), 2000)
  }

  const handleAppend = async (remoteId: string) => {
    setAppendingId(remoteId)
    const result = await runOperation(id => SupportService.AppendSupportBundle(id, remoteId, connectedDeviceId))
    setAppendingId(null)
    if (result.ok) {
      loadBundles()
      toast.success('Bundle updated')
      return
    }
    if (result.canceled) return
    const failure = operationFailure(result)
    setAppendError({ remoteId, message: handleError(failure, 'Could not update this bundle.'), detail: errorDetail(failure) })
  }

  const handleDelete = async (remoteId: string) => {
    setDeletingId(remoteId)
    try {
      setDeleteError(null)
      await SupportService.DeleteSupportBundle(remoteId)
      setConfirmDeleteBundle(null)
      loadBundles()
    } catch (err) {
      setDeleteError({ message: handleError(err, 'Could not delete the bundle.'), detail: errorDetail(err) })
    } finally {
      setDeletingId(null)
    }
  }

  const handleNewBundleClose = (v: boolean) => {
    setShowNewBundle(v)
    if (!v) loadBundles()
  }


  const handleReportBundleUploaded = (url: string) => {
    setReportBundleURL(url)
    setShowReportBundleDialog(false)
    loadBundles()
  }

  const handleSubmitReport = async () => {
    if (!reportDescription.trim()) return
    setReportSubmitting(true)
    try {
      await SupportService.SubmitProblemReport(
        reportDescription.trim(),
        reportEmail.trim(),
        reportBundleURL,
        reportDeviceId,
      )
      setReportError(null)
      setScreen('report-sent')
    } catch (err) {
      setReportError({ message: handleError(err, 'Could not send the report.'), detail: errorDetail(err) })
    } finally {
      setReportSubmitting(false)
    }
  }


  const handleChatBundleUploaded = (url: string) => {
    setChatBundleURL(url)
    setShowChatBundleDialog(false)
    loadBundles()
  }

  const handleOpenDiscord = () => {
    if (chatBundleURL) {
      SupportService.CopyToClipboard(chatBundleURL)
      toast.success('Bundle link copied')
    }
    SystemService.OpenExternalURL('https://discord.com/invite/u3P9sDW')
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
                    aria-label="Copy link"
                  >
                    {copiedId === bundle.remoteId ? (
                      <Check className="h-3.5 w-3.5" />
                    ) : (
                      <Copy className="h-3.5 w-3.5" />
                    )}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Copy link</TooltipContent>
              </Tooltip>
            </div>
            {appendError?.remoteId === bundle.remoteId && (
              <>
                <p className="pt-1 text-xs text-destructive">{appendError.message}</p>
                <OutputDisclosure output={appendError.detail} />
              </>
            )}
            <div className="flex items-center justify-end gap-2 pt-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    onClick={() => { setAppendError(null); handleAppend(bundle.remoteId) }}
                    disabled={isExpired || appendingId === bundle.remoteId}
                  >
                    {appendingId === bundle.remoteId ? (
                      <CircleNotch className="h-3.5 w-3.5 animate-spin mr-1.5" />
                    ) : (
                      <ArrowsClockwise className="h-3.5 w-3.5 mr-1.5" />
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
                    <Trash className="h-3.5 w-3.5 mr-1.5" />
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
      <DialogContent size="lg">

        {screen === 'report' && (
          <>
            <DialogHeader severity="neutral" icon={Bug} className="flex-shrink-0">
              <DialogTitle>Report a problem</DialogTitle>
              <DialogDescription>
                Describe what happened so the developer can look into it.
              </DialogDescription>
            </DialogHeader>

            <DialogBody className="space-y-4">
              <div>
                <Label>What went wrong?</Label>
                <textarea
                  className="mt-2 flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm touch:no-focus-zoom ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 min-h-[6rem] resize-y"
                  placeholder="Describe the issue…"
                  value={reportDescription}
                  onChange={(e) => setReportDescription(e.target.value)}
                />
              </div>

              <div>
                <Label>
                  Email <span className="font-normal text-muted-foreground">(optional)</span>
                </Label>
                <Input
                  className="mt-2"
                  type="email"
                  placeholder="your@email.com"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  value={reportEmail}
                  onChange={(e) => setReportEmail(e.target.value)}
                />
                <div className="text-xs text-muted-foreground mt-1">
                  The developer cannot reply without an address.
                </div>
              </div>

              {savedDevices.length > 0 && (
                <div>
                  <Label>
                    reMarkable <span className="font-normal text-muted-foreground">(optional)</span>
                  </Label>
                  <Select value={reportDeviceId} onValueChange={setReportDeviceId}>
                    <SelectTrigger className="mt-2 w-full">
                      <SelectValue placeholder="Select a reMarkable" />
                    </SelectTrigger>
                    <SelectContent>
                      {savedDevices.map(d => (
                        <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              <div className="border rounded-lg p-3.5 bg-muted/50 flex flex-col gap-3 sm:flex-row sm:items-center">
                <div className="flex items-center gap-3 sm:flex-1 sm:min-w-0">
                  <Package className="h-5 w-5 text-muted-foreground flex-shrink-0" />
                  <div className="min-w-0">
                    <div className="text-sm font-medium">Include a diagnostic bundle?</div>
                    <div className="text-xs text-muted-foreground">Helps investigate the issue faster. No personal data is included.</div>
                  </div>
                </div>
                {reportBundleURL ? (
                  <div className="flex items-center justify-center gap-1.5 sm:flex-shrink-0">
                    <CheckCircle className="h-4 w-4 text-green-600" />
                    <span className="text-xs text-muted-foreground">Attached</span>
                  </div>
                ) : (
                  <Button variant="outline" size="sm" className="w-full sm:w-auto sm:flex-shrink-0" onClick={() => setShowReportBundleDialog(true)}>
                    Attach bundle
                  </Button>
                )}
              </div>
            </DialogBody>

            {reportError && (
              <Alert variant="destructive" className="flex-shrink-0">
                <WarningCircle className="h-4 w-4" />
                <AlertDescription>{reportError.message}</AlertDescription>
              </Alert>
            )}
            {reportError && <OutputDisclosure output={reportError.detail} className="flex-shrink-0" />}
            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                onClick={handleSubmitReport}
                disabled={!reportDescription.trim() || reportSubmitting}
              >
                {reportSubmitting && <CircleNotch className="h-4 w-4 animate-spin mr-1.5" />}
                Send report
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'report-sent' && (
          <>
            <DialogHeader severity="success" icon={CheckCircle} className="flex-shrink-0">
              <DialogTitle>Report a problem</DialogTitle>
              <DialogDescription>Your report was sent.</DialogDescription>
            </DialogHeader>

            <div className="flex-1">
              <div>
                <div className="text-sm font-medium">Thank you for your report</div>
                <div className="text-xs text-muted-foreground mt-1">
                  If you included an email address, the developer may follow up.
                </div>
              </div>
            </div>

            <DialogFooter className="flex-shrink-0">
              <Button onClick={() => onOpenChange(false)}>
                Done
              </Button>
            </DialogFooter>
          </>
        )}

        {screen === 'chat' && (
          <>
            <DialogHeader severity="neutral" icon={Chat} className="flex-shrink-0">
              <DialogTitle>Community chat</DialogTitle>
              <DialogDescription>
                Join the Discord to get help from other users and the developer.
              </DialogDescription>
            </DialogHeader>

            <DialogBody className="space-y-4">
              {supportUpload && (
                <div className="border rounded-lg p-3.5 bg-muted/50 flex flex-col gap-3 sm:flex-row sm:items-center">
                  <div className="flex items-center gap-3 sm:flex-1 sm:min-w-0">
                    <Package className="h-5 w-5 text-muted-foreground flex-shrink-0" />
                    <div className="min-w-0">
                      <div className="text-sm font-medium">Generate a support bundle first?</div>
                      <div className="text-xs text-muted-foreground">You can share the link in Discord so others can help diagnose your issue.</div>
                    </div>
                  </div>
                  {chatBundleURL ? (
                    <div className="flex items-center justify-center gap-1.5 sm:flex-shrink-0">
                      <CheckCircle className="h-4 w-4 text-green-600" />
                      <span className="text-xs text-muted-foreground">Ready</span>
                    </div>
                  ) : (
                    <Button variant="outline" size="sm" className="w-full sm:w-auto sm:flex-shrink-0" onClick={() => setShowChatBundleDialog(true)}>
                      Generate
                    </Button>
                  )}
                </div>
              )}

              <p className="text-xs text-muted-foreground">
                {chatBundleURL
                  ? <>The bundle link is copied to your clipboard when you open Discord. Open a thread in the <a href="https://discord.com/channels/385916768696139794/1180628656851275938" className="underline hover:text-foreground" onClick={(e) => { e.preventDefault(); SystemService.OpenExternalURL('https://discord.com/channels/385916768696139794/1180628656851275938') }}>#support channel</a> and paste it there.</>
                  : <>You'll be taken to Discord in your browser. Open a thread in the <a href="https://discord.com/channels/385916768696139794/1180628656851275938" className="underline hover:text-foreground" onClick={(e) => { e.preventDefault(); SystemService.OpenExternalURL('https://discord.com/channels/385916768696139794/1180628656851275938') }}>#support channel</a> to get help.</>
                }
              </p>
            </DialogBody>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
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
              <DialogTitle>Support bundles</DialogTitle>
              <DialogDescription>
                Generate and share diagnostic data for troubleshooting.
                {supportUpload && <span className="block text-xs mt-1">Anyone with the link can view the uploaded data.</span>}
              </DialogDescription>
            </DialogHeader>

            <DialogBody>
              {!supportUpload ? (
                <div className="flex flex-col items-center justify-center h-full text-muted-foreground py-8">
                  <Package className="h-8 w-8 mb-3" />
                  <p className="text-sm">This build of reManager cannot upload bundles</p>
                  <p className="text-xs mt-1">Save a bundle as a file and share it where you ask for help.</p>
                </div>
              ) : bundles.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-muted-foreground py-8">
                  <WarningCircle className="h-8 w-8 mb-3" />
                  <p className="text-sm">No uploaded bundles</p>
                  <p className="text-xs mt-1">Upload a bundle to share it with support.</p>
                </div>
              ) : bundleList}
            </DialogBody>

            <DialogFooter className="flex-shrink-0">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={() => setShowNewBundle(true)}>
                New bundle
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>

      <Dialog open={confirmDeleteBundle !== null} onOpenChange={(v) => { if (!v) setConfirmDeleteBundle(null) }}>
        <DialogContent size="sm">
          <DialogHeader severity="destructive" icon={Warning}>
            <DialogTitle>Delete bundle</DialogTitle>
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
          {deleteError && (
            <Alert variant="destructive">
              <WarningCircle className="h-4 w-4" />
              <AlertDescription>{deleteError.message}</AlertDescription>
            </Alert>
          )}
          {deleteError && <OutputDisclosure output={deleteError.detail} />}
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
                <CircleNotch className="h-4 w-4 animate-spin mr-1.5" />
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
        supportUpload={supportUpload}
      />

      <SupportBundleDialog
        open={showReportBundleDialog}
        onOpenChange={(v) => { if (!v) setShowReportBundleDialog(false) }}
        isConnected={isConnected || !!reportDeviceId}
        savedDevices={savedDevices}
        connectedDeviceId={reportDeviceId || connectedDeviceId}
        supportUpload={supportUpload}
        onUploaded={handleReportBundleUploaded}
      />

      <SupportBundleDialog
        open={showChatBundleDialog}
        onOpenChange={(v) => { if (!v) setShowChatBundleDialog(false) }}
        isConnected={isConnected}
        savedDevices={savedDevices}
        connectedDeviceId={connectedDeviceId}
        supportUpload={supportUpload}
        onUploaded={handleChatBundleUploaded}
      />
    </Dialog>
  )
}
