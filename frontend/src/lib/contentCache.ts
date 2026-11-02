import { PackagesService } from '@bindings/reManager'


// Readme and changelog URLs are version-pinned by the packaging rules, so content keyed by URL cannot go stale while the app runs.
const MAX_ENTRIES = 24

const cache = new Map<string, string>()

export function getCachedContent(url: string): string | undefined {
  return cache.get(url)
}

export function setCachedContent(url: string, content: string): void {
  if (cache.has(url)) cache.delete(url)
  cache.set(url, content)
  while (cache.size > MAX_ENTRIES) {
    const oldest = cache.keys().next().value
    if (oldest === undefined) break
    cache.delete(oldest)
  }
}

const inFlight = new Map<string, Promise<void>>()

export function prefetchContent(url: string | null): void {
  if (!url) return
  const fetchUrl = url.split('#')[0]
  if (!fetchUrl || cache.has(fetchUrl) || inFlight.has(fetchUrl)) return

  const request = PackagesService.GetReadmeContent(fetchUrl)
    .then((text: string) => { setCachedContent(fetchUrl, text) })
    .catch(() => {})
    .finally(() => { inFlight.delete(fetchUrl) })

  inFlight.set(fetchUrl, request)
}
