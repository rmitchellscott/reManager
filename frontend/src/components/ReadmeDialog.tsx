import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import type { ReactNode } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Loader2, X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'

interface ReadmeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  url: string | null
  packageName: string
}

function resolveUrl(src: string, baseUrl: string): string {
  if (src.startsWith('http://') || src.startsWith('https://') || src.startsWith('data:')) {
    return src
  }
  const base = baseUrl.substring(0, baseUrl.lastIndexOf('/') + 1)
  return base + src
}

function stripHtmlComments(text: string): string {
  return text.replace(/<!--[\s\S]*?-->/g, '')
}

function textContent(node: ReactNode): string {
  if (typeof node === 'string') return node
  if (typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContent).join('')
  if (node && typeof node === 'object' && 'props' in node) return textContent(node.props.children)
  return ''
}

function githubSlug(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
}

function trimToAnchor(markdown: string, anchor: string): string {
  const lines = markdown.split('\n')
  const headingPattern = /^(#{1,6})\s+(.+)$/
  let startIdx = -1
  let startLevel = 0

  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(headingPattern)
    if (match && githubSlug(match[2]) === anchor) {
      startIdx = i
      startLevel = match[1].length
      break
    }
  }

  if (startIdx === -1) return markdown

  let endIdx = lines.length
  for (let i = startIdx + 1; i < lines.length; i++) {
    const match = lines[i].match(headingPattern)
    if (match && match[1].length <= startLevel) {
      endIdx = i
      break
    }
  }

  return lines.slice(startIdx, endIdx).join('\n')
}

export function ReadmeDialog({ open, onOpenChange, url, packageName }: ReadmeDialogProps) {
  const [rawContent, setRawContent] = useState<string | null>(null)
  const [cachedUrl, setCachedUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetchUrl = url?.split('#')[0] || null
  const initialAnchor = url?.includes('#') ? url.split('#')[1] : null

  const baseUrl = url || ''
  const slugCounts = useRef<Record<string, number>>({})
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open || !fetchUrl) return
    slugCounts.current = {}

    if (cachedUrl === fetchUrl && rawContent) return

    setRawContent(null)
    setLoading(true)
    setError(null)
    window.go.main.App.GetReadmeContent(fetchUrl)
      .then((text: string) => {
        setRawContent(text)
        setCachedUrl(fetchUrl)
      })
      .catch(() => setError('Failed to load readme'))
      .finally(() => setLoading(false))
  }, [open, fetchUrl])

  const [enlargedImage, setEnlargedImage] = useState<string | null>(null)

  const content = useMemo(() => {
    if (!rawContent) return null
    let cleaned = stripHtmlComments(rawContent)
    if (initialAnchor) {
      cleaned = trimToAnchor(cleaned, initialAnchor)
      cleaned = cleaned.replace(/^#{1,6}\s+.+\n?/, '')
      cleaned = cleaned.replace(/\[!\[vellum\]\(https:\/\/img\.shields\.io\/badge\/vellum-[^)]*\)\]\([^)]*\)\s*/g, '')
    }
    return cleaned.trim()
  }, [rawContent, initialAnchor])

  const getSlug = useCallback((children: ReactNode) => {
    const raw = githubSlug(textContent(children))
    const count = slugCounts.current[raw] || 0
    slugCounts.current[raw] = count + 1
    return count === 0 ? raw : `${raw}-${count}`
  }, [])


  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) setEnlargedImage(null); onOpenChange(v) }}>
      <DialogContent className="w-[92vw] sm:w-[80vw] sm:max-w-5xl max-h-[85vh] flex flex-col relative">
        <DialogHeader className="flex-shrink-0">
          <div className="flex items-center justify-between pr-1">
            <DialogTitle>{packageName}</DialogTitle>
            <button
              onClick={() => onOpenChange(false)}
              className="rounded-sm opacity-70 hover:opacity-100 transition-opacity"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </DialogHeader>
        <div className="overflow-y-auto flex-1 min-h-0" ref={scrollRef}>
        {loading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : error ? <p className="text-destructive text-sm">{error}</p> : null}
        {content && (
          <div className="readme-content text-sm">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeRaw]}
              urlTransform={(src) => resolveUrl(src, baseUrl)}
              components={{
                h1: ({ children }) => <h1 id={getSlug(children)} className="text-2xl font-bold mt-6 mb-3 pb-2 border-b first:mt-0">{children}</h1>,
                h2: ({ children }) => <h2 id={getSlug(children)} className="text-xl font-semibold mt-5 mb-2 pb-1 border-b">{children}</h2>,
                h3: ({ children }) => <h3 id={getSlug(children)} className="text-lg font-semibold mt-4 mb-2">{children}</h3>,
                h4: ({ children }) => <h4 id={getSlug(children)} className="text-base font-semibold mt-3 mb-1">{children}</h4>,
                p: ({ children }) => <p className="my-2 leading-relaxed">{children}</p>,
                ul: ({ children }) => <ul className="my-2 ml-6 list-disc space-y-1">{children}</ul>,
                ol: ({ children }) => <ol className="my-2 ml-6 list-decimal space-y-1">{children}</ol>,
                li: ({ children }) => <li className="leading-relaxed">{children}</li>,
                a: ({ href, children }) => (
                  <button
                    className="text-primary hover:underline"
                    onClick={() => {
                      if (!href) return
                      if (href.startsWith('#')) {
                        const el = scrollRef.current?.querySelector(href)
                        el?.scrollIntoView({ behavior: 'smooth' })
                      } else {
                        window.runtime.BrowserOpenURL(href)
                      }
                    }}
                  >
                    {children}
                  </button>
                ),
                code: ({ className, children }) => {
                  const isBlock = className?.includes('language-')
                  if (isBlock) {
                    return (
                      <pre className="my-3 p-3 bg-muted rounded-md overflow-x-auto">
                        <code className="text-xs">{children}</code>
                      </pre>
                    )
                  }
                  return <code className="px-1 py-0.5 bg-muted rounded text-xs">{children}</code>
                },
                pre: ({ children }) => <>{children}</>,
                blockquote: ({ children }) => (
                  <blockquote className="my-3 pl-4 border-l-4 border-muted-foreground/30 text-muted-foreground italic">
                    {children}
                  </blockquote>
                ),
                table: ({ children }) => (
                  <div className="my-3 overflow-x-auto">
                    <table className="w-full border-collapse text-sm">{children}</table>
                  </div>
                ),
                th: ({ children }) => <th className="border px-3 py-1.5 bg-muted text-left font-semibold">{children}</th>,
                td: ({ children }) => <td className="border px-3 py-1.5">{children}</td>,
                img: ({ src, alt, height, width }) => (
                  <img
                    src={src}
                    alt={alt || ''}
                    style={{
                      maxWidth: '100%',
                      height: height ? `${height}px` : undefined,
                      width: width ? `${width}px` : undefined,
                    }}
                    className="my-2 rounded cursor-pointer hover:opacity-80 transition-opacity"
                    onClick={() => src && setEnlargedImage(src)}
                  />
                ),
                hr: () => <hr className="my-4 border-t" />,
              }}
            >
              {content}
            </ReactMarkdown>
          </div>
        )}
        </div>
        {enlargedImage && (
          <div
            className="absolute inset-0 z-10 bg-black/85 flex items-center justify-center rounded-lg cursor-pointer"
            onClick={() => setEnlargedImage(null)}
          >
            <img
              src={enlargedImage}
              alt=""
              className="max-w-[95%] max-h-[90%] object-contain rounded"
              onClick={(e) => e.stopPropagation()}
            />
            <button
              onClick={() => setEnlargedImage(null)}
              className="absolute top-3 right-3 text-white opacity-70 hover:opacity-100 transition-opacity"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
