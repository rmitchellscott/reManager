import { DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'
import { Loader2, CheckCircle2 } from 'lucide-react'

interface ProgressModalProps {
  title: string
  progressText: string
  percentage: number
  terminalOutput: string
  isComplete: boolean
  onClose: () => void
  canStop?: boolean
  onStop?: () => void
  terminalTheme?: 'dark' | 'light'
  showProgress?: boolean
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
  terminalTheme,
  showProgress = true,
}: ProgressModalProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
      </DialogHeader>
      <div className="space-y-4">
        <p className="text-sm font-medium text-center text-foreground flex items-center justify-center gap-2">
          <span className="h-4 w-4 flex-shrink-0">
            {!isComplete && <Loader2 className="h-4 w-4 animate-spin" />}
          </span>
          {progressText}
        </p>
        {showProgress && <Progress value={percentage} />}
        {isComplete && !terminalOutput.trim() ? (
          <div className="h-[400px] rounded-lg flex flex-col items-center justify-center gap-3">
            <CheckCircle2 className="h-12 w-12 text-green-600 dark:text-green-400" />
            <p className="text-sm text-muted-foreground">Operation completed successfully.</p>
          </div>
        ) : (
          <div className="h-[400px] rounded-lg overflow-hidden overscroll-y-contain">
            <TerminalWithCopy output={terminalOutput} theme={terminalTheme} />
          </div>
        )}
      </div>
      <DialogFooter className="min-h-[40px]">
        {isComplete ? (
          <Button onClick={onClose}>Close</Button>
        ) : canStop ? (
          <Button onClick={onStop} variant="destructive">Stop</Button>
        ) : null}
      </DialogFooter>
    </>
  )
}
