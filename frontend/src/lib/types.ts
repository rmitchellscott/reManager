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
  success?: boolean
  primaryAction?: string
  actions: DialogActionRequest[]
}

export interface QueueConflictEntry {
  queueEntry: string
  package: string
  blocks: string
}

export interface InstallSimulationResult {
  packages: string[]
  requested: string[]
  error?: string
  conflicts?: QueueConflictEntry[]
  unresolvedVirtuals?: string[]
}

export interface InstallSet {
  resolved: string[]
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
  provides: string[]
  osMin: string | null
  osMax: string | null
  osConstraints: { version: string; operator: '>=' | '<' | '>' | '<=' | '=' }[] | null
  compatible: boolean
  incompatibleReason?: string
  status: string
  donateUrl: string | null
  readmeUrl: string | null
}

export interface ProviderOption {
  name: string
  version: string
  description: string
  compatible: boolean
  incompatibleReason?: string
  osMin: string | null
  osMax: string | null
  osConstraints: { version: string; operator: '>=' | '<' | '>' | '<=' | '=' }[] | null
}

export interface PackageConflict {
  conflictsWith: string
  queued: boolean
}

export interface VirtualChoice {
  virtual: string
  requiredBy: string[]
  providers: ProviderOption[]
  default?: string
}

export interface CompatibilityResult {
  compatible: string[]
  incompatible: string[]
  noConstraint: string[]
  fetchFailed: boolean
  statusMap: Record<string, string>
}

export interface TransferView {
  id: string
  kind: 'upload' | 'download'
  name: string
  state: 'queued' | 'starting' | 'running' | 'done' | 'failed' | 'canceled'
  bytesDone: number
  bytesTotal: number
  filesDone: number
  filesTotal: number
  percentage: number
  error?: string
  failedFiles?: string[]
  isDir: boolean
}

export interface TransferSnapshot {
  transfers: TransferView[]
  activeCount: number
  bytesDone: number
  bytesTotal: number
  percentage: number
  bytesPerSecond: number
  secondsRemaining: number
}
