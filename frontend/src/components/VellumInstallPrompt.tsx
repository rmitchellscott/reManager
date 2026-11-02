import { Package, WarningCircle } from '@phosphor-icons/react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { OutputDisclosure } from '@/components/OutputDisclosure'
import { Progress } from '@/components/ui/progress'

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
        <div className="flex items-center gap-4">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-blue-500/10 text-blue-600 ring-1 ring-inset ring-blue-500/20 dark:text-blue-400">
            <Package className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <CardTitle>
              {bootstrapping ? 'Installing Vellum…' : 'Install Vellum'}
            </CardTitle>
            <CardDescription>
              {bootstrapping
                ? 'Vellum is being installed on your reMarkable.'
                : 'Required to install mods.'}
            </CardDescription>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {bootstrapping && (
          <Progress indeterminate indicatorClassName="bg-[var(--info)]" />
        )}

        {bootstrapError && (
          <Alert variant="destructive">
            <WarningCircle className="h-4 w-4" />
            <AlertDescription>Installation failed: {bootstrapError}</AlertDescription>
          </Alert>
        )}

        {(bootstrapping || bootstrapError) && (
          <OutputDisclosure output={bootstrapOutput} theme={terminalTheme} failed={!!bootstrapError} awaitingOutput={bootstrapping} />
        )}

        {!bootstrapping && (
          <>
            {!bootstrapError && (
              <>
                <p className="text-sm text-muted-foreground">
                  Vellum is the package manager reManager uses to install and manage mods on your reMarkable.
                </p>
                <p className="text-sm text-muted-foreground">
                  Vellum and any packages installed through it are third-party software provided
                  as-is with no warranty. reManager is not responsible for any data loss, device
                  damage, or other issues that may result from their use.
                </p>
              </>
            )}

            <div className="flex justify-end pt-2">
              <Button onClick={onInstall}>
                {bootstrapError ? 'Try again' : 'Install Vellum'}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
