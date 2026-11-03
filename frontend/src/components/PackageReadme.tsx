import { CircleNotch, X } from '@phosphor-icons/react'
import { useState, useEffect, useMemo, useRef } from 'react'
import { createPortal } from 'react-dom'
import ReactMarkdown from 'react-markdown'
import type { Root, RootContent } from 'hast'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize from 'rehype-sanitize'
import { getCachedContent, setCachedContent } from '@/lib/contentCache'
import { MarkdownLink, resolveMarkdownUrl } from '@/components/MarkdownLink'
import { PackagesService } from '@bindings/reManager'

interface PackageReadmeProps {
  // The readme scroller's edge-fade mask clips descendants, so the enlarged image must render here rather than inside the readme.
  imageOverlayContainer?: HTMLElement | null
  url: string | null
}

function stripHtmlComments(text: string): string {
  return text.replace(/<!--[\s\S]*?-->/g, '')
}

function hastText(node: Root | RootContent): string {
  if (node.type === 'text') return node.value
  if ('children' in node) return node.children.map(hastText).join('')
  return ''
}

function githubSlug(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
}

// Ids are assigned in document order so they match the `#anchor` links GitHub generates.
function rehypeHeadingIds() {
  return (tree: Root) => {
    const slugCounts: Record<string, number> = {}
    const assignIds = (node: Root | RootContent) => {
      if (node.type === 'element' && /^h[1-6]$/.test(node.tagName)) {
        if (node.properties.id) return
        const slug = githubSlug(hastText(node))
        const count = slugCounts[slug] ?? 0
        slugCounts[slug] = count + 1
        node.properties.id = count === 0 ? slug : `${slug}-${count}`
        return
      }
      if ('children' in node) node.children.forEach(assignIds)
    }
    assignIds(tree)
  }
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

export function PackageReadme({ url, imageOverlayContainer }: PackageReadmeProps) {
  const [rawContent, setRawContent] = useState<string | null>(null)
  const [cachedUrl, setCachedUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [enlargedImage, setEnlargedImage] = useState<string | null>(null)

  const fetchUrl = url?.split('#')[0] || null
  const initialAnchor = url?.includes('#') ? url.split('#')[1] : null

  const baseUrl = url || ''
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!fetchUrl) return

    if (cachedUrl === fetchUrl && rawContent) return

    const cached = getCachedContent(fetchUrl)
    if (cached !== undefined) {
      setRawContent(cached)
      setCachedUrl(fetchUrl)
      setLoading(false)
      setError(null)
      return
    }

    let superseded = false
    setRawContent(null)
    setLoading(true)
    setError(null)
    PackagesService.GetReadmeContent(fetchUrl)
      .then((text: string) => {
        setCachedContent(fetchUrl, text)
        if (superseded) return
        setRawContent(text)
        setCachedUrl(fetchUrl)
      })
      .catch(() => {
        if (!superseded) setError('Could not load the README.')
      })
      .finally(() => {
        if (!superseded) setLoading(false)
      })
    return () => {
      superseded = true
    }
  }, [fetchUrl])

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

  const scrollToHeading = (id: string) => {
    scrollRef.current?.querySelector(`#${CSS.escape(id)}`)?.scrollIntoView({ behavior: 'smooth' })
  }

  const enlargedImageOverlay = (
    <div
      className="absolute inset-0 z-10 bg-black/85 flex items-center justify-center rounded-lg cursor-pointer"
      onClick={() => setEnlargedImage(null)}
    >
      <img
        src={enlargedImage ?? undefined}
        alt=""
        className="max-w-[95%] max-h-[90%] object-contain rounded"
        onClick={(e) => e.stopPropagation()}
      />
      <button
        onClick={() => setEnlargedImage(null)}
        aria-label="Close image"
        className="absolute top-3 right-3 text-white opacity-70 hover:opacity-100 transition-opacity"
      >
        <X className="h-5 w-5" />
      </button>
    </div>
  )

  return (
    <div>
      <div ref={scrollRef}>
        {loading ? (
          <div className="flex justify-center py-8">
            <CircleNotch className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : error ? <p className="text-destructive text-sm">{error}</p> : null}
        {content && (
          <div className="readme-content text-sm break-words">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeRaw, rehypeSanitize, rehypeHeadingIds]}
              urlTransform={(src) => resolveMarkdownUrl(src, baseUrl)}
              components={{
                h1: ({ id, children }) => <h1 id={id} className="text-sm font-semibold mt-6 mb-2 pb-1.5 border-b first:mt-0">{children}</h1>,
                h2: ({ id, children }) => <h2 id={id} className="text-sm font-semibold mt-5 mb-2">{children}</h2>,
                h3: ({ id, children }) => <h3 id={id} className="text-sm font-medium mt-4 mb-1.5">{children}</h3>,
                h4: ({ id, children }) => <h4 id={id} className="text-xs font-semibold uppercase tracking-[0.6px] text-muted-foreground mt-3 mb-1">{children}</h4>,
                p: ({ children }) => <p className="my-2 leading-relaxed">{children}</p>,
                ul: ({ children }) => <ul className="my-2 ml-6 list-disc space-y-1">{children}</ul>,
                ol: ({ children }) => <ol className="my-2 ml-6 list-decimal space-y-1">{children}</ol>,
                li: ({ children }) => <li className="leading-relaxed">{children}</li>,
                a: ({ href, children }) => (
                  <MarkdownLink href={href} className="text-primary hover:underline" onInPageLink={scrollToHeading}>
                    {children}
                  </MarkdownLink>
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
                      maxWidth: width ? `min(100%, ${width}px)` : '100%',
                      maxHeight: height ? `${height}px` : undefined,
                      width: 'auto',
                      height: 'auto',
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
      {enlargedImage && (imageOverlayContainer ? createPortal(enlargedImageOverlay, imageOverlayContainer) : enlargedImageOverlay)}
    </div>
  )
}
