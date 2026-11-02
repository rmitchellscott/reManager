import { Package } from '@phosphor-icons/react'
import { DeviceIcon } from '@/components/DeviceIcon'
import { TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useEffect, useRef, type ComponentType } from 'react'
import { PlanBarContent, PlanBarWash, type PlanBarModel } from '@/components/mods/PlanBar'
import type { ModsDotLevel } from '@/lib/modsStatus'
import { useTouchLayout } from '@/hooks/useMediaQuery'

export type MainTab = 'mods' | 'system'
interface MainTabBarProps {
  activeTab: MainTab
  collapsed: boolean
  hidden: boolean
  onExpand: () => void
  modsDot?: ModsDotLevel | null
  modsDotBesidePlanBar?: ModsDotLevel | null
  systemAlert?: AlertLevel | null
  device: string
  planBar?: PlanBarModel | null
  modsDisabled?: boolean
}

type AlertLevel = 'unread' | 'info' | 'pending' | 'warning' | 'destructive'

const LEVEL_RANK: Record<AlertLevel, number> = { unread: 0, info: 1, pending: 2, warning: 3, destructive: 4 }
const LEVEL_DOT: Record<AlertLevel, string> = {
  unread: 'bg-[var(--unread)]',
  info: 'bg-[var(--info)]',
  pending: 'bg-[var(--pending)]',
  warning: 'bg-[var(--warning)]',
  destructive: 'bg-[var(--destructive)]',
}

const LEVEL_LABEL: Record<AlertLevel, string> = {
  unread: 'new items',
  info: 'new information',
  pending: 'changes queued',
  warning: 'needs attention',
  destructive: 'needs attention',
}

const PLAN_BAR_REPLACES_MODS_DOT = true

interface TabIconProps {
  device: string
  className: string
}

interface TabDefinition {
  value: MainTab
  label: string
  Icon: ComponentType<TabIconProps>
}

const TABS: TabDefinition[] = [
  { value: 'mods', label: 'Mods', Icon: ({ className }) => <Package className={className} aria-hidden /> },
  { value: 'system', label: 'System', Icon: ({ device, className }) => <DeviceIcon device={device} className={className} aria-hidden /> },
]

const desktopTrigger = 'px-[18px] min-w-[180px]'

const shadcnTrigger =
  'flex-col h-full basis-0 min-w-0 rounded-lg py-[6px] text-[12px] leading-tight font-medium overflow-hidden text-muted-foreground transition-[flex-grow,opacity,padding,gap,color,background-color] duration-300 ease-out data-[state=active]:shadow-none'

const shadcnActiveExpanded =
  'data-[state=active]:bg-accent data-[state=active]:text-accent-foreground'

const shadcnActiveCollapsed =
  'data-[state=active]:bg-transparent data-[state=active]:text-foreground'

function TabBadge({ level }: { level: AlertLevel }) {
  return (
    <span aria-hidden className={`absolute left-full top-1/2 ml-1 h-1.5 w-1.5 -translate-y-1/2 rounded-full ${LEVEL_DOT[level]}`} />
  )
}

export function MainTabBar({
  activeTab,
  collapsed,
  hidden,
  onExpand,
  modsDot = null,
  modsDotBesidePlanBar = null,
  systemAlert = null,
  device,
  planBar = null,
  modsDisabled = false,
}: MainTabBarProps) {
  const floating = useTouchLayout()
  const lastPlanBar = useRef<PlanBarModel | null>(null)
  if (planBar) lastPlanBar.current = planBar
  const shownPlanBar = planBar ?? lastPlanBar.current
  const hasPlan = floating && !!planBar

  const previousTab = useRef(activeTab)
  const leftByTab = useRef(false)
  const tabChanged = previousTab.current !== activeTab
  if (!planBar && tabChanged) leftByTab.current = true
  else if (!planBar && !tabChanged && activeTab === 'mods') leftByTab.current = false
  useEffect(() => { previousTab.current = activeTab }, [activeTab])
  const modsLevel = PLAN_BAR_REPLACES_MODS_DOT && planBar ? modsDotBesidePlanBar : modsDot
  const alerted: Partial<Record<MainTab, AlertLevel>> = {
    ...(modsLevel ? { mods: modsLevel } : {}),
    ...(systemAlert ? { system: systemAlert } : {}),
  }

  const worstAlert = TABS
    .map(({ value }) => alerted[value])
    .filter((l): l is AlertLevel => !!l)
    .sort((a, b) => LEVEL_RANK[b] - LEVEL_RANK[a])[0]

  const dotLevel = (value: MainTab): AlertLevel | undefined =>
    floating && collapsed
      ? (value === activeTab ? worstAlert : undefined)
      : alerted[value]
  const badged = (value: MainTab) => !!dotLevel(value)

  const wrapper = !floating
    ? 'relative'
    : 'fixed z-50 left-[max(12px,var(--safe-area-left))] right-[max(12px,var(--safe-area-right))] bottom-[calc(var(--safe-area-bottom)+8px)] h-[136px] pointer-events-none'

  const surface = 'rounded-xl border bg-background/95 shadow-md backdrop-blur supports-[backdrop-filter]:bg-background/80'

  const list = !floating
    ? ''
    : `absolute bottom-0 flex overflow-hidden p-[6px] pointer-events-auto ${surface} transition-[width,height,left] duration-300 ease-out ${
        collapsed ? `w-[56px] h-[56px] ${hasPlan ? 'left-0' : 'left-[calc(50%-28px)]'}` : 'left-0 w-full h-[64px]'
      }`

  const hiddenByTab = !planBar && leftByTab.current
  const planBarBox = `absolute z-10 right-0 bottom-0 pl-[14px] pr-2 ${surface} transition-[translate,left,height,opacity,scale] ease-out ${hiddenByTab ? 'duration-0' : 'duration-300'} ${
    collapsed ? 'left-[64px] translate-y-0 h-[56px]' : 'left-0 -translate-y-[72px] h-[64px]'
  } ${planBar
    ? 'pointer-events-auto'
    : hiddenByTab
      ? 'pointer-events-none opacity-0 -translate-x-[calc(100%+24px)] motion-reduce:translate-x-0'
      : 'pointer-events-none opacity-0 scale-[0.97]'}`

  function triggerClass(value: MainTab) {
    if (!floating) return desktopTrigger

    const shrunk = collapsed && value !== activeTab
    return `${shadcnTrigger} ${
      collapsed ? `gap-0 ${shadcnActiveCollapsed}` : `gap-[4px] ${shadcnActiveExpanded}`
    } ${
      shrunk
        ? 'grow-0 px-0 opacity-0 pointer-events-none'
        : 'grow px-[8px] opacity-100'
    }`
  }

  // Radix switches tabs on pointer down, so whether a tab was already active must be read at the press, not the click.
  const pressedActiveTab = useRef<MainTab | null>(null)
  const returnToTop = (value: MainTab) => {
    const wasActive = pressedActiveTab.current === value
    pressedActiveTab.current = null
    if (!floating || collapsed || !wasActive) return
    window.scrollTo({ top: 0 })
  }

  const labelClass = !floating
    ? ''
    : `block overflow-hidden transition-[max-height,opacity] duration-300 ease-out ${
        collapsed ? 'max-h-0 opacity-0' : 'max-h-[16px] opacity-100'
      }`

  return (
    <div className={wrapper} style={floating && hidden ? { display: 'none' } : undefined}>
      <TabsList
        onClick={floating && collapsed ? onExpand : undefined}
        className={floating
          ? `grid grid-cols-2 mb-0 ${list}`
          : 'grid w-auto grid-cols-2'}
      >
        {TABS.map(({ value, label, Icon }) => (
          <TabsTrigger key={value} value={value} disabled={value === 'mods' && modsDisabled} className={triggerClass(value)} onPointerDown={() => { pressedActiveTab.current = activeTab }} onClick={() => returnToTop(value)}>
            {floating && (
              <span className="relative shrink-0">
                <Icon device={device} className="h-[20px] w-[20px]" />
                {badged(value) && (
                  <span aria-hidden className={`absolute -right-1 -top-0.5 h-[8px] w-[8px] rounded-full ring-2 ring-background ${LEVEL_DOT[dotLevel(value)!]}`} />
                )}
              </span>
            )}
            <span className="flex w-full min-w-0 justify-center">
              <span className="relative min-w-0 max-w-full text-center">
                <span className={`block truncate ${labelClass}`}>{label}</span>
                {!floating && badged(value) && (
                  <TabBadge level={dotLevel(value)!} />
                )}
              </span>
            </span>
            {alerted[value] && <span className="sr-only">, {LEVEL_LABEL[alerted[value]!]}</span>}
          </TabsTrigger>
        ))}
      </TabsList>
      {floating && shownPlanBar && (
        <div className={planBarBox} aria-hidden={!planBar || undefined}>
          <PlanBarWash wash={shownPlanBar.wash} />
          <PlanBarContent model={shownPlanBar} focusable={!!planBar} className="h-full" />
        </div>
      )}
    </div>
  )
}
