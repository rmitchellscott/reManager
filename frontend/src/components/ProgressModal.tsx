import { DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import { Terminal } from '@/components/Terminal'
import { Loader2 } from 'lucide-react'

interface ProgressModalProps {
  title: string
  progressText: string
  percentage: number
  terminalOutput: string
  isComplete: boolean
  onClose: () => void
  canStop?: boolean
  onStop?: () => void
}

export function ProgressModal({
  title,
  progressText,
  percentage,
  terminalOutput,
  isComplete,
  onClose,
  canStop,
  onStop,
}: ProgressModalProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
      </DialogHeader>
      <div className="space-y-4">
        <p className="text-sm font-medium text-center text-foreground flex items-center justify-center gap-2">
          {!isComplete && <Loader2 className="h-4 w-4 animate-spin" />}
          {progressText}
        </p>
        <Progress value={percentage} />
        <div className="h-[400px] rounded-lg overflow-hidden overscroll-y-contain">
          <Terminal output={terminalOutput} />
        </div>
      </div>
      <DialogFooter>
        {isComplete ? (
          <Button onClick={onClose}>Close</Button>
        ) : canStop ? (
          <Button onClick={onStop} variant="destructive">Stop</Button>
        ) : null}
      </DialogFooter>
    </>
  )
}
