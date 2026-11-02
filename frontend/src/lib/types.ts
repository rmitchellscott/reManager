export type {
  BackupInfo,
  DeviceOutlook,
  FileBrowserError,
  FileInfo,
  FollowInfo,
  HashtabVersionStatus,
  MaintenanceCommandInfo,
  OSChannelStatus,
  PackageInfo,
  PlatformCapabilities,
  SettingsInfo,
  SSHKey,
  SystemTaskInfo,
  TimezoneStatus,
  TransferFinished,
  TransferSnapshot,
  UpgradeCandidate,
  UserGuideStatus,
  InstallProgress,
  DialogRequest,
  OperationHook,
  OperationResult,
  PackageCompatibilityStatus,
  RestoreFile,
  OSInstallProgress,
} from '@bindings/reManager/models'
export { DialogSeverity } from '@bindings/reManager/internal/component/models'
export { ErrorCode } from '@bindings/reManager/internal/errors/models'
export { ColorScheme, InstallStatus, OSInstallStage, TransferKind, TransferState } from '@bindings/reManager/models'
export { SyncKey } from '@bindings/reManager/internal/storage/models'
export type {
  CompatibilityResultJSON as CompatibilityResult,
  ManagedKeyInfo as ManagedKey,
  SavedDeviceInfo as SavedDevice,
} from '@bindings/reManager/models'

export { ReenableStatus } from '@bindings/reManager/internal/vellum/models'
export type {
  PackageConflict,
  PackageRelease,
  QueueConflictEntry,
  VirtualChoice,
} from '@bindings/reManager/internal/vellum/models'
export type {
  UpdateServiceStatus,
} from '@bindings/reManager/internal/support/models'
export type {
  Progress as BackupProgress,
} from '@bindings/reManager/internal/backup/models'
export { Phase as BackupPhase } from '@bindings/reManager/internal/backup/models'
export type { BundleRecord } from '@bindings/reManager/internal/storage/models'
export type { OSVersionInfo } from '@bindings/reManager/internal/swupdate/models'
export type {
  Info as PartitionSlot,
  SystemInfo as PartitionInfo,
} from '@bindings/github.com/rmitchellscott/remarkable-go/partition/models'

export type Step = 'connect' | 'browse' | 'select' | 'install' | 'done'


