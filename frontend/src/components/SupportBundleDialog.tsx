import { useState, useEffect, useCallback } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Loader2, CheckCircle2, ShieldCheck, ShieldOff, AlertCircle, Copy, Check } from 'lucide-react'

interface SupportBundleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  savedDevices: { id: string; name: string }[]
  connectedDeviceId: string
}

type Stage = 'consent' | 'progress' | 'complete' | 'upload-complete' | 'error'

interface ProgressData {
  phase: string
  message: string
}

interface CompleteData {
  path: string
  bundleId: string
}

interface UploadCompleteData {
  url: string
  remoteId: string
}

export function SupportBundleDialog({ open, onOpenChange, isConnected, savedDevices, connectedDeviceId }: SupportBundleDialogProps) {
  const [stage, setStage] = useState<Stage>('consent')
  const [progressMessage, setProgressMessage] = useState('')
  const [completePath, setCompletePath] = useState('')
  const [uploadURL, setUploadURL] = useState('')
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
      window.go.main.App.GetSupportBundlePreview().then(setPreview)
    }
  }, [open, connectedDeviceId])

  const handleProgress = useCallback((...args: unknown[]) => {
    const data = args[0] as ProgressData
    setStage('progress')
    setProgressMessage(data.message)
  }, [])

  const handleComplete = useCallback((...args: unknown[]) => {
    const data = args[0] as CompleteData
    setStage('complete')
    setCompletePath(data.path)
  }, [])

  const handleUploadComplete = useCallback((...args: unknown[]) => {
    const data = args[0] as UploadCompleteData
    setStage('upload-complete')
    setUploadURL(data.url)
  }, [])

  const handleError = useCallback((...args: unknown[]) => {
    const data = args[0] as { message: string }
    setStage('error')
    setErrorMessage(data.message)
  }, [])

  useEffect(() => {
    if (!open) return
    const unsub1 = window.runtime.EventsOn('support-bundle:progress', handleProgress)
    const unsub2 = window.runtime.EventsOn('support-bundle:complete', handleComplete)
    const unsub3 = window.runtime.EventsOn('support-bundle:error', handleError)
    const unsub4 = window.runtime.EventsOn('support-bundle:upload-complete', handleUploadComplete)
    const unsub5 = window.runtime.EventsOn('support-bundle:upload-error', handleError)
    return () => {
      unsub1()
      unsub2()
      unsub3()
      unsub4()
      unsub5()
    }
  }, [open, handleProgress, handleComplete, handleUploadComplete, handleError])

  const handleGenerate = () => {
    setStage('progress')
    setProgressMessage('Preparing...')
    window.go.main.App.GenerateSupportBundle(selectedDeviceId)
  }

  const handleUpload = () => {
    setStage('progress')
    setProgressMessage('Preparing...')
    window.go.main.App.UploadSupportBundle(selectedDeviceId)
  }

  const handleCopyURL = () => {
    window.go.main.App.CopyToClipboard(uploadURL)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const connectedDeviceName = savedDevices.find(d => d.id === connectedDeviceId)?.name
  const isSelectedDeviceConnected = isConnected && selectedDeviceId === connectedDeviceId

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v || stage !== 'progress') onOpenChange(v) }}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Support Bundle</DialogTitle>
          <DialogDescription>
            {stage === 'consent' && 'Review what will be included in the diagnostic bundle.'}
            {stage === 'progress' && 'Generating support bundle...'}
            {stage === 'complete' && 'Support bundle saved successfully.'}
            {stage === 'upload-complete' && 'Support bundle uploaded successfully.'}
            {stage === 'error' && 'Failed to generate support bundle.'}
          </DialogDescription>
        </DialogHeader>

        {stage === 'consent' && preview && (
          <div className="space-y-4">
            {isConnected && connectedDeviceName ? (
              <div className="text-sm">
                Bundle for <span className="font-medium">{connectedDeviceName}</span>
              </div>
            ) : savedDevices.length > 0 ? (
              <div className="space-y-2">
                <label className="text-sm font-medium">Device</label>
                <Select value={selectedDeviceId} onValueChange={setSelectedDeviceId}>
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Select a device" />
                  </SelectTrigger>
                  <SelectContent>
                    {savedDevices.map(d => (
                      <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedDeviceId && !isSelectedDeviceConnected && (
                  <div className="flex items-start gap-2 text-xs text-muted-foreground">
                    <AlertCircle className="h-3.5 w-3.5 mt-0.5 flex-shrink-0" />
                    <span>Not connected to this device. Bundle will include cached device data, which may be outdated.</span>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex items-start gap-2 text-sm text-muted-foreground">
                <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" />
                <span>No saved devices. Bundle will include application logs only.</span>
              </div>
            )}

            <div className="grid grid-cols-2 gap-4">
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
                  <ShieldOff className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm font-medium">Not included</span>
                </div>
                <ul className="list-disc pl-4 space-y-1">
                  {preview.excluded.map((item) => (
                    <li key={item} className="text-xs text-muted-foreground">{item}</li>
                  ))}
                </ul>
              </div>
            </div>
            <div className="text-xs text-muted-foreground space-y-0.5">
              <p>Uploading generates a unique URL you can share for support.</p>
              <p>Uploaded data is automatically deleted after 30 days, or you can delete it at any time.</p>
            </div>
          </div>
        )}

        {stage === 'progress' && (
          <div className="flex items-center gap-3 py-4">
            <Loader2 className="h-5 w-5 animate-spin" />
            <span className="text-sm">{progressMessage}</span>
          </div>
        )}

        {stage === 'complete' && (
          <div className="flex items-center gap-3 py-4">
            <CheckCircle2 className="h-5 w-5 text-green-600" />
            <div className="text-sm">
              <p>Bundle saved to:</p>
              <p className="text-muted-foreground break-all">{completePath}</p>
            </div>
          </div>
        )}

        {stage === 'upload-complete' && (
          <div className="space-y-3 py-4">
            <div className="flex items-center gap-3">
              <CheckCircle2 className="h-5 w-5 text-green-600 flex-shrink-0" />
              <span className="text-sm">Share this link with support:</span>
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-xs bg-muted px-3 py-2 rounded break-all">{uploadURL}</code>
              <Button variant="outline" size="icon" className="flex-shrink-0" onClick={handleCopyURL}>
                {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              You can view this URL, update the bundle, or delete it from the Support page.
            </p>
          </div>
        )}

        {stage === 'error' && (
          <div className="py-4">
            <p className="text-sm text-destructive">{errorMessage}</p>
          </div>
        )}

        <DialogFooter className="flex-col sm:flex-row gap-2">
          {stage === 'consent' && (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)} className="">
                Cancel
              </Button>
              <Button variant="outline" onClick={handleGenerate}>
                Save Locally
              </Button>
              <Button onClick={handleUpload}>
                Upload
              </Button>
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
                Retry
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
