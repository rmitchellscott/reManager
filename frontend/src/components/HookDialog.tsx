import { CheckCircle, Info, Warning, Wrench } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { DialogSeverity, type DialogRequest } from '@/lib/types'

interface HookDialogProps {
  open: boolean
  request: DialogRequest | null
  onRespond: (response: string) => void
  // Closing without answering, which the backend reads as a cancel.
  onDismiss: () => void
}

// DialogConfig carries no kind; the shape is inferred from these flags, whose valid combinations are defined in internal/vellum/hooks.go.
function isActionList(request: DialogRequest): boolean {
  return !!request.actions?.length && !request.infoOnly && !request.confirmText
}

const SEVERITY = {
  [DialogSeverity.DialogSuccess]: { severity: 'success', icon: CheckCircle },
  [DialogSeverity.DialogInfo]: { severity: 'info', icon: Info },
  [DialogSeverity.DialogWarning]: { severity: 'warning', icon: Warning },
  [DialogSeverity.DialogNeutral]: { severity: 'neutral', icon: Wrench },
} as const

// A hook that states no severity (prompts in internal/vellum/hooks.go all do) gets neutral, not warning: silence should not claim a condition.
function headerFor(request: DialogRequest) {
  return SEVERITY[request.severity || DialogSeverity.DialogNeutral]
}

function ActionList({ request, onRespond, onDismiss }: Omit<HookDialogProps, 'open'> & { request: DialogRequest }) {
  const exitLabel = request.cancelText || 'Cancel'
  const actions = request.actions ?? []
  const primaryLast = [...actions].sort(
    (a, b) => Number(a.id === request.primaryAction) - Number(b.id === request.primaryAction)
  )

  if (!request.choices && actions.length <= 2) {
    return (
      <DialogFooter separateExit={actions.length === 2}>
        <Button variant="outline" onClick={onDismiss}>
          {exitLabel}
        </Button>
        {primaryLast.map((action) => (
          <Button
            key={action.id}
            variant={action.id === request.primaryAction ? 'default' : 'outline'}
            onClick={() => onRespond(action.id)}
          >
            {action.label}
          </Button>
        ))}
      </DialogFooter>
    )
  }

  return (
    <div className="flex flex-col gap-2 pt-2">
      {actions.map((action) => (
        <Button
          key={action.id}
          variant={action.id === request.primaryAction ? 'default' : 'outline'}
          className="w-full justify-center gap-2 h-auto min-h-9 py-2 whitespace-normal"
          onClick={() => onRespond(action.id)}
        >
          {action.label}
        </Button>
      ))}
      <Button variant="ghost" className="w-full justify-center mt-1" onClick={onDismiss}>
        {exitLabel}
      </Button>
    </div>
  )
}

// The prompt copy lives in internal/vellum/hooks.go, not here.
function HookDialogBody({ request, onRespond, onDismiss }: Omit<HookDialogProps, 'open'>) {
  if (!request) return null

  const { severity, icon } = headerFor(request)
  const paragraphs = request.message ? request.message.split('\n\n') : []
  const hasBody = paragraphs.length > 0 || !!request.inProgressMessage || !!request.note || !!request.steps?.length

  return (
    <>
      <DialogHeader severity={severity} icon={icon}>
        <DialogTitle>{request.title}</DialogTitle>
        {request.subtitle && <DialogDescription>{request.subtitle}</DialogDescription>}
      </DialogHeader>

      {hasBody && (
      <div className="space-y-3 text-sm text-muted-foreground">
        {paragraphs.map((paragraph, idx) => (
          <p key={idx}>{paragraph}</p>
        ))}
        {request.inProgressMessage && (
          <p className="font-semibold text-foreground">{request.inProgressMessage}</p>
        )}
        {request.note && (
          <div className="flex items-center gap-2 rounded-lg border p-3 bg-muted/50">
            <Info className="h-4 w-4 text-blue-500 flex-shrink-0" />
            <span className="text-xs text-muted-foreground">
              {request.note.split('\n').map((line, idx) => (
                <span key={idx}>{idx > 0 && <br />}{line}</span>
              ))}
            </span>
          </div>
        )}
        {request.steps && request.steps.length > 0 && (
          <>
            <p className="font-medium text-foreground">This will:</p>
            <ol className="list-decimal list-outside space-y-1 text-sm pl-8">
              {request.steps.map((step, idx) => (
                <li key={idx}>{step}</li>
              ))}
            </ol>
          </>
        )}
      </div>
      )}

      {isActionList(request) ? (
        <ActionList request={request} onRespond={onRespond} onDismiss={onDismiss} />      ) : (
        <DialogFooter>
          {!request.infoOnly && (
            <Button variant="outline" onClick={onDismiss}>
              {request.cancelText || 'Cancel'}
            </Button>
          )}
          <Button onClick={() => onRespond('confirm')}>
            {request.confirmText || (request.infoOnly ? 'Got it' : 'Proceed')}
          </Button>
        </DialogFooter>
      )}
    </>
  )
}

export function HookDialog({ open, request, onRespond, onDismiss }: HookDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onDismiss() }}>
      <DialogContent>
        <HookDialogBody request={request} onRespond={onRespond} onDismiss={onDismiss} />
      </DialogContent>
    </Dialog>
  )
}
