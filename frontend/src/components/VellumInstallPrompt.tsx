import { Loader2, Package, AlertCircle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { TerminalWithCopy } from '@/components/TerminalWithCopy'

interface VellumInstallPromptProps {
  bootstrapping: boolean
  bootstrapOutput: string
  bootstrapError: string | null
  onInstall: () => void
  terminalTheme?: 'dark' | 'light'
}

export function VellumInstallPrompt({
  bootstrapping,
  bootstrapOutput,
  bootstrapError,
  onInstall,
  terminalTheme,
}: VellumInstallPromptProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Package className="h-5 w-5 text-muted-foreground" />
          <CardTitle>
            {bootstrapping ? 'Installing Vellum...' : 'Install Vellum Package Manager'}
          </CardTitle>
        </div>
        <CardDescription>
          {bootstrapping
            ? 'Please wait while Vellum is being installed on your device.'
            : 'Vellum package manager is required to install mods.'}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        {bootstrapping && (
          <div className="flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span className="text-sm">Installing...</span>
          </div>
        )}

        {bootstrapError && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>Installation failed: {bootstrapError}</AlertDescription>
          </Alert>
        )}

        {(bootstrapping || bootstrapError) && bootstrapOutput && (
          <div className="h-[300px] rounded-lg overflow-hidden">
            <TerminalWithCopy output={bootstrapOutput} theme={terminalTheme} />
          </div>
        )}

        {!bootstrapping && (
          <>
            {!bootstrapError && (
              <p className="text-sm text-muted-foreground">
                Vellum is a lightweight package manager for reMarkable devices.
                It enables easy installation and management of mods and extensions.
              </p>
            )}

            <div className="flex justify-end pt-2">
              <Button onClick={onInstall}>
                {bootstrapError ? 'Retry Installation' : 'Install Vellum'}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
