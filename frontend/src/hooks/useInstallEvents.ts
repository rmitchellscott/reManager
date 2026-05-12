import { useState, useEffect, useRef } from 'react'
import { debugLog } from '@/lib/utils'
import { DialogRequest, InstallProgress, InstallResult, HashtabVersionStatus } from '@/lib/types'

interface UseInstallEventsParams {
  setInstalling: (v: boolean) => void
  setUninstalling: (v: boolean) => void
  setCommandRunning: (v: boolean) => void
  setInstallQueue: (v: Set<string>) => void
  setXochitlRunning: (v: boolean) => void
  setXoviActive: (v: boolean) => void
  setHashtabMismatch: (v: HashtabVersionStatus | null) => void
  setHashtabMissing: (v: boolean) => void
  setCompatibilityStatus: (v: any) => void
  checkDnsProxyError: (dnsError: boolean) => Promise<void>
  rescanAllPackages: () => Promise<boolean>
  osMismatchDetectedRef: React.MutableRefObject<boolean>
  installedPackagesRef: React.MutableRefObject<Map<string, string>>
}

export function useInstallEvents({
  setInstalling,
  setUninstalling,
  setCommandRunning,
  setInstallQueue,
  setXochitlRunning,
  setXoviActive,
  setHashtabMismatch,
  setHashtabMissing,
  setCompatibilityStatus,
  checkDnsProxyError,
  rescanAllPackages,
  osMismatchDetectedRef,
  installedPackagesRef,
}: UseInstallEventsParams) {
  const [output, setOutput] = useState('')
  const [currentComponent, setCurrentComponent] = useState('')
  const [progressStatus, setProgressStatus] = useState('')
  const [progressIndex, setProgressIndex] = useState(0)
  const [progressTotal, setProgressTotal] = useState(0)
  const [progressPercentage, setProgressPercentage] = useState(0)
  const [showProgressModal, setShowProgressModal] = useState(false)
  const [progressModalType, setProgressModalType] = useState<'install' | 'maintenance' | null>(null)
  const [showRebuildDialog, setShowRebuildDialog] = useState(false)
  const [dialogRequest, setDialogRequest] = useState<DialogRequest | null>(null)
  const dialogRespondedRef = useRef(false)
  const [runningHookTitle, setRunningHookTitle] = useState<string | null>(null)
  const [lastInstallSuccess, setLastInstallSuccess] = useState<boolean | null>(null)
  const [lastOperationType, setLastOperationType] = useState<'install' | 'uninstall' | null>(null)
  const [showStartUIDialog, setShowStartUIDialog] = useState(false)

  const setInstallingRef = useRef(setInstalling)
  useEffect(() => { setInstallingRef.current = setInstalling }, [setInstalling])

  const setUninstallingRef = useRef(setUninstalling)
  useEffect(() => { setUninstallingRef.current = setUninstalling }, [setUninstalling])

  const setCommandRunningRef = useRef(setCommandRunning)
  useEffect(() => { setCommandRunningRef.current = setCommandRunning }, [setCommandRunning])

  const setInstallQueueRef = useRef(setInstallQueue)
  useEffect(() => { setInstallQueueRef.current = setInstallQueue }, [setInstallQueue])

  const setXochitlRunningRef = useRef(setXochitlRunning)
  useEffect(() => { setXochitlRunningRef.current = setXochitlRunning }, [setXochitlRunning])

  const setXoviActiveRef = useRef(setXoviActive)
  useEffect(() => { setXoviActiveRef.current = setXoviActive }, [setXoviActive])

  const setHashtabMismatchRef = useRef(setHashtabMismatch)
  useEffect(() => { setHashtabMismatchRef.current = setHashtabMismatch }, [setHashtabMismatch])

  const setHashtabMissingRef = useRef(setHashtabMissing)
  useEffect(() => { setHashtabMissingRef.current = setHashtabMissing }, [setHashtabMissing])

  const setCompatibilityStatusRef = useRef(setCompatibilityStatus)
  useEffect(() => { setCompatibilityStatusRef.current = setCompatibilityStatus }, [setCompatibilityStatus])

  const checkDnsProxyErrorRef = useRef(checkDnsProxyError)
  useEffect(() => { checkDnsProxyErrorRef.current = checkDnsProxyError }, [checkDnsProxyError])

  const rescanAllPackagesRef = useRef(rescanAllPackages)
  useEffect(() => { rescanAllPackagesRef.current = rescanAllPackages }, [rescanAllPackages])

  useEffect(() => {
    if (typeof window.runtime === 'undefined') return

    const unsubscribeProgress = window.runtime.EventsOn('install:progress', (...args: unknown[]) => {
      const progress = args[0] as InstallProgress
      debugLog('Received install:progress:', progress)
      setCurrentComponent(progress.component)
      setProgressStatus(progress.status)
      setProgressTotal(progress.total)

      setProgressIndex(progress.index)
      if (progress.total > 0 && progress.index >= 0) {
        setProgressPercentage(Math.round(((progress.index + 1) / progress.total) * 100))
      }

      if (progress.status === 'downloading' || progress.status === 'transferring' || progress.status === 'installing') {
        if (progress.message) {
          setOutput((prev) => prev + progress.message + '\n')
        }
      } else if (progress.status === 'completed') {
        setOutput((prev) => prev + `${progress.message}\n`)
      } else if (progress.status === 'error') {
        setOutput((prev) => prev + `Error: ${progress.message}\n`)
      }
    })

    const unsubscribeComplete = window.runtime.EventsOn('install:complete', async (...args: unknown[]) => {
      const result = args[0] as InstallResult
      debugLog('Received install:complete:', result)

      setLastInstallSuccess(result.success)

      if (result.success) {
        setOutput((prev) => prev + '\n=== Operation complete! ===\n')
        setProgressPercentage(100)
      } else {
        setOutput((prev) => prev + `\nErrors occurred:\n${result.errors.join('\n')}\n`)
      }

      await checkDnsProxyErrorRef.current(result.dnsError ?? false)

      await rescanAllPackagesRef.current()

      if (osMismatchDetectedRef.current) {
        window.go.main.App.GetPackageCompatibilityStatus().then(status => {
          setCompatibilityStatusRef.current(status)
        }).catch(() => {})
      }

      const hashtabStatus = await window.go.main.App.CheckHashtabVersion()
      if (hashtabStatus.needsRebuild) {
        setHashtabMismatchRef.current(hashtabStatus)
      } else {
        setHashtabMismatchRef.current(null)
      }
      setHashtabMissingRef.current(!hashtabStatus.installed && installedPackagesRef.current.has('qt-resource-rebuilder'))

      const xochitlStatus = await window.go.main.App.GetXochitlStatus()
      setXochitlRunningRef.current(xochitlStatus.running)
      setXoviActiveRef.current(xochitlStatus.xoviActive)

      setInstallingRef.current(false)
      setUninstallingRef.current(false)
      setCommandRunningRef.current(false)
      setCurrentComponent('')
      setProgressStatus('')
      setDialogRequest(null)
      setRunningHookTitle(null)
      setInstallQueueRef.current(new Set())
    })

    const unsubscribeDialog = window.runtime.EventsOn('hook:dialog', (...args: unknown[]) => {
      const dialog = args[0] as DialogRequest
      debugLog('Received hook:dialog:', dialog)
      dialogRespondedRef.current = false
      setDialogRequest(dialog)
      setShowRebuildDialog(true)
    })

    const unsubscribeHookStarted = window.runtime.EventsOn('hook:started', (...args: unknown[]) => {
      const data = args[0] as { title: string }
      setRunningHookTitle(data.title)
    })

    return () => {
      unsubscribeProgress()
      unsubscribeComplete()
      unsubscribeDialog()
      unsubscribeHookStarted()
    }
  }, [])

  return {
    output,
    setOutput,
    currentComponent,
    progressStatus,
    progressIndex,
    setProgressIndex,
    progressTotal,
    setProgressTotal,
    progressPercentage,
    setProgressPercentage,
    showProgressModal,
    setShowProgressModal,
    progressModalType,
    setProgressModalType,
    showRebuildDialog,
    setShowRebuildDialog,
    dialogRequest,
    setDialogRequest,
    dialogRespondedRef,
    runningHookTitle,
    setRunningHookTitle,
    lastInstallSuccess,
    setLastInstallSuccess,
    lastOperationType,
    setLastOperationType,
    showStartUIDialog,
    setShowStartUIDialog,
  }
}
