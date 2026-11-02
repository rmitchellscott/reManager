import { Check, CheckCircle, Copy, Package, ShieldCheck, ShieldSlash, Warning, WarningCircle } from '@phosphor-icons/react'
import { useState, useEffect } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SupportService } from '@bindings/reManager'
import { runOperation } from '@/lib/operations'
interface SupportBundleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  savedDevices: { id: string; name: string }[]
  connectedDeviceId: string
  supportUpload: boolean
  onUploaded?: (url: string) => void
}

type Stage = 'consent' | 'progress' | 'complete' | 'upload-complete' | 'error'

export function SupportBundleDialog({ open, onOpenChange, isConnected, savedDevices, connectedDeviceId, supportUpload, onUploaded }: SupportBundleDialogProps) {
  const [stage, setStage] = useState<Stage>('consent')
  const [progressMessage, setProgressMessage] = useState('')
  const [completePath, setCompletePath] = useState('')
  const [uploadURL, setUploadURL] = useState('')
  const [uploadSavedOnDevice, setUploadSavedOnDevice] = useState(true)
  const [errorMessage, setErrorMessage] = useState('')
  const [preview, setPreview] = useState<{ included: string[]; excluded: string[] } | null>(null)
  const [selectedDeviceId, setSelectedDeviceId] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (open) {
      setStage('consent')
      setProgressMessage('')
      setCompletePath('')
      setUploadURL('')
      setErrorMessage('')
      setCopied(false)
      setSelectedDeviceId(connectedDeviceId || '')
      SupportService.GetSupportBundlePreview().then(p => setPreview(p ? { included: p.included ?? [], excluded: p.excluded ?? [] } : null))
    }
  }, [open, connectedDeviceId])

  const runBundle = (start: (id: string) => Promise<unknown>) => {
    setStage('progress')
    setProgressMessage('Preparing…')
    return runOperation(start, { onProgress: progress => setProgressMessage(progress.message) })
  }

  const handleGenerate = async () => {
    const result = await runBundle(id => SupportService.GenerateSupportBundle(id, selectedDeviceId))
    if (result.ok && result.supportBundle) {
      setStage('complete')
      setCompletePath(result.supportBundle.path ?? '')
    } else if (result.canceled) {
      setStage('consent')
    } else {
      setStage('error')
      setErrorMessage(result.error ?? '')
    }
  }

  const handleUpload = async () => {
    const result = await runBundle(id => SupportService.UploadSupportBundle(id, selectedDeviceId))
    if (result.ok && result.supportBundle) {
      const url = result.supportBundle.url ?? ''
      setStage('upload-complete')
      setUploadURL(url)
      setUploadSavedOnDevice(result.supportBundle.savedOnDevice)
      onUploaded?.(url)
    } else {
      setStage('error')
      setErrorMessage(result.error ?? '')
    }
  }

  const handleCopyURL = () => {
    SupportService.CopyToClipboard(uploadURL)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const connectedDeviceName = savedDevices.find(d => d.id === connectedDeviceId)?.name
  const isSelectedDeviceConnected = isConnected && selectedDeviceId === connectedDeviceId

  const header = stage === 'error'
    ? { severity: 'destructive' as const, icon: WarningCircle }
    : stage === 'complete' || stage === 'upload-complete'
      ? { severity: 'success' as const, icon: CheckCircle }
      : stage === 'consent'
        ? { severity: 'warning' as const, icon: Warning }
        : { severity: 'neutral' as const, icon: Package }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v || stage !== 'progress') onOpenChange(v) }}>
      <DialogContent>
        <DialogHeader severity={header.severity} icon={header.icon}>
          <DialogTitle>Support bundle</DialogTitle>
          <DialogDescription>
            {stage === 'consent' && 'Review what will be included in the diagnostic bundle.'}
            {stage === 'progress' && 'Generating support bundle…'}
            {stage === 'complete' && 'Support bundle saved.'}
            {stage === 'upload-complete' && 'Support bundle uploaded.'}
            {stage === 'error' && 'Could not generate the support bundle.'}
          </DialogDescription>
        </DialogHeader>

        {stage === 'consent' && preview && (
          <DialogBody className="space-y-4">
            {isConnected && connectedDeviceName ? (
              <div className="text-sm">
                Bundle for <span className="font-medium">{connectedDeviceName}</span>
              </div>
            ) : savedDevices.length > 0 ? (
              <div className="space-y-2">
                <label className="text-sm font-medium">reMarkable</label>
                <Select value={selectedDeviceId} onValueChange={setSelectedDeviceId}>
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Select a reMarkable" />
                  </SelectTrigger>
                  <SelectContent>
                    {savedDevices.map(d => (
                      <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedDeviceId && !isSelectedDeviceConnected && (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <WarningCircle className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Not connected to this reMarkable. The bundle will include cached data, which may be out of date.</span>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <WarningCircle className="h-4 w-4 flex-shrink-0" />
                <span>No saved reMarkables. The bundle will include app logs only.</span>
              </div>
            )}

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <div className="flex items-center gap-1.5 mb-2">
                  <ShieldCheck className="h-4 w-4 text-green-600" />
                  <span className="text-sm font-medium">Included</span>
                </div>
                <ul className="list-disc pl-4 space-y-1">
                  {preview.included.map((item) => (
                    <li key={item} className="text-xs text-muted-foreground">{item}</li>
                  ))}
                </ul>
              </div>
              <div>
                <div className="flex items-center gap-1.5 mb-2">
                  <ShieldSlash className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm font-medium">Not included</span>
                </div>
                <ul className="list-disc pl-4 space-y-1">
                  {preview.excluded.map((item) => (
                    <li key={item} className="text-xs text-muted-foreground">{item}</li>
                  ))}
                </ul>
              </div>
            </div>
            {supportUpload && (
              <div className="text-xs text-muted-foreground space-y-0.5">
                <p>Uploading creates a link you can share with support.</p>
                <p>Uploaded data is automatically deleted after 30 days, or you can delete it at any time.</p>
              </div>
            )}
          </DialogBody>
        )}

        {stage === 'progress' && (
          <div className="space-y-3">
            <Progress indeterminate className="h-1 bg-muted" indicatorClassName="bg-primary" aria-label="Generating support bundle" />
            <p className="text-sm text-muted-foreground">{progressMessage}</p>
          </div>
        )}

        {stage === 'complete' && (
          <div className="text-sm">
            <p>Bundle saved to:</p>
            <p className="text-muted-foreground break-all">{completePath}</p>
          </div>
        )}

        {stage === 'upload-complete' && (
          <div className="space-y-3">
            <span className="text-sm">Share this link with support:</span>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-xs bg-muted px-3 py-2 rounded break-all">{uploadURL}</code>
              <Button variant="outline" size="icon" className="flex-shrink-0" onClick={handleCopyURL} aria-label="Copy link">
                {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {uploadSavedOnDevice
                ? 'You can view, update, or delete this bundle in Settings › Get help.'
                : 'reManager could not save this bundle, so you cannot update or delete it from here.'}
            </p>
          </div>
        )}

        {stage === 'error' && (
          <p className="text-sm text-destructive">{errorMessage}</p>
        )}

        <DialogFooter separateExit={stage === 'consent'}>
          {stage === 'consent' && (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button variant={supportUpload ? 'outline' : 'default'} onClick={handleGenerate}>
                Save locally
              </Button>
              {supportUpload && (
                <Button onClick={handleUpload}>
                  Upload
                </Button>
              )}
            </>
          )}
          {(stage === 'complete' || stage === 'upload-complete') && (
            <Button onClick={() => onOpenChange(false)}>
              Close
            </Button>
          )}
          {stage === 'error' && (
            <>
              <Button variant="outline" onClick={() => setStage('consent')}>
                Try again
              </Button>
              <Button onClick={() => onOpenChange(false)}>
                Close
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
