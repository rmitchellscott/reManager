import { useState, useEffect, useRef } from 'react'
import { debugLog } from '@/lib/utils'
import { HashtabVersionStatus, OSChannelStatus, PackageCompatibilityStatus, ReenableStatus, TimezoneStatus, UpdateServiceStatus, UserGuideStatus } from '@/lib/types'
import { onEvent } from '@/lib/events'

export function useWarnings() {
  const [warningsChecked, setWarningsChecked] = useState(false)
  const warningsCheckedRef = useRef(false)
  const [osMismatchDetected, setOsMismatchDetected] = useState(false)
  const osMismatchDetectedRef = useRef(false)
  const [storedOsVersion, setStoredOsVersion] = useState('')
  const [currentOsVersion, setCurrentOsVersion] = useState('')
  const [compatibilityStatus, setCompatibilityStatus] = useState<PackageCompatibilityStatus | null>(null)
  const [hashtab, setHashtab] = useState<HashtabVersionStatus | null>(null)
  const [osChannel, setOsChannel] = useState<OSChannelStatus | null>(null)
  const [incompatibleWithLatestOS, setIncompatibleWithLatestOS] = useState<string[]>([])
  const [timezoneMismatch, setTimezoneMismatch] = useState<TimezoneStatus | null>(null)
  const [deviceTimezone, setDeviceTimezone] = useState('')
  const [selectedTimezone, setSelectedTimezone] = useState('')
  const [reenableStatus, setReenableStatus] = useState(ReenableStatus.$zero)
  const [updateServiceStatus, setUpdateServiceStatus] = useState<UpdateServiceStatus>({
    enabled: false,
    running: false,
  })
  const [xochitlRunning, setXochitlRunning] = useState(true)
  const [xoviActive, setXoviActive] = useState(false)
  const [activeLauncher, setActiveLauncher] = useState('')
  const [currentLauncher, setCurrentLauncher] = useState('')
  const [guideStatus, setGuideStatus] = useState<UserGuideStatus | null>(null)
  const [showGuideRestartDialog, setShowGuideRestartDialog] = useState(false)

  useEffect(() => {
    debugLog('Setting up event listeners')

    const unsubscribeConnectWarnings = onEvent('connect:warnings', w => {
      debugLog('Received connect:warnings:', w)
      setWarningsChecked(true)
      warningsCheckedRef.current = true

      if (w.osMismatch) {
        debugLog('connect:warnings: setting osMismatchDetected=true', w.osMismatch)
        setOsMismatchDetected(true)
        osMismatchDetectedRef.current = true
        setStoredOsVersion(w.osMismatch.prevVersion)
        setCurrentOsVersion(w.osMismatch.newVersion)
        if (w.compatibilityStatus) {
          debugLog('connect:warnings: compatibilityStatus received inline')
          setCompatibilityStatus(w.compatibilityStatus)
        }
      } else {
        debugLog('connect:warnings: no osMismatch, clearing')
        setOsMismatchDetected(false)
        osMismatchDetectedRef.current = false
      }
      if (w.reenableStatus) {
        setReenableStatus(w.reenableStatus)
      }
      setHashtab(w.hashtab ?? null)
      if (w.updateService) {
        setUpdateServiceStatus(w.updateService)
      }
      if (w.timezoneStatus?.deviceTimezone) {
        setDeviceTimezone(w.timezoneStatus.deviceTimezone)
        setSelectedTimezone(w.timezoneStatus.savedTimezone || w.timezoneStatus.deviceTimezone)
      }
      setTimezoneMismatch(w.timezoneStatus?.needsUpdate ? w.timezoneStatus : null)
      if (w.xochitl) {
        setXochitlRunning(w.xochitl.running)
        setXoviActive(w.xochitl.xoviActive)
        setActiveLauncher(w.xochitl.activeLauncher || '')
        setCurrentLauncher(w.xochitl.currentLauncher || '')
      }
      setOsChannel(w.osChannel ?? null)
      setIncompatibleWithLatestOS(w.incompatibleWithLatestOS ?? [])
    })

    const unsubscribeGuideStatus = onEvent('guide:status', setGuideStatus)

    return () => {
      unsubscribeConnectWarnings()
      unsubscribeGuideStatus()
    }
  }, [])

  return {
    warningsChecked, setWarningsChecked,
    warningsCheckedRef,
    osMismatchDetected, setOsMismatchDetected,
    osMismatchDetectedRef,
    storedOsVersion, setStoredOsVersion,
    currentOsVersion, setCurrentOsVersion,
    compatibilityStatus, setCompatibilityStatus,
    hashtab, setHashtab,
    osChannel, setOsChannel,
    incompatibleWithLatestOS, setIncompatibleWithLatestOS,
    timezoneMismatch, setTimezoneMismatch,
    deviceTimezone, setDeviceTimezone,
    selectedTimezone, setSelectedTimezone,
    reenableStatus, setReenableStatus,
    updateServiceStatus, setUpdateServiceStatus,
    xochitlRunning, setXochitlRunning,
    xoviActive, setXoviActive,
    activeLauncher, setActiveLauncher,
    currentLauncher, setCurrentLauncher,
    guideStatus, setGuideStatus,
    showGuideRestartDialog, setShowGuideRestartDialog,
  }
}
