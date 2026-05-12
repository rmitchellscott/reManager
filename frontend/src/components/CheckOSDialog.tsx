import { useState, useEffect, useRef } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Loader2, Check, X, AlertCircle } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { StatusBadge } from '@/components/StatusBadge'
import { CompatibilityResult } from '@/lib/types'

interface CheckOSDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isConnected: boolean
  onSelectPackage: (name: string, targetOS: string, isCompatible: boolean) => void
  initialVersion?: string
}

export function CheckOSDialog({
  open,
  onOpenChange,
  isConnected,
  onSelectPackage,
  initialVersion,
}: CheckOSDialogProps) {
  const [targetVersion, setTargetVersion] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<CompatibilityResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const autoChecked = useRef(false)

  useEffect(() => {
    if (open && initialVersion && !autoChecked.current) {
      autoChecked.current = true
      setTargetVersion(initialVersion)
      setLoading(true)
      setError(null)
      setResult(null)
      ;(async () => {
        try {
          const res = await window.go.main.App.CheckOSCompatibility(initialVersion)
          if (res.fetchFailed) {
            setError('Could not fetch package index. Check your connection.')
          } else {
            setResult(res)
          }
        } catch {
          setError('Failed to check compatibility')
        } finally {
          setLoading(false)
        }
      })()
    }
    if (!open) {
      autoChecked.current = false
    }
  }, [open, initialVersion])

  const handleCheck = async () => {
    if (!targetVersion.trim()) return

    setLoading(true)
    setError(null)
    setResult(null)

    try {
      const res = await window.go.main.App.CheckOSCompatibility(targetVersion.trim())
      if (res.fetchFailed) {
        setError('Could not fetch package index. Check your connection.')
      } else {
        setResult(res)
      }
    } catch (err) {
      setError('Failed to check compatibility')
    } finally {
      setLoading(false)
    }
  }

  const handleClose = () => {
    setTargetVersion('')
    setResult(null)
    setError(null)
    onOpenChange(false)
  }

  const compatiblePackages = result ? [...result.compatible, ...result.noConstraint] : []
  const incompatiblePackages = result?.incompatible ?? []

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Check OS Compatibility</DialogTitle>
          <DialogDescription>
            Check if installed packages are compatible with a target OS version.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="target-version">Target OS Version</Label>
            <div className="flex gap-2">
              <Input
                id="target-version"
                placeholder="e.g., 3.24.0.149"
                value={targetVersion}
                onChange={(e) => setTargetVersion(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleCheck()}
                disabled={loading || !isConnected}
                readOnly={!!initialVersion}
              />
              <Button
                onClick={handleCheck}
                disabled={loading || !targetVersion.trim() || !isConnected}
              >
                {loading ? (
                  <>
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Checking
                  </>
                ) : (
                  'Check'
                )}
              </Button>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {result && !error && (
            <div className="space-y-2">
              <Accordion type="multiple" defaultValue={incompatiblePackages.length > 0 ? ["incompatible"] : ["compatible"]}>
                {compatiblePackages.length > 0 && (
                  <AccordionItem value="compatible" className="border-none">
                    <AccordionTrigger className="py-2 text-sm font-medium hover:no-underline">
                      Compatible Packages ({compatiblePackages.length})
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="max-h-48 overflow-y-auto">
                        {compatiblePackages.map(pkg => (
                          <div key={pkg} className="py-1 text-sm flex items-center gap-2">
                            <Check className="h-4 w-4 text-green-500 flex-shrink-0" />
                            <button
                              onClick={() => onSelectPackage(pkg, targetVersion, true)}
                              className="text-primary hover:underline text-left"
                            >
                              {pkg}
                            </button>
                            {result?.statusMap?.[pkg] && <StatusBadge status={result.statusMap[pkg]} />}
                          </div>
                        ))}
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                )}

                {incompatiblePackages.length > 0 && (
                  <AccordionItem value="incompatible" className="border-none">
                    <AccordionTrigger className="py-2 text-sm font-medium hover:no-underline">
                      Incompatible Packages ({incompatiblePackages.length})
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="max-h-48 overflow-y-auto">
                        {incompatiblePackages.map(pkg => (
                          <div key={pkg} className="py-1 text-sm flex items-center gap-2">
                            <X className="h-4 w-4 text-destructive flex-shrink-0" />
                            <button
                              onClick={() => onSelectPackage(pkg, targetVersion, false)}
                              className="text-primary hover:underline text-left"
                            >
                              {pkg}
                            </button>
                            {result?.statusMap?.[pkg] && <StatusBadge status={result.statusMap[pkg]} />}
                          </div>
                        ))}
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                )}
              </Accordion>

              {compatiblePackages.length === 0 && incompatiblePackages.length === 0 && (
                <p className="text-sm text-muted-foreground">No packages installed.</p>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
