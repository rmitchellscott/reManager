import { Archive, File, FileText, Folder, Image } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import type { FileInfo } from '@/lib/types'

const BY_EXTENSION: Record<string, { Glyph: Icon; color: string }> = {}

const register = (extensions: string[], Glyph: Icon, color: string) => {
  for (const extension of extensions) {
    BY_EXTENSION[extension] = { Glyph, color }
  }
}

register(['txt', 'md', 'json', 'xml', 'sh', 'conf', 'cfg'], FileText, 'text-[var(--file-doc)]')
register(['png', 'jpg', 'jpeg', 'gif', 'svg', 'bmp'], Image, 'text-[var(--file-image)]')
register(['tar', 'gz', 'zip', 'bz2', 'xz', 'apk'], Archive, 'text-[var(--file-archive)]')

function getFileIcon(file: FileInfo) {
  if (file.isDir) return { Glyph: Folder, color: 'text-[var(--file-folder)]' }
  const extension = file.name.split('.').pop()?.toLowerCase()
  return (extension && BY_EXTENSION[extension]) || { Glyph: File, color: 'text-[var(--file-default)]' }
}

export function FileIcon({ file, className = 'h-4 w-4' }: { file: FileInfo; className?: string }) {
  const { Glyph, color } = getFileIcon(file)
  return <Glyph className={`${className} ${color}`} />
}
