import { useState, useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { debugLog } from '@/lib/utils'
import { DialogRequest, HashtabVersionStatus, TimezoneStatus, UpdateServiceStatus, PackageInfo } from '@/lib/types'

interface UseMaintenanceEventsParams {
  setOutput: (v: string | ((prev: string) => string)) => void
  setCommandRunning: (v: boolean) => void
  setShowProgressModal: (v: boolean) => void
  setProgressModalType: (v: 'install' | 'maintenance' | null) => void
  setDialogRequest: (v: DialogRequest | null) => void
  setRunningHookTitle: (v: string | null) => void
  setInstalling: (v: boolean) => void
  setHashtabMismatch: (v: HashtabVersionStatus | null) => void
  setHashtabMissing: (v: boolean) => void
  setTimezoneMismatch: (v: TimezoneStatus | null) => void
  setDeviceTimezone: (v: string) => void
  setSelectedTimezone: (v: string) => void
  setUpdateServiceStatus: (v: UpdateServiceStatus) => void
  setXochitlRunning: (v: boolean) => void
  setXoviActive: (v: boolean) => void
  setUpgradesAvailable: (v: boolean) => void
  setOsMismatchDetected: (v: boolean) => void
  osMismatchDetectedRef: React.MutableRefObject<boolean>
  setChecklistLoading: (v: boolean) => void
  setCompatibilityStatus: (v: any) => void
  setDeviceInfo: (v: Record<string, string>) => void
  setPackages: (v: PackageInfo[]) => void
  checkDnsProxyError: (dnsError: boolean) => Promise<void>
  rescanAllPackages: () => Promise<boolean>
  deviceRef: React.MutableRefObject<string>
  installedPackagesRef: React.MutableRefObject<Map<string, string>>
  selectedTimezoneRef: React.MutableRefObject<string>
  setActiveTab: (v: 'mods' | 'maintenance' | 'utilities') => void
}

export function useMaintenanceEvents({
  setOutput,
  setCommandRunning,
  setShowProgressModal,
  setProgressModalType,
  setDialogRequest,
  setRunningHookTitle,
  setInstalling,
  setHashtabMismatch,
  setHashtabMissing,
  setTimezoneMismatch,
  setDeviceTimezone,
  setSelectedTimezone,
  setUpdateServiceStatus,
  setXochitlRunning,
  setXoviActive,
  setUpgradesAvailable,
  setOsMismatchDetected,
  osMismatchDetectedRef,
  setChecklistLoading,
  setCompatibilityStatus,
  setDeviceInfo,
  setPackages,
  checkDnsProxyError,
  rescanAllPackages,
  deviceRef,
  installedPackagesRef,
  selectedTimezoneRef,
  setActiveTab,
}: UseMaintenanceEventsParams) {
  const [maintenanceOutput, setMaintenanceOutput] = useState('')
  const [commandContext, setCommandContext] = useState<'install' | 'maintenance' | null>(null)
  const commandContextRef = useRef<'install' | 'maintenance' | null>(null)
  const [currentRunningCommand, setCurrentRunningCommand] = useState<{
    componentId: string
    commandId: string
  } | null>(null)
  const [runningSystemTask, setRunningSystemTask] = useState<string | null>(null)
  const runningSystemTaskRef = useRef<string | null>(null)
  const [settingTimezone, setSettingTimezone] = useState(false)
  const settingTimezoneRef = useRef(false)
  const manuallyStoppedRef = useRef(false)
  const [rescanning, setRescanning] = useState(false)

  useEffect(() => {
    commandContextRef.current = commandContext
  }, [commandContext])

  const setOutputRef = useRef(setOutput)
  useEffect(() => { setOutputRef.current = setOutput }, [setOutput])

  const setCommandRunningRef = useRef(setCommandRunning)
  useEffect(() => { setCommandRunningRef.current = setCommandRunning }, [setCommandRunning])

  const setShowProgressModalRef = useRef(setShowProgressModal)
  useEffect(() => { setShowProgressModalRef.current = setShowProgressModal }, [setShowProgressModal])

  const setProgressModalTypeRef = useRef(setProgressModalType)
  useEffect(() => { setProgressModalTypeRef.current = setProgressModalType }, [setProgressModalType])

  const setDialogRequestRef = useRef(setDialogRequest)
  useEffect(() => { setDialogRequestRef.current = setDialogRequest }, [setDialogRequest])

  const setRunningHookTitleRef = useRef(setRunningHookTitle)
  useEffect(() => { setRunningHookTitleRef.current = setRunningHookTitle }, [setRunningHookTitle])

  const setInstallingRef = useRef(setInstalling)
  useEffect(() => { setInstallingRef.current = setInstalling }, [setInstalling])

  const setHashtabMismatchRef = useRef(setHashtabMismatch)
  useEffect(() => { setHashtabMismatchRef.current = setHashtabMismatch }, [setHashtabMismatch])

  const setHashtabMissingRef = useRef(setHashtabMissing)
  useEffect(() => { setHashtabMissingRef.current = setHashtabMissing }, [setHashtabMissing])

  const setTimezoneMismatchRef = useRef(setTimezoneMismatch)
  useEffect(() => { setTimezoneMismatchRef.current = setTimezoneMismatch }, [setTimezoneMismatch])

  const setDeviceTimezoneRef = useRef(setDeviceTimezone)
  useEffect(() => { setDeviceTimezoneRef.current = setDeviceTimezone }, [setDeviceTimezone])

  const setSelectedTimezoneRef = useRef(setSelectedTimezone)
  useEffect(() => { setSelectedTimezoneRef.current = setSelectedTimezone }, [setSelectedTimezone])

  const setUpdateServiceStatusRef = useRef(setUpdateServiceStatus)
  useEffect(() => { setUpdateServiceStatusRef.current = setUpdateServiceStatus }, [setUpdateServiceStatus])

  const setXochitlRunningRef = useRef(setXochitlRunning)
  useEffect(() => { setXochitlRunningRef.current = setXochitlRunning }, [setXochitlRunning])

  const setXoviActiveRef = useRef(setXoviActive)
  useEffect(() => { setXoviActiveRef.current = setXoviActive }, [setXoviActive])

  const setUpgradesAvailableRef = useRef(setUpgradesAvailable)
  useEffect(() => { setUpgradesAvailableRef.current = setUpgradesAvailable }, [setUpgradesAvailable])

  const setOsMismatchDetectedRef = useRef(setOsMismatchDetected)
  useEffect(() => { setOsMismatchDetectedRef.current = setOsMismatchDetected }, [setOsMismatchDetected])

  const setChecklistLoadingRef = useRef(setChecklistLoading)
  useEffect(() => { setChecklistLoadingRef.current = setChecklistLoading }, [setChecklistLoading])

  const setCompatibilityStatusRef = useRef(setCompatibilityStatus)
  useEffect(() => { setCompatibilityStatusRef.current = setCompatibilityStatus }, [setCompatibilityStatus])

  const setDeviceInfoRef = useRef(setDeviceInfo)
  useEffect(() => { setDeviceInfoRef.current = setDeviceInfo }, [setDeviceInfo])

  const setPackagesRef = useRef(setPackages)
  useEffect(() => { setPackagesRef.current = setPackages }, [setPackages])

  const checkDnsProxyErrorRef = useRef(checkDnsProxyError)
  useEffect(() => { checkDnsProxyErrorRef.current = checkDnsProxyError }, [checkDnsProxyError])

  const rescanAllPackagesRef = useRef(rescanAllPackages)
  useEffect(() => { rescanAllPackagesRef.current = rescanAllPackages }, [rescanAllPackages])

  const setActiveTabRef = useRef(setActiveTab)
  useEffect(() => { setActiveTabRef.current = setActiveTab }, [setActiveTab])

  useEffect(() => {
    if (typeof window.runtime === 'undefined') return

    const unsubscribeOutput = window.runtime.EventsOn('command:output', (...args: unknown[]) => {
      const data = args[0] as string
      debugLog('Received command:output:', data)

      if (commandContextRef.current === 'maintenance') {
        setMaintenanceOutput((prev) => prev + data)
      } else {
        setOutputRef.current((prev: string) => prev + data)
      }
    })

    const unsubscribeDone = window.runtime.EventsOn('command:done', async (...args: unknown[]) => {
      const success = args[0] as boolean
      debugLog('Received command:done:', success)

      if (runningSystemTaskRef.current || settingTimezoneRef.current) {
        if (!success && !manuallyStoppedRef.current) {
          setMaintenanceOutput((prev) => prev + '\n[Command failed]\n')
          setShowProgressModalRef.current(true)
          runningSystemTaskRef.current = null
          settingTimezoneRef.current = false
          setCommandRunningRef.current(false)
          setRunningSystemTask(null)
          setSettingTimezone(false)
          const status = await window.go.main.App.GetUpdateServiceStatus()
          setUpdateServiceStatusRef.current(status)
          const xochitlStatus = await window.go.main.App.GetXochitlStatus()
          setXochitlRunningRef.current(xochitlStatus.running)
          setXoviActiveRef.current(xochitlStatus.xoviActive)
        }
        return
      }

      if (!success && manuallyStoppedRef.current) {
        manuallyStoppedRef.current = false
        setShowProgressModalRef.current(false)
        setMaintenanceOutput('')
        setProgressModalTypeRef.current(null)
        setCommandRunningRef.current(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
        return
      }
      if (!success) {
        if (commandContextRef.current === 'maintenance') {
          setMaintenanceOutput((prev) => prev + '\n[Command failed]\n')
          setShowProgressModalRef.current(true)
        } else {
          setOutputRef.current((prev: string) => prev + '\n[Command failed]\n')
        }
      }
      if (commandContextRef.current === 'maintenance') {
        const [hashtabStatus, tzStatus] = await Promise.all([
          window.go.main.App.CheckHashtabVersion(),
          window.go.main.App.GetTimezoneStatus(),
        ])
        let updateStatus = await window.go.main.App.GetUpdateServiceStatus()
        const maxAttempts = 6
        for (let i = 0; i < maxAttempts && updateStatus.enabled !== updateStatus.running; i++) {
          await new Promise(r => setTimeout(r, 500))
          updateStatus = await window.go.main.App.GetUpdateServiceStatus()
        }
        setUpdateServiceStatusRef.current(updateStatus)
        const xochitlStatus = await window.go.main.App.GetXochitlStatus()
        setXochitlRunningRef.current(xochitlStatus.running)
        setXoviActiveRef.current(xochitlStatus.xoviActive)
        if (hashtabStatus.needsRebuild) {
          setHashtabMismatchRef.current(hashtabStatus)
        } else {
          setHashtabMismatchRef.current(null)
        }
        setHashtabMissingRef.current(!hashtabStatus.installed && installedPackagesRef.current.has('qt-resource-rebuilder'))

        if (tzStatus.needsUpdate) {
          setTimezoneMismatchRef.current(tzStatus)
        } else {
          setTimezoneMismatchRef.current(null)
        }
        if (tzStatus.deviceTimezone) {
          setDeviceTimezoneRef.current(tzStatus.deviceTimezone)
          if (!selectedTimezoneRef.current) {
            setSelectedTimezoneRef.current(tzStatus.savedTimezone || tzStatus.deviceTimezone)
          }
        }
        setCommandRunningRef.current(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
      } else {
        const xochitlStatus = await window.go.main.App.GetXochitlStatus()
        setXochitlRunningRef.current(xochitlStatus.running)
        setXoviActiveRef.current(xochitlStatus.xoviActive)
        setCommandRunningRef.current(false)
        setRunningSystemTask(null)
        setSettingTimezone(false)
        setCommandContext(null)
      }
    })

    const unsubscribeTimezoneComplete = window.runtime.EventsOn('timezone:complete', (...args: unknown[]) => {
      const timezone = args[0] as string
      debugLog('Timezone set complete:', timezone)
      settingTimezoneRef.current = false
      setSettingTimezone(false)
      setCommandRunningRef.current(false)
      setTimezoneMismatchRef.current(null)
      setDeviceTimezoneRef.current(timezone)
      window.go.main.App.GetTimezoneStatus().then(status => {
        if (status.needsUpdate) {
          setTimezoneMismatchRef.current(status)
        }
      }).catch(() => {})
    })

    const unsubscribeTimezoneError = window.runtime.EventsOn('timezone:error', (...args: unknown[]) => {
      console.error('Timezone error:', args[0])
      settingTimezoneRef.current = false
      setSettingTimezone(false)
      setCommandRunningRef.current(false)
    })

    const unsubscribeUpgradeBlocked = window.runtime.EventsOn('upgrade:blocked', (...args: unknown[]) => {
      const compat = args[0] as {
        compatible: string[]
        incompatible: string[]
        noConstraint: string[]
        fetchFailed: boolean
      }
      debugLog('Received upgrade:blocked:', compat)
      setChecklistLoadingRef.current(false)
    })

    const unsubscribeUpgradeError = window.runtime.EventsOn('upgrade:error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received upgrade:error:', errMsg)
      setChecklistLoadingRef.current(false)
    })

    const unsubscribeUpgradeComplete = window.runtime.EventsOn('upgrade:complete', async (...args: unknown[]) => {
      const result = args[0] as { success: boolean; dnsError: boolean }
      debugLog('Received upgrade:complete:', result)
      setChecklistLoadingRef.current(false)
      if (result.success) {
        setUpgradesAvailableRef.current(false)
        setOsMismatchDetectedRef.current(false)
        osMismatchDetectedRef.current = false
        setCompatibilityStatusRef.current(null)
        setRescanning(true)
        try {
          const info = await window.go.main.App.GetDeviceInfo()
          setDeviceInfoRef.current(info)
          const arch = await window.go.main.App.GetDeviceArchitecture(deviceRef.current)
          const filteredPkgs = await window.go.main.App.GetPackages(deviceRef.current, info.firmware || '', arch)
          setPackagesRef.current(filteredPkgs || [])
        } catch (err) {
          debugLog('Failed to refresh device info after upgrade:', err)
        }
        await rescanAllPackagesRef.current()
        setRescanning(false)
      }
      setCommandRunningRef.current(false)
      setCommandContext(null)
      await checkDnsProxyErrorRef.current(result.dnsError)
    })

    const unsubscribePackageUpgradeComplete = window.runtime.EventsOn('package-upgrade:complete', async (...args: unknown[]) => {
      const result = args[0] as { success: boolean; dnsError: boolean }
      debugLog('Received package-upgrade:complete:', result)
      if (result.success) {
        setUpgradesAvailableRef.current(false)
        setRescanning(true)
        try {
          const info = await window.go.main.App.GetDeviceInfo()
          setDeviceInfoRef.current(info)
          const arch = await window.go.main.App.GetDeviceArchitecture(deviceRef.current)
          const filteredPkgs = await window.go.main.App.GetPackages(deviceRef.current, info.firmware || '', arch)
          setPackagesRef.current(filteredPkgs || [])
        } catch (err) {
          debugLog('Failed to refresh device info after package upgrade:', err)
        }
        await rescanAllPackagesRef.current()
        setRescanning(false)
      }
      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunningRef.current(xochitlStatus.running)
      setXoviActiveRef.current(xochitlStatus.xoviActive)
      setCommandRunningRef.current(false)
      setCommandContext(null)
      await checkDnsProxyErrorRef.current(result.dnsError)
    })

    const unsubscribeSystemTaskComplete = window.runtime.EventsOn('systemtask:complete', async (...args: unknown[]) => {
      const success = args[0] as boolean
      debugLog('Received systemtask:complete:', success)

      const [hashtabStatus, tzStatus] = await Promise.all([
        window.go.main.App.CheckHashtabVersion(),
        window.go.main.App.GetTimezoneStatus(),
      ])
      let updateStatus = await window.go.main.App.GetUpdateServiceStatus()
      const maxAttempts = 6
      for (let i = 0; i < maxAttempts && updateStatus.enabled !== updateStatus.running; i++) {
        await new Promise(r => setTimeout(r, 500))
        updateStatus = await window.go.main.App.GetUpdateServiceStatus()
      }
      setUpdateServiceStatusRef.current(updateStatus)
      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunningRef.current(xochitlStatus.running)
      setXoviActiveRef.current(xochitlStatus.xoviActive)
      if (hashtabStatus.needsRebuild) {
        setHashtabMismatchRef.current(hashtabStatus)
      } else {
        setHashtabMismatchRef.current(null)
      }
      setHashtabMissingRef.current(!hashtabStatus.installed && installedPackagesRef.current.has('qt-resource-rebuilder'))
      if (tzStatus.needsUpdate) {
        setTimezoneMismatchRef.current(tzStatus)
      } else {
        setTimezoneMismatchRef.current(null)
      }
      if (tzStatus.deviceTimezone) {
        setDeviceTimezoneRef.current(tzStatus.deviceTimezone)
        if (!selectedTimezoneRef.current) {
          setSelectedTimezoneRef.current(tzStatus.savedTimezone || tzStatus.deviceTimezone)
        }
      }
      runningSystemTaskRef.current = null
      setCommandRunningRef.current(false)
      setRunningSystemTask(null)
    })

    const unsubscribeUpgradesAvailable = window.runtime.EventsOn('packages:upgrades-available', (...args: unknown[]) => {
      const data = args[0] as { packages: string[] }
      debugLog('Received packages:upgrades-available:', data)
      setUpgradesAvailableRef.current(true)
      toast.info(`${data.packages.length} package upgrade${data.packages.length !== 1 ? 's' : ''} available`, {
        action: {
          label: 'Go to Maintenance',
          onClick: () => setActiveTabRef.current('maintenance'),
        },
        duration: 6000,
      })
    })

    return () => {
      unsubscribeOutput()
      unsubscribeDone()
      unsubscribeTimezoneComplete()
      unsubscribeTimezoneError()
      unsubscribeUpgradeBlocked()
      unsubscribeUpgradeError()
      unsubscribeUpgradeComplete()
      unsubscribePackageUpgradeComplete()
      unsubscribeSystemTaskComplete()
      unsubscribeUpgradesAvailable()
    }
  }, [])

  return {
    maintenanceOutput,
    setMaintenanceOutput,
    commandContext,
    setCommandContext,
    commandContextRef,
    currentRunningCommand,
    setCurrentRunningCommand,
    runningSystemTask,
    setRunningSystemTask,
    runningSystemTaskRef,
    settingTimezone,
    setSettingTimezone,
    settingTimezoneRef,
    manuallyStoppedRef,
    rescanning,
    setRescanning,
  }
}
