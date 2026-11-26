import {
  extractErrorMessage,
  parseAuthStatus,
  parseCreatedShare,
  parseFileList,
  parseHostBrowse,
  parseMediaInfo,
  parseMountRoots,
  parseMountsList,
  parsePublicShare,
  parseSharesList,
  parseSystemSummary,
  parseTextDocument,
  parseUploadInit,
  parseUploadOffset,
  parseWargameInstance,
  parseWargameList,
  parseWargameLogs,
  type AuthStatus,
  type FileListResult,
  type HostBrowseResult,
  type MediaInfo,
  type MountInfo,
  type MountRoot,
  type PublicShareState,
  type ShareItem,
  type SharesListResult,
  type SystemSummary,
  type TextDocument,
  type WargameConnKind,
  type WargameInstance,
  type WargameListResult,
  type WargameLogsResult,
  type WargameTemplateKind,
} from '../types/api'

async function requestJSON(url: string, init?: RequestInit): Promise<unknown> {
  const res = await fetch(url, {
    credentials: 'same-origin',
    ...init,
  })
  let body: unknown = null
  const contentType = res.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    body = await res.json()
  }
  if (!res.ok) {
    throw new Error(extractErrorMessage(body, `요청 처리 중 오류가 발생했습니다 (${res.status})`))
  }
  return body
}

export const api = {
  async fetchMe(): Promise<AuthStatus> {
    const raw = await requestJSON('/api/auth/me')
    return parseAuthStatus(raw)
  },

  async login(params: { readonly username: string; readonly password: string }): Promise<AuthStatus> {
    const raw = await requestJSON('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    return parseAuthStatus(raw)
  },

  async logout(): Promise<void> {
    await requestJSON('/api/auth/logout', { method: 'POST' })
  },

  async changePassword(params: { readonly currentPassword: string; readonly newPassword: string }): Promise<void> {
    await requestJSON('/api/auth/password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async fetchRoots(): Promise<readonly MountRoot[]> {
    const raw = await requestJSON('/api/files/roots')
    return parseMountRoots(raw)
  },

  async listFiles(params: { readonly rootId: string; readonly relPath: string }): Promise<FileListResult> {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    const raw = await requestJSON(`/api/files/list?${qs.toString()}`)
    return parseFileList(raw)
  },

  async mkdir(params: { readonly rootId: string; readonly relPath: string }): Promise<void> {
    await requestJSON('/api/files/mkdir', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async renameEntry(params: {
    readonly rootId: string
    readonly fromRel: string
    readonly toRel: string
  }): Promise<void> {
    await requestJSON('/api/files/rename', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async deleteEntry(params: { readonly rootId: string; readonly relPath: string }): Promise<void> {
    await requestJSON('/api/files/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async readTextFile(params: { readonly rootId: string; readonly relPath: string }): Promise<TextDocument> {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    const raw = await requestJSON(`/api/files/text?${qs.toString()}`)
    return parseTextDocument(raw)
  },

  async writeTextFile(params: {
    readonly rootId: string
    readonly relPath: string
    readonly content: string
  }): Promise<void> {
    await requestJSON('/api/files/text', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  downloadUrl(params: { readonly rootId: string; readonly relPath: string }): string {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    return `/api/files/download?${qs.toString()}`
  },

  rawFileUrl(params: { readonly rootId: string; readonly relPath: string }): string {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    return `/api/files/raw?${qs.toString()}`
  },

  async uploadFileChunked(params: {
    readonly rootId: string
    readonly dirPath: string
    readonly file: File
    readonly onProgress?: (uploadedBytes: number, totalBytes: number) => void
  }): Promise<void> {
    const initRaw = await requestJSON('/api/files/upload/init', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        rootId: params.rootId,
        dirPath: params.dirPath,
        fileName: params.file.name,
        totalSize: params.file.size,
      }),
    })
    const { uploadId, chunkSize } = parseUploadInit(initRaw)

    let offset = 0
    while (offset < params.file.size) {
      const end = Math.min(offset + chunkSize, params.file.size)
      const slice = params.file.slice(offset, end)
      const chunkRaw = await requestJSON(`/api/files/upload/${encodeURIComponent(uploadId)}`, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/octet-stream',
          'X-Upload-Offset': String(offset),
        },
        body: slice,
      })
      offset = parseUploadOffset(chunkRaw)
      params.onProgress?.(offset, params.file.size)
    }

    await requestJSON(`/api/files/upload/${encodeURIComponent(uploadId)}/complete`, {
      method: 'POST',
    })
    params.onProgress?.(params.file.size, params.file.size)
  },

  async fetchMediaInfo(params: { readonly rootId: string; readonly relPath: string }): Promise<MediaInfo> {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    const raw = await requestJSON(`/api/media/info?${qs.toString()}`)
    return parseMediaInfo(raw)
  },

  mediaStreamUrl(params: { readonly rootId: string; readonly relPath: string }): string {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    return `/api/media/stream?${qs.toString()}`
  },

  mediaSubtitleUrl(params: { readonly rootId: string; readonly relPath: string }): string {
    const qs = new URLSearchParams({ root: params.rootId, path: params.relPath })
    return `/api/media/subtitle?${qs.toString()}`
  },

  async saveMediaProgress(params: {
    readonly rootId: string
    readonly relPath: string
    readonly positionSec: number
    readonly durationSec: number
  }): Promise<void> {
    await requestJSON('/api/media/progress', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async listShares(): Promise<SharesListResult> {
    const raw = await requestJSON('/api/shares')
    return parseSharesList(raw)
  },

  async createShare(params: {
    readonly rootId: string
    readonly relPath: string
    readonly password: string
    readonly expiresHours: number
    readonly allowDownload: boolean
  }): Promise<{ readonly share: ShareItem; readonly publicBaseUrl: string }> {
    const raw = await requestJSON('/api/shares', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    return parseCreatedShare(raw)
  },

  async deleteShare(token: string): Promise<void> {
    await requestJSON(`/api/shares/${encodeURIComponent(token)}`, {
      method: 'DELETE',
    })
  },

  async fetchPublicShare(token: string): Promise<PublicShareState> {
    const raw = await requestJSON(`/api/public/shares/${encodeURIComponent(token)}`)
    return parsePublicShare(raw)
  },

  async unlockPublicShare(params: { readonly token: string; readonly password: string }): Promise<void> {
    await requestJSON(`/api/public/shares/${encodeURIComponent(params.token)}/unlock`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: params.password }),
    })
  },

  publicShareDownloadUrl(token: string): string {
    return `/api/public/shares/${encodeURIComponent(token)}/download`
  },

  publicShareStreamUrl(token: string): string {
    return `/api/public/shares/${encodeURIComponent(token)}/stream`
  },

  publicShareSubtitleUrl(params: { readonly token: string; readonly relPath: string }): string {
    const qs = new URLSearchParams({ path: params.relPath })
    return `/api/public/shares/${encodeURIComponent(params.token)}/subtitle?${qs.toString()}`
  },

  async fetchSystemSummary(): Promise<SystemSummary> {
    const raw = await requestJSON('/api/system/summary')
    return parseSystemSummary(raw)
  },

  async listMounts(): Promise<readonly MountInfo[]> {
    const raw = await requestJSON('/api/mounts')
    return parseMountsList(raw)
  },

  async createMount(params: {
    readonly name: string
    readonly hostPath: string
    readonly readOnly: boolean
    readonly privateOnly: boolean
  }): Promise<void> {
    await requestJSON('/api/mounts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
  },

  async updateMount(params: {
    readonly id: string
    readonly name: string
    readonly readOnly: boolean
    readonly privateOnly: boolean
  }): Promise<void> {
    await requestJSON(`/api/mounts/${encodeURIComponent(params.id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: params.name,
        readOnly: params.readOnly,
        privateOnly: params.privateOnly,
      }),
    })
  },

  async deleteMount(id: string): Promise<void> {
    await requestJSON(`/api/mounts/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  },

  async browseHostDir(params: { readonly path?: string; readonly showHidden?: boolean }): Promise<HostBrowseResult> {
    const qs = new URLSearchParams()
    if (params.path) qs.set('path', params.path)
    if (params.showHidden) qs.set('hidden', '1')
    const raw = await requestJSON(`/api/mounts/browse-host?${qs.toString()}`)
    return parseHostBrowse(raw)
  },

  async listWargames(): Promise<WargameListResult> {
    const raw = await requestJSON('/api/wargames')
    return parseWargameList(raw)
  },

  async prepareWargameUpload(params: {
    readonly file: File
    readonly name?: string
  }): Promise<WargameInstance> {
    const form = new FormData()
    form.append('file', params.file)
    if (params.name && params.name.trim() !== '') {
      form.append('name', params.name.trim())
    }
    const raw = await requestJSON('/api/wargames/prepare/upload', {
      method: 'POST',
      body: form,
    })
    return parseWargameInstance(raw)
  },

  async prepareWargameMount(params: {
    readonly rootId: string
    readonly relPath: string
    readonly name?: string
  }): Promise<WargameInstance> {
    const raw = await requestJSON('/api/wargames/prepare/mount', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        rootId: params.rootId,
        relPath: params.relPath,
        name: params.name ?? '',
      }),
    })
    return parseWargameInstance(raw)
  },

  async startWargame(params: {
    readonly id: string
    readonly name: string
    readonly templateKind: WargameTemplateKind
    readonly containerPort: number
    readonly connKind: WargameConnKind
    readonly relaxSecurity: boolean
  }): Promise<WargameInstance> {
    const raw = await requestJSON(`/api/wargames/${encodeURIComponent(params.id)}/start`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: params.name,
        templateKind: params.templateKind,
        containerPort: params.containerPort,
        connKind: params.connKind,
        relaxSecurity: params.relaxSecurity,
      }),
    })
    return parseWargameInstance(raw)
  },

  async resetWargame(id: string): Promise<WargameInstance> {
    const raw = await requestJSON(`/api/wargames/${encodeURIComponent(id)}/reset`, {
      method: 'POST',
    })
    return parseWargameInstance(raw)
  },

  async stopWargame(id: string): Promise<WargameInstance> {
    const raw = await requestJSON(`/api/wargames/${encodeURIComponent(id)}/stop`, {
      method: 'POST',
    })
    return parseWargameInstance(raw)
  },

  async deleteWargame(id: string): Promise<void> {
    await requestJSON(`/api/wargames/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  },

  async extendWargame(id: string): Promise<WargameInstance> {
    const raw = await requestJSON(`/api/wargames/${encodeURIComponent(id)}/extend`, {
      method: 'POST',
    })
    return parseWargameInstance(raw)
  },

  async fetchWargameLogs(id: string): Promise<WargameLogsResult> {
    const raw = await requestJSON(`/api/wargames/${encodeURIComponent(id)}/logs`)
    return parseWargameLogs(raw)
  },

  wargameSourceUrl(id: string): string {
    return `/api/wargames/${encodeURIComponent(id)}/source`
  },
}
