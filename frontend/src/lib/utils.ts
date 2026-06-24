import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function debugLog(...args: unknown[]) {
  if (import.meta.env.DEV) {
    console.log(...args)
  }
}

export function isRealVersion(v: string | undefined | null): boolean {
  return !!v && /^\d+(\.\d+)*$/.test(v)
}

export function compareVersions(a: string, b: string): number {
  const pa = a.split('.')
  const pb = b.split('.')
  const len = Math.max(pa.length, pb.length)
  for (let i = 0; i < len; i++) {
    const na = parseInt(pa[i] ?? '0', 10) || 0
    const nb = parseInt(pb[i] ?? '0', 10) || 0
    if (na < nb) return -1
    if (na > nb) return 1
  }
  return 0
}
