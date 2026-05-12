export type Step = 'connect' | 'select' | 'install' | 'done'

export interface MaintenanceCommandInfo {
  id: string
  label: string
  description: string
  requiresTerminal: boolean
  allowStop: boolean
  hook?: string
}

export interface SystemTaskInfo {
  id: string
  label: string
  description: string
  requiresTerminal: boolean
  needsWriteableRoot: boolean
}

export interface SSHKey {
  path: string
  name: string
}

export interface SavedDevice {
  id: string
  name: string
  host: string
  authType: 'password' | 'key' | 'agent'
  keyPath?: string
  lastConnected?: number
}

export interface UpdateServiceStatus {
  enabled: boolean
  running: boolean
}

export interface InstallProgress {
  component: string
  index: number
  total: number
  status: string
  message: string
}

export interface InstallResult {
  success: boolean
  errors: string[]
  dnsError?: boolean
}

export interface DialogActionRequest {
  id: string
  label: string
  type: string
  value: string
}

export interface DialogRequest {
  title: string
  message: string
  note: string
  steps: string[]
  confirmText: string
  cancelText: string
  inProgressMessage: string
  infoOnly: boolean
  installFlow?: boolean
  actions: DialogActionRequest[]
}

export interface InstallSimulationResult {
  packages: string[]
  requested: string[]
}

export interface UninstallSimulationResult {
  packages: string[]
  blocked: Record<string, string[]> | null
  recursivePackages: string[] | null
  worldDeps: string[] | null
  allAffected: string[] | null
}

export interface InstalledPackagesResult {
  packages: string[]
  osUpgraded: boolean
  prevVersion: string
  newVersion: string
}

export interface HashtabVersionStatus {
  installed: boolean
  hashtabVersion: string
  firmwareVersion: string
  needsRebuild: boolean
}

export interface TimezoneStatus {
  deviceTimezone: string
  savedTimezone: string
  needsUpdate: boolean
}

export interface PackageInfo {
  name: string
  version: string
  description: string
  upstreamAuthor: string
  categories: string[]
  url: string
  license: string
  devices: string[]
  depends: string[]
  conflicts: string[]
  osMin: string | null
  osMax: string | null
  osConstraints: { version: string; operator: '>=' | '<' | '>' | '<=' | '=' }[] | null
  compatible: boolean
  incompatibleReason?: string
  status: string
  donateUrl: string | null
  readmeUrl: string | null
}

export interface CompatibilityResult {
  compatible: string[]
  incompatible: string[]
  noConstraint: string[]
  fetchFailed: boolean
  statusMap: Record<string, string>
}
