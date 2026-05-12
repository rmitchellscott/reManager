import { useState, useEffect, useRef } from 'react'
import { debugLog } from '@/lib/utils'

interface UseVellumEventsParams {
  setVellumInstalled: (v: boolean | null) => void
  setInstalledPackages: (pkgs: Map<string, string>) => void
  setShowSettingsDialog: (v: boolean) => void
  rescanAllPackages: () => Promise<boolean>
}

export function useVellumEvents({
  setVellumInstalled,
  setInstalledPackages,
  setShowSettingsDialog,
  rescanAllPackages,
}: UseVellumEventsParams) {
  const [bootstrapping, setBootstrapping] = useState(false)
  const [bootstrapOutput, setBootstrapOutput] = useState('')
  const [bootstrapError, setBootstrapError] = useState<string | null>(null)
  const [bootstrapSuccess, setBootstrapSuccess] = useState(false)
  const [vellumBrokenInstall, setVellumBrokenInstall] = useState<string[] | null>(null)
  const [vellumCleaning, setVellumCleaning] = useState(false)
  const [vellumUninstalling, setVellumUninstalling] = useState(false)
  const [vellumUninstallOutput, setVellumUninstallOutput] = useState('')
  const [vellumUninstallError, setVellumUninstallError] = useState<string | null>(null)
  const [vellumUninstallSuccess, setVellumUninstallSuccess] = useState(false)

  const setVellumInstalledRef = useRef(setVellumInstalled)
  setVellumInstalledRef.current = setVellumInstalled

  const setInstalledPackagesRef = useRef(setInstalledPackages)
  setInstalledPackagesRef.current = setInstalledPackages

  const setShowSettingsDialogRef = useRef(setShowSettingsDialog)
  setShowSettingsDialogRef.current = setShowSettingsDialog

  const rescanAllPackagesRef = useRef(rescanAllPackages)
  rescanAllPackagesRef.current = rescanAllPackages

  useEffect(() => {
    if (typeof window.runtime === 'undefined') return

    const unsubscribeBootstrapPrompt = window.runtime.EventsOn('vellum:bootstrap-prompt', () => {
      debugLog('Received vellum:bootstrap-prompt')
      setVellumInstalledRef.current(false)
    })

    const unsubscribeBootstrapStart = window.runtime.EventsOn('vellum:bootstrap-start', () => {
      debugLog('Received vellum:bootstrap-start')
      setBootstrapping(true)
      setBootstrapOutput('')
      setBootstrapError(null)
      setBootstrapSuccess(false)
    })

    const unsubscribeBootstrapOutput = window.runtime.EventsOn('vellum:bootstrap-output', (...args: unknown[]) => {
      const line = args[0] as string
      setBootstrapOutput((prev) => prev + line)
    })

    const unsubscribeBootstrapComplete = window.runtime.EventsOn('vellum:bootstrap-complete', () => {
      debugLog('Received vellum:bootstrap-complete')
      setBootstrapping(false)
      setBootstrapSuccess(true)
      setVellumInstalledRef.current(true)
      rescanAllPackagesRef.current()
    })

    const unsubscribeBootstrapError = window.runtime.EventsOn('vellum:bootstrap-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:bootstrap-error:', errMsg)
      setBootstrapping(false)
      setBootstrapError(errMsg)
    })

    const unsubscribeVellumReady = window.runtime.EventsOn('vellum:ready', () => {
      debugLog('Received vellum:ready')
      setVellumInstalledRef.current(true)
    })

    const unsubscribeVellumUninstallStart = window.runtime.EventsOn('vellum:uninstall-start', () => {
      debugLog('Received vellum:uninstall-start')
      setVellumUninstalling(true)
      setVellumUninstallOutput('')
      setVellumUninstallError(null)
      setVellumUninstallSuccess(false)
    })

    const unsubscribeVellumUninstallOutput = window.runtime.EventsOn('vellum:uninstall-output', (...args: unknown[]) => {
      const line = args[0] as string
      setVellumUninstallOutput((prev) => prev + line)
    })

    const unsubscribeVellumUninstallComplete = window.runtime.EventsOn('vellum:uninstall-complete', () => {
      debugLog('Received vellum:uninstall-complete')
      setVellumUninstalling(false)
      setVellumInstalledRef.current(false)
      setInstalledPackagesRef.current(new Map())
      setShowSettingsDialogRef.current(false)
      setVellumUninstallSuccess(true)
    })

    const unsubscribeVellumUninstallError = window.runtime.EventsOn('vellum:uninstall-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:uninstall-error:', errMsg)
      setVellumUninstalling(false)
      setVellumUninstallError(errMsg)
    })

    const unsubscribeBrokenInstall = window.runtime.EventsOn('vellum:broken-install', (...args: unknown[]) => {
      const missing = args[0] as string[]
      debugLog('Received vellum:broken-install, missing:', missing)
      setVellumBrokenInstall(missing)
    })

    const unsubscribeCleanupStart = window.runtime.EventsOn('vellum:cleanup-start', () => {
      debugLog('Received vellum:cleanup-start')
      setVellumCleaning(true)
    })

    const unsubscribeCleanupComplete = window.runtime.EventsOn('vellum:cleanup-complete', () => {
      debugLog('Received vellum:cleanup-complete')
      setVellumCleaning(false)
      setVellumBrokenInstall(null)
    })

    const unsubscribeCleanupError = window.runtime.EventsOn('vellum:cleanup-error', (...args: unknown[]) => {
      const errMsg = args[0] as string
      debugLog('Received vellum:cleanup-error:', errMsg)
      setVellumCleaning(false)
    })

    return () => {
      unsubscribeBootstrapPrompt()
      unsubscribeBootstrapStart()
      unsubscribeBootstrapOutput()
      unsubscribeBootstrapComplete()
      unsubscribeBootstrapError()
      unsubscribeVellumReady()
      unsubscribeVellumUninstallStart()
      unsubscribeVellumUninstallOutput()
      unsubscribeVellumUninstallComplete()
      unsubscribeVellumUninstallError()
      unsubscribeBrokenInstall()
      unsubscribeCleanupStart()
      unsubscribeCleanupComplete()
      unsubscribeCleanupError()
    }
  }, [])

  return {
    bootstrapping,
    setBootstrapping,
    bootstrapOutput,
    setBootstrapOutput,
    bootstrapError,
    setBootstrapError,
    bootstrapSuccess,
    setBootstrapSuccess,
    vellumBrokenInstall,
    setVellumBrokenInstall,
    vellumCleaning,
    setVellumCleaning,
    vellumUninstalling,
    setVellumUninstalling,
    vellumUninstallOutput,
    setVellumUninstallOutput,
    vellumUninstallError,
    setVellumUninstallError,
    vellumUninstallSuccess,
    setVellumUninstallSuccess,
  }
}
