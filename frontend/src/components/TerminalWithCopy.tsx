import { Terminal } from './Terminal'
import { Button } from './ui/button'
import { Tooltip, TooltipTrigger, TooltipContent } from './ui/tooltip'
import { Copy, CopyCheck } from 'lucide-react'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'

interface TerminalWithCopyProps {
  output: string
  theme?: 'dark' | 'light'
}

export function TerminalWithCopy({ output, theme }: TerminalWithCopyProps) {
  const { isCopied, copyToClipboard } = useCopyToClipboard()

  return (
    <div className="relative w-full h-full">
      <Terminal output={output} theme={theme} />
      <div className="absolute top-2 right-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => copyToClipboard(output)}
              className="h-8 w-8 bg-background/95 backdrop-blur-sm hover:bg-background"
            >
              {isCopied ? (
                <CopyCheck className="h-4 w-4" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {isCopied ? 'Copied!' : 'Copy output'}
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  )
}
