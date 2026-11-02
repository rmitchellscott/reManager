import type { ReactNode } from 'react'
import { defaultUrlTransform } from 'react-markdown'
import { SystemService } from '@bindings/reManager'

interface MarkdownLinkProps {
  href?: string
  children?: ReactNode
  className?: string
  onInPageLink?: (id: string) => void
}

export function resolveMarkdownUrl(src: string, documentUrl: string): string {
  if (src.startsWith('data:')) return src
  if (src.startsWith('#') || /^[a-z][a-z\d+.-]*:/i.test(src)) return defaultUrlTransform(src)
  const base = documentUrl.substring(0, documentUrl.lastIndexOf('/') + 1)
  return base + src
}

export function MarkdownLink({ href, children, className, onInPageLink }: MarkdownLinkProps) {
  return (
    <a
      href={href}
      className={className}
      onClick={(e) => {
        e.preventDefault()
        if (!href) return
        if (href.startsWith('#')) {
          onInPageLink?.(href.slice(1))
        } else {
          SystemService.OpenExternalURL(href)
        }
      }}
    >
      {children}
    </a>
  )
}
