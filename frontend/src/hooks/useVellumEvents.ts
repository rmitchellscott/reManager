import { useState, useEffect, useRef } from 'react'
import { debugLog } from '@/lib/utils'
import { onEvent } from '@/lib/events'

interface UseVellumEventsParams {
  setVellumInstalled: (v: boolean | null) => void
}

export function useVellumEvents({
  setVellumInstalled,
}: UseVellumEventsParams) {
  const [vellumBrokenInstall, setVellumBrokenInstall] = useState<string[] | null>(null)

  const setVellumInstalledRef = useRef(setVellumInstalled)
  setVellumInstalledRef.current = setVellumInstalled

  useEffect(() => {

    const unsubscribeVellumMissing = onEvent('vellum:missing', () => {
      debugLog('Received vellum:missing')
      setVellumInstalledRef.current(false)
    })

    const unsubscribeVellumReady = onEvent('vellum:ready', () => {
      debugLog('Received vellum:ready')
      setVellumInstalledRef.current(true)
    })

    const unsubscribeBrokenInstall = onEvent('vellum:broken-install', missing => {
      debugLog('Received vellum:broken-install, missing:', missing)
      setVellumBrokenInstall(missing)
    })

    return () => {
      unsubscribeVellumMissing()
      unsubscribeVellumReady()
      unsubscribeBrokenInstall()
    }
  }, [])

  return {
    vellumBrokenInstall,
    setVellumBrokenInstall,
  }
}
