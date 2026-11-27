import type { FileKind } from '../types/api'

export const DSM_FILE_DRAG_MIME = 'application/x-dsm-file-entry'
const DSM_FILES_CHANGED_EVENT = 'dsm:files-changed'

export interface DsmFileDragPayload {
  readonly rootId: string
  readonly relPath: string
  readonly name: string
  readonly isDir: boolean
  readonly kind: FileKind
}

export interface DsmFilesChangedDetail {
  readonly roots: readonly string[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function parseFileKind(raw: unknown): FileKind | null {
  switch (raw) {
    case 'dir':
    case 'video':
    case 'audio':
    case 'image':
    case 'code':
    case 'archive':
    case 'file':
      return raw
    default:
      return null
  }
}

export function setDsmFileDragData(
  dt: DataTransfer | null,
  payload: DsmFileDragPayload,
): void {
  if (!dt) return
  dt.effectAllowed = 'copyMove'
  dt.setData(DSM_FILE_DRAG_MIME, JSON.stringify(payload))
  dt.setData('text/plain', payload.name)
}

export function hasDsmFileDragData(dt: DataTransfer | null): boolean {
  if (!dt) return false
  return Array.from(dt.types).includes(DSM_FILE_DRAG_MIME)
}

export function readDsmFileDragData(dt: DataTransfer | null): DsmFileDragPayload | null {
  if (!dt) return null
  const raw = dt.getData(DSM_FILE_DRAG_MIME)
  if (!raw) return null
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!isRecord(parsed)) return null
    const kind = parseFileKind(parsed.kind)
    if (
      typeof parsed.rootId !== 'string' ||
      typeof parsed.relPath !== 'string' ||
      typeof parsed.name !== 'string' ||
      typeof parsed.isDir !== 'boolean' ||
      kind === null
    ) {
      return null
    }
    return {
      rootId: parsed.rootId,
      relPath: parsed.relPath,
      name: parsed.name,
      isDir: parsed.isDir,
      kind,
    }
  } catch {
    return null
  }
}

export function emitDsmFilesChanged(roots: readonly string[]): void {
  window.dispatchEvent(
    new CustomEvent<DsmFilesChangedDetail>(DSM_FILES_CHANGED_EVENT, {
      detail: { roots },
    }),
  )
}

export function onDsmFilesChanged(
  handler: (detail: DsmFilesChangedDetail) => void,
): () => void {
  const listener = (ev: Event): void => {
    if (!(ev instanceof CustomEvent) || !isRecord(ev.detail)) return
    const rawRoots = ev.detail.roots
    if (!Array.isArray(rawRoots)) return
    const roots = rawRoots.filter((item): item is string => typeof item === 'string')
    handler({ roots })
  }
  window.addEventListener(DSM_FILES_CHANGED_EVENT, listener)
  return () => {
    window.removeEventListener(DSM_FILES_CHANGED_EVENT, listener)
  }
}
