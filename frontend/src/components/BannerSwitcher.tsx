import { useState, useRef, useEffect } from 'react'
import { type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

type BannerSeverity = 'destructive' | 'warning' | 'info'

export interface BannerEntry {
  id: string
  priority: number
  severity: BannerSeverity
  icon: LucideIcon
  active: boolean
  content: React.ReactNode
}

interface BannerSwitcherProps {
  banners: BannerEntry[]
}

const severityStyles: Record<BannerSeverity, { selected: string; icon: string; hover: string }> = {
  destructive: {
    selected: 'bg-red-100 dark:bg-red-950',
    icon: 'text-destructive',
    hover: 'hover:bg-red-50 dark:hover:bg-red-950/50',
  },
  warning: {
    selected: 'bg-amber-100 dark:bg-amber-950',
    icon: 'text-amber-600 dark:text-amber-400',
    hover: 'hover:bg-amber-50 dark:hover:bg-amber-950/50',
  },
  info: {
    selected: 'bg-blue-100 dark:bg-blue-950',
    icon: 'text-blue-600 dark:text-blue-400',
    hover: 'hover:bg-blue-50 dark:hover:bg-blue-950/50',
  },
}

export function BannerSwitcher({ banners }: BannerSwitcherProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const prevActiveIdsRef = useRef<Set<string>>(new Set())

  const activeBanners = banners
    .filter(b => b.active)
    .sort((a, b) => a.priority - b.priority)

  const activeKey = activeBanners.map(b => b.id).join(',')

  useEffect(() => {
    const currentActiveIds = new Set(activeBanners.map(b => b.id))
    const prevActiveIds = prevActiveIdsRef.current

    const newlyAppeared = activeBanners.filter(b => !prevActiveIds.has(b.id))
    if (newlyAppeared.length > 0) {
      const highestNew = newlyAppeared[0]
      const currentSelected = activeBanners.find(b => b.id === selectedId)
      if (!currentSelected || highestNew.priority < currentSelected.priority) {
        setSelectedId(highestNew.id)
      }
    }

    if (selectedId && !currentActiveIds.has(selectedId) && activeBanners.length > 0) {
      setSelectedId(activeBanners[0].id)
    }

    if (activeBanners.length === 0) {
      setSelectedId(null)
    }

    if (!selectedId && activeBanners.length > 0) {
      setSelectedId(activeBanners[0].id)
    }

    prevActiveIdsRef.current = currentActiveIds
  }, [activeKey])

  if (activeBanners.length === 0) return null

  const selected = activeBanners.find(b => b.id === selectedId) ?? activeBanners[0]

  if (activeBanners.length === 1) {
    return <div className="mb-4">{selected.content}</div>
  }

  return (
    <div className="mb-4 flex items-stretch">
      <div className="flex flex-col gap-1 pr-2 justify-center shrink-0">
        {activeBanners.map(banner => {
          const styles = severityStyles[banner.severity]
          const isSelected = banner.id === selected.id
          return (
            <button
              key={banner.id}
              onClick={() => setSelectedId(banner.id)}
              className={cn(
                'rounded-md p-1.5 transition-colors',
                styles.icon,
                isSelected ? styles.selected : cn('bg-transparent', styles.hover),
              )}
            >
              <banner.icon className="h-4 w-4" />
            </button>
          )
        })}
      </div>
      <div className="flex-1 min-w-0 grid [&>*]:col-start-1 [&>*]:row-start-1 [&>*>*]:h-full">
        {activeBanners.map(banner => (
          <div
            key={banner.id}
            className={banner.id !== selected.id ? 'invisible pointer-events-none' : ''}
            aria-hidden={banner.id !== selected.id}
          >
            {banner.content}
          </div>
        ))}
      </div>
    </div>
  )
}
