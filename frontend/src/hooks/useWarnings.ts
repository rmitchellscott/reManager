import { useState, useEffect, useRef } from 'react'
import { debugLog } from '@/lib/utils'
import { HashtabVersionStatus, TimezoneStatus, UpdateServiceStatus } from '@/lib/types'

export function useWarnings() {
  const [warningsChecked, setWarningsChecked] = useState(false)
  const [osUpgradeDetected, setOsUpgradeDetected] = useState(false)
  const [prevOsVersion, setPrevOsVersion] = useState('')
  const [newOsVersion, setNewOsVersion] = useState('')
  const [osMismatchDetected, setOsMismatchDetected] = useState(false)
  const osMismatchDetectedRef = useRef(false)
  const [storedOsVersion, setStoredOsVersion] = useState('')
  const [currentOsVersion, setCurrentOsVersion] = useState('')
  const [checklistLoading, setChecklistLoading] = useState(false)
  const [compatibilityStatus, setCompatibilityStatus] = useState<{
    installedPackages: string[]
    compatiblePackages: string[]
    incompatiblePackages: string[]
    currentOsVersion: string
    storedOsVersion: string
    fetchFailed: boolean
    statusMap?: Record<string, string>
  } | null>(null)
  const [hashtabMismatch, setHashtabMismatch] = useState<HashtabVersionStatus | null>(null)
  const [hashtabMissing, setHashtabMissing] = useState(false)
  const [timezoneMismatch, setTimezoneMismatch] = useState<TimezoneStatus | null>(null)
  const [deviceTimezone, setDeviceTimezone] = useState('')
  const [selectedTimezone, setSelectedTimezone] = useState('')
  const [reenableStatus, setReenableStatus] = useState('')
  const [showAutoUpdateBanner, setShowAutoUpdateBanner] = useState(false)
  const [updateServiceStatus, setUpdateServiceStatus] = useState<UpdateServiceStatus>({
    enabled: false,
    running: false,
  })
  const [xochitlRunning, setXochitlRunning] = useState(true)
  const [xoviActive, setXoviActive] = useState(false)
  const [guideOffer, setGuideOffer] = useState<'install' | 'update' | null>(null)
  const [guideInstalling, setGuideInstalling] = useState(false)
  const [showGuideRestartDialog, setShowGuideRestartDialog] = useState(false)

  useEffect(() => {
    if (typeof window.runtime === 'undefined') {
      debugLog('window.runtime is undefined, events will not work')
      return
    }
    debugLog('Setting up event listeners')

    const unsubscribeConnectWarnings = window.runtime.EventsOn('connect:warnings', (...args: unknown[]) => {
      const w = args[0] as {
        osMismatch?: { prevVersion: string; newVersion: string }
        compatibilityStatus?: {
          installedPackages: string[]
          compatiblePackages: string[]
          incompatiblePackages: string[]
          currentOsVersion: string
          storedOsVersion: string
          fetchFailed: boolean
          statusMap?: Record<string, string>
        }
        reenableStatus?: string
        hashtabMismatch?: HashtabVersionStatus
        hashtabMissing?: boolean
        autoUpdateEnabled?: { enabled: boolean; running: boolean }
        timezoneStatus?: TimezoneStatus
        timezoneMismatch?: TimezoneStatus
        xochitlNotRunning?: boolean
        xoviNotRunning?: boolean
      }
      debugLog('Received connect:warnings:', w)
      setWarningsChecked(true)

      if (w.osMismatch) {
        debugLog('connect:warnings — setting osMismatchDetected=true', w.osMismatch)
        setOsMismatchDetected(true)
        osMismatchDetectedRef.current = true
        setStoredOsVersion(w.osMismatch.prevVersion)
        setCurrentOsVersion(w.osMismatch.newVersion)
        if (w.compatibilityStatus) {
          debugLog('connect:warnings — compatibilityStatus received inline')
          setCompatibilityStatus(w.compatibilityStatus)
        }
      } else {
        debugLog('connect:warnings — no osMismatch, clearing')
        setOsMismatchDetected(false)
        osMismatchDetectedRef.current = false
      }
      if (w.reenableStatus) {
        setReenableStatus(w.reenableStatus)
      }
      if (w.hashtabMismatch) {
        setHashtabMismatch(w.hashtabMismatch)
      } else {
        setHashtabMismatch(null)
      }
      setHashtabMissing(!!w.hashtabMissing)
      if (w.autoUpdateEnabled) {
        setShowAutoUpdateBanner(true)
      } else {
        setShowAutoUpdateBanner(false)
      }
      if (w.timezoneStatus?.deviceTimezone) {
        setDeviceTimezone(w.timezoneStatus.deviceTimezone)
        setSelectedTimezone(w.timezoneStatus.savedTimezone || w.timezoneStatus.deviceTimezone)
      }
      if (w.timezoneMismatch) {
        setTimezoneMismatch(w.timezoneMismatch)
        setDeviceTimezone(w.timezoneMismatch.deviceTimezone)
        setSelectedTimezone(w.timezoneMismatch.savedTimezone || w.timezoneMismatch.deviceTimezone)
      } else {
        setTimezoneMismatch(null)
      }
      setXochitlRunning(!w.xochitlNotRunning)
      setXoviActive(!w.xoviNotRunning)

    })

    const unsubscribeGuideOffer = window.runtime.EventsOn('guide:offer', (...args: unknown[]) => {
      const data = args[0] as { type: 'install' | 'update' }
      setGuideOffer(data.type)
    })

    return () => {
      unsubscribeConnectWarnings()
      unsubscribeGuideOffer()
    }
  }, [])

  return {
    warningsChecked, setWarningsChecked,
    osUpgradeDetected, setOsUpgradeDetected,
    prevOsVersion, setPrevOsVersion,
    newOsVersion, setNewOsVersion,
    osMismatchDetected, setOsMismatchDetected,
    osMismatchDetectedRef,
    storedOsVersion, setStoredOsVersion,
    currentOsVersion, setCurrentOsVersion,
    checklistLoading, setChecklistLoading,
    compatibilityStatus, setCompatibilityStatus,
    hashtabMismatch, setHashtabMismatch,
    hashtabMissing, setHashtabMissing,
    timezoneMismatch, setTimezoneMismatch,
    deviceTimezone, setDeviceTimezone,
    selectedTimezone, setSelectedTimezone,
    reenableStatus, setReenableStatus,
    showAutoUpdateBanner, setShowAutoUpdateBanner,
    updateServiceStatus, setUpdateServiceStatus,
    xochitlRunning, setXochitlRunning,
    xoviActive, setXoviActive,
    guideOffer, setGuideOffer,
    guideInstalling, setGuideInstalling,
    showGuideRestartDialog, setShowGuideRestartDialog,
  }
}
