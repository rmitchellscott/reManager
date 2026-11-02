import { createContext, useContext, ReactNode } from 'react'
import { FileBrowserError, OperationResult, PackageInfo, TransferFinished, TransferSnapshot } from '@/lib/types'
import type { OperationKind, OperationSpec } from '@/hooks/useOperation'
import type { ConnectionStatus } from '@/hooks/useConnectionEvents'

export interface AppContextValue {
  device: string
  deviceInfo: Record<string, string>
  connectionStatus: ConnectionStatus
  connectionError: string | null
  connectedDeviceId: string

  installedPackages: Map<string, string>
  setInstalledPackages: (pkgs: Map<string, string>) => void
  installTimes: Map<string, number>
  packages: PackageInfo[]
  setPackages: (pkgs: PackageInfo[]) => void
  vellumInstalled: boolean | null
  setVellumInstalled: (v: boolean | null) => void

  commandRunning: boolean
  runningOperation: { kind: OperationKind; target: string } | null
  startOperation: (spec: OperationSpec) => Promise<OperationResult>
  writableRootBusy: boolean

  transfers: TransferSnapshot | null
  lastTransferFinished: TransferFinished | null
  transferError: FileBrowserError | null
  setTransferError: (v: FileBrowserError | null) => void

  appVersion: string
  resolvedAppTheme: string
  resolvedTerminalTheme: string
  resolvedEditorTheme: string
  suppressSystemFileWarnings: boolean

  refreshDeviceState: (deviceType: string) => Promise<void>
  rescanAllPackages: () => Promise<boolean>
  checkDnsProxyError: (dnsError: boolean) => Promise<void>
  refreshAfterPackageChange: (dnsError: boolean) => Promise<void>
  refreshAfterUpgrade: (ok: boolean, dnsError: boolean) => Promise<void>
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
