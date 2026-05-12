import { createContext, useContext, ReactNode } from 'react'
import { PackageInfo } from '@/lib/types'

export interface AppContextValue {
  device: string
  deviceInfo: Record<string, string>
  connectionStatus: 'connected' | 'lost' | 'reconnecting' | 'failed'
  connectionError: string | null
  connectedDeviceId: string

  installedPackages: Map<string, string>
  setInstalledPackages: (pkgs: Map<string, string>) => void
  packages: PackageInfo[]
  setPackages: (pkgs: PackageInfo[]) => void
  vellumInstalled: boolean | null
  setVellumInstalled: (v: boolean | null) => void

  commandRunning: boolean
  setCommandRunning: (v: boolean) => void
  installing: boolean
  uninstalling: boolean
  writeableRootBusy: boolean

  appVersion: string
  resolvedAppTheme: string
  resolvedTerminalTheme: string
  resolvedEditorTheme: string
  tabVisibility: Record<string, boolean>
  suppressSystemFileWarnings: boolean

  refreshDeviceState: (deviceType: string) => Promise<void>
  rescanAllPackages: () => Promise<boolean>
  startMaintenanceOperation: (options?: { showModal?: boolean; resetProgress?: boolean }) => void
  checkDnsProxyError: (dnsError: boolean) => Promise<void>
}

const AppContext = createContext<AppContextValue | null>(null)

export function useAppContext(): AppContextValue {
  const ctx = useContext(AppContext)
  if (!ctx) {
    throw new Error('useAppContext must be used within an AppProvider')
  }
  return ctx
}

export function AppProvider({ value, children }: { value: AppContextValue; children: ReactNode }) {
  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}
