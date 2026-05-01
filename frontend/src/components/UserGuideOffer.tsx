import { BookOpen, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Banner } from '@/components/ui/banner'

interface UserGuideOfferProps {
  type: 'install' | 'update'
  installing: boolean
  onInstall: () => void
  onDismiss: () => void
  onDismissPermanently: () => void
}

export function UserGuideOffer({
  type,
  installing,
  onInstall,
  onDismiss,
  onDismissPermanently,
}: UserGuideOfferProps) {
  return (
    <Banner
      severity="info"
      icon={BookOpen}
      title={type === 'install' ? 'User Guide Available' : 'User Guide Update'}
      onDismiss={!installing ? onDismiss : undefined}
      actions={
        <>
          <Button variant="outline" size="sm" onClick={onDismissPermanently} disabled={installing}>
            Don't ask again
          </Button>
          <Button size="sm" onClick={onInstall} disabled={installing}>
            {installing ? (
              <>
                <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
                Installing...
              </>
            ) : (
              type === 'install' ? 'Install' : 'Update'
            )}
          </Button>
        </>
      }
    >
      {type === 'install'
        ? 'A user guide for reManager is available. Install it on your reMarkable?'
        : 'An updated user guide is available for your reMarkable.'}
    </Banner>
  )
}
