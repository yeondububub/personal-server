export type AccessTier = 'public' | 'private'

export type FileKind = 'dir' | 'video' | 'audio' | 'image' | 'code' | 'archive' | 'file'

export interface UserInfo {
  readonly id: number
  readonly username: string
}

export type AuthStatus =
  | { readonly kind: 'loading' }
  | { readonly kind: 'unauthenticated'; readonly accessTier: AccessTier }
  | { readonly kind: 'authenticated'; readonly accessTier: AccessTier; readonly user: UserInfo }

export interface MountRoot {
  readonly id: string
  readonly name: string
  readonly readOnly: boolean
  readonly privateOnly: boolean
}

export interface MountInfo {
  readonly id: string
  readonly name: string
  readonly hostPath: string
  readonly readOnly: boolean
  readonly privateOnly: boolean
  readonly exists: boolean
  readonly createdAt: number
}

export interface HostQuickPath {
  readonly label: string
  readonly path: string
}

export interface HostDirEntry {
  readonly name: string
  readonly path: string
}

export interface HostBrowseResult {
  readonly currentPath: string
  readonly parentPath: string
  readonly quickPaths: readonly HostQuickPath[]
  readonly directories: readonly HostDirEntry[]
}

export interface FileEntry {
  readonly name: string
  readonly relPath: string
  readonly isDir: boolean
  readonly size: number
  readonly modTime: number
  readonly kind: FileKind
}

export interface FileListResult {
  readonly rootId: string
  readonly relPath: string
  readonly readOnly: boolean
  readonly entries: readonly FileEntry[]
}

export interface TextDocument {
  readonly rootId: string
  readonly relPath: string
  readonly name: string
  readonly content: string
  readonly size: number
  readonly modTime: number
  readonly readOnly: boolean
}

export interface SubtitleTrack {
  readonly name: string
  readonly format: string
  readonly relPath: string
}

export type MediaPlaybackMode = 'direct' | 'hls'

export type HlsHwAccel = 'vaapi' | 'software' | 'copy'

export interface MediaInfo {
  readonly name: string
  readonly rootId: string
  readonly relPath: string
  readonly size: number
  readonly positionSec: number
  readonly durationSec: number
  readonly subtitles: readonly SubtitleTrack[]
  readonly playbackMode: MediaPlaybackMode
  readonly videoCodec: string
  readonly audioCodec: string
  readonly ffmpegAvailable: boolean
}

export interface HlsStartResult {
  readonly sessionId: string
  readonly playlistUrl: string
  readonly hwAccel: HlsHwAccel
  readonly startSec: number
}

export interface ShareItem {
  readonly token: string
  readonly rootId: string
  readonly relPath: string
  readonly name: string
  readonly isDir: boolean
  readonly hasPassword: boolean
  readonly allowDownload: boolean
  readonly expiresAt: number
  readonly createdAt: number
}

export interface SharesListResult {
  readonly shares: readonly ShareItem[]
  readonly publicBaseUrl: string
}

export type PublicShareState =
  | {
      readonly kind: 'locked'
      readonly token: string
      readonly expiresAt: number
    }
  | {
      readonly kind: 'unlocked'
      readonly token: string
      readonly requiresPassword: boolean
      readonly name: string
      readonly isDir: boolean
      readonly size: number
      readonly fileKind: FileKind
      readonly allowDownload: boolean
      readonly expiresAt: number
      readonly subtitles: readonly SubtitleTrack[]
    }

export interface DiskSummary {
  readonly id: string
  readonly name: string
  readonly totalBytes: number
  readonly freeBytes: number
}

export interface SystemSummary {
  readonly accessTier: AccessTier
  readonly goOS: string
  readonly goArch: string
  readonly cpuCores: number
  readonly uptimeSeconds: number
  readonly appMemoryBytes: number
  readonly disks: readonly DiskSummary[]
}

export interface MonitorSnapshot {
  readonly timestamp: number
  readonly hostname: string
  readonly os: string
  readonly platform: string
  readonly cpuTotalPercent: number
  readonly cpuPerCore: readonly number[]
  readonly memTotalBytes: number
  readonly memUsedBytes: number
  readonly memUsedPercent: number
  readonly swapTotalBytes: number
  readonly swapUsedBytes: number
  readonly netRxBytesPerSec: number
  readonly netTxBytesPerSec: number
  readonly uptimeSeconds: number
  readonly disks: readonly DiskSummary[]
}

export type TerminalTargetKind = 'host' | 'docker'

export interface TerminalTarget {
  readonly kind: TerminalTargetKind
  readonly id: string
  readonly label: string
}

export interface UploadInitResult {
  readonly uploadId: string
  readonly chunkSize: number
}

export type WargameMode = 'compose' | 'dockerfile' | 'template'

export type WargameTemplateKind = '' | 'pwn-socat' | 'python-flask' | 'php-apache' | 'node-express'

export type WargameStatus = 'detected' | 'building' | 'running' | 'stopped' | 'failed'

export type WargameConnKind = 'nc' | 'http'

export interface WargamePortBinding {
  readonly service: string
  readonly containerPort: number
  readonly hostPort: number
  readonly protocol: string
  readonly connKind: WargameConnKind
}

export interface WargameInstance {
  readonly id: string
  readonly name: string
  readonly mode: WargameMode
  readonly templateKind: WargameTemplateKind
  readonly entrypointFile: string
  readonly composeFile: string
  readonly relaxSecurity: boolean
  readonly status: WargameStatus
  readonly bindIp: string
  readonly ports: readonly WargamePortBinding[]
  readonly warnings: readonly string[]
  readonly expiresAt: number
  readonly createdAt: number
  readonly lastError: string
}

export interface WargameListResult {
  readonly dockerAvailable: boolean
  readonly dockerError: string
  readonly bindIp: string
  readonly instances: readonly WargameInstance[]
}

export interface WargameLogsResult {
  readonly buildLog: string
  readonly containerLog: string
  readonly status: WargameStatus
}

export interface DockerDiskUsageItem {
  readonly type: string
  readonly totalCount: string
  readonly active: string
  readonly size: string
  readonly reclaimable: string
}

export interface DockerOverview {
  readonly available: boolean
  readonly error: string
  readonly serverVersion: string
  readonly containersRunning: number
  readonly containersStopped: number
  readonly imagesCount: number
  readonly volumesCount: number
  readonly networksCount: number
  readonly diskUsage: readonly DockerDiskUsageItem[]
}

export interface DockerContainer {
  readonly id: string
  readonly name: string
  readonly image: string
  readonly state: string
  readonly status: string
  readonly ports: string
  readonly createdAt: string
  readonly isWargame: boolean
  readonly cpuPercent: string
  readonly memUsage: string
  readonly memPercent: string
}

export interface DockerContainerMount {
  readonly type: string
  readonly source: string
  readonly destination: string
  readonly rw: boolean
}

export interface DockerContainerInspect {
  readonly id: string
  readonly name: string
  readonly image: string
  readonly state: string
  readonly restartPolicy: string
  readonly env: readonly string[]
  readonly mounts: readonly DockerContainerMount[]
  readonly networks: readonly string[]
  readonly ports: readonly string[]
}

export interface DockerImage {
  readonly id: string
  readonly repository: string
  readonly tag: string
  readonly size: string
  readonly createdAt: string
  readonly inUse: boolean
}

export interface DockerStack {
  readonly name: string
  readonly status: string
  readonly servicesCount: number
  readonly runningCount: number
  readonly updatedAt: string
}

export interface DockerStackDetail {
  readonly name: string
  readonly composeYaml: string
  readonly status: string
}

export interface DockerVolume {
  readonly name: string
  readonly driver: string
  readonly mountpoint: string
  readonly createdAt: string
}

export interface DockerNetwork {
  readonly id: string
  readonly name: string
  readonly driver: string
  readonly scope: string
}

// Runtime boundary validators

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readString(obj: Record<string, unknown>, key: string, fallback = ''): string {
  const val = obj[key]
  return typeof val === 'string' ? val : fallback
}

function readNumber(obj: Record<string, unknown>, key: string, fallback = 0): number {
  const val = obj[key]
  return typeof val === 'number' && Number.isFinite(val) ? val : fallback
}

function readBoolean(obj: Record<string, unknown>, key: string, fallback = false): boolean {
  const val = obj[key]
  return typeof val === 'boolean' ? val : fallback
}

function parseAccessTier(raw: unknown): AccessTier {
  return raw === 'private' ? 'private' : 'public'
}

function parseFileKind(raw: unknown): FileKind {
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
      return 'file'
  }
}

export function extractErrorMessage(payload: unknown, fallback: string): string {
  if (isRecord(payload) && typeof payload.error === 'string' && payload.error.trim() !== '') {
    return payload.error
  }
  return fallback
}

export function parseAuthStatus(payload: unknown): AuthStatus {
  if (!isRecord(payload)) {
    throw new Error('유효하지 않은 인증 응답입니다.')
  }
  const accessTier = parseAccessTier(payload.accessTier)
  const authenticated = readBoolean(payload, 'authenticated')
  if (authenticated && isRecord(payload.user)) {
    return {
      kind: 'authenticated',
      accessTier,
      user: {
        id: readNumber(payload.user, 'id'),
        username: readString(payload.user, 'username'),
      },
    }
  }
  return {
    kind: 'unauthenticated',
    accessTier,
  }
}

export function parseMountRoots(payload: unknown): readonly MountRoot[] {
  if (!isRecord(payload) || !Array.isArray(payload.roots)) {
    return []
  }
  const out: MountRoot[] = []
  for (const item of payload.roots) {
    if (isRecord(item)) {
      const id = readString(item, 'id')
      const name = readString(item, 'name', id)
      if (id !== '') {
        out.push({
          id,
          name,
          readOnly: readBoolean(item, 'readOnly'),
          privateOnly: readBoolean(item, 'privateOnly'),
        })
      }
    }
  }
  return out
}

export function parseMountsList(payload: unknown): readonly MountInfo[] {
  if (!isRecord(payload) || !Array.isArray(payload.mounts)) {
    return []
  }
  const out: MountInfo[] = []
  for (const item of payload.mounts) {
    if (!isRecord(item)) continue
    out.push({
      id: readString(item, 'id'),
      name: readString(item, 'name'),
      hostPath: readString(item, 'hostPath'),
      readOnly: readBoolean(item, 'readOnly'),
      privateOnly: readBoolean(item, 'privateOnly'),
      exists: readBoolean(item, 'exists'),
      createdAt: readNumber(item, 'createdAt'),
    })
  }
  return out
}

export function parseHostBrowse(payload: unknown): HostBrowseResult {
  if (!isRecord(payload)) {
    throw new Error('호스트 디렉토리 목록 응답이 올바르지 않습니다.')
  }
  const quickRaw = Array.isArray(payload.quickPaths) ? payload.quickPaths : []
  const quickPaths: HostQuickPath[] = []
  for (const q of quickRaw) {
    if (!isRecord(q)) continue
    quickPaths.push({
      label: readString(q, 'label'),
      path: readString(q, 'path'),
    })
  }
  const dirsRaw = Array.isArray(payload.directories) ? payload.directories : []
  const directories: HostDirEntry[] = []
  for (const d of dirsRaw) {
    if (!isRecord(d)) continue
    directories.push({
      name: readString(d, 'name'),
      path: readString(d, 'path'),
    })
  }
  return {
    currentPath: readString(payload, 'currentPath'),
    parentPath: readString(payload, 'parentPath'),
    quickPaths,
    directories,
  }
}

export function parseFileList(payload: unknown): FileListResult {
  if (!isRecord(payload)) {
    throw new Error('유효하지 않은 파일 목록 응답입니다.')
  }
  const entriesRaw = Array.isArray(payload.entries) ? payload.entries : []
  const entries: FileEntry[] = []
  for (const item of entriesRaw) {
    if (!isRecord(item)) continue
    entries.push({
      name: readString(item, 'name'),
      relPath: readString(item, 'relPath'),
      isDir: readBoolean(item, 'isDir'),
      size: readNumber(item, 'size'),
      modTime: readNumber(item, 'modTime'),
      kind: parseFileKind(item.kind),
    })
  }
  return {
    rootId: readString(payload, 'rootId'),
    relPath: readString(payload, 'relPath'),
    readOnly: readBoolean(payload, 'readOnly'),
    entries,
  }
}

export function parseTextDocument(payload: unknown): TextDocument {
  if (!isRecord(payload)) {
    throw new Error('유효하지 않은 텍스트 파일 응답입니다.')
  }
  return {
    rootId: readString(payload, 'rootId'),
    relPath: readString(payload, 'relPath'),
    name: readString(payload, 'name'),
    content: readString(payload, 'content'),
    size: readNumber(payload, 'size'),
    modTime: readNumber(payload, 'modTime'),
    readOnly: readBoolean(payload, 'readOnly'),
  }
}

function parseSubtitles(raw: unknown): readonly SubtitleTrack[] {
  if (!Array.isArray(raw)) return []
  const out: SubtitleTrack[] = []
  for (const s of raw) {
    if (!isRecord(s)) continue
    out.push({
      name: readString(s, 'name'),
      format: readString(s, 'format'),
      relPath: readString(s, 'relPath'),
    })
  }
  return out
}

function parseMediaPlaybackMode(raw: unknown): MediaPlaybackMode {
  return raw === 'hls' ? 'hls' : 'direct'
}

function parseHlsHwAccel(raw: unknown): HlsHwAccel {
  if (raw === 'vaapi' || raw === 'copy') return raw
  return 'software'
}

export function parseMediaInfo(payload: unknown): MediaInfo {
  if (!isRecord(payload)) {
    throw new Error('유효하지 않은 미디어 정보 응답입니다.')
  }
  return {
    name: readString(payload, 'name'),
    rootId: readString(payload, 'rootId'),
    relPath: readString(payload, 'relPath'),
    size: readNumber(payload, 'size'),
    positionSec: readNumber(payload, 'positionSec'),
    durationSec: readNumber(payload, 'durationSec'),
    subtitles: parseSubtitles(payload.subtitles),
    playbackMode: parseMediaPlaybackMode(payload.playbackMode),
    videoCodec: readString(payload, 'videoCodec'),
    audioCodec: readString(payload, 'audioCodec'),
    ffmpegAvailable: readBoolean(payload, 'ffmpegAvailable'),
  }
}

export function parseHlsStartResult(payload: unknown): HlsStartResult {
  if (!isRecord(payload)) {
    throw new Error('HLS 트랜스코딩 세션 응답이 올바르지 않습니다.')
  }
  const sessionId = readString(payload, 'sessionId')
  const playlistUrl = readString(payload, 'playlistUrl')
  if (sessionId === '' || playlistUrl === '') {
    throw new Error('HLS 세션 정보가 누락되었습니다.')
  }
  return {
    sessionId,
    playlistUrl,
    hwAccel: parseHlsHwAccel(payload.hwAccel),
    startSec: readNumber(payload, 'startSec', 0),
  }
}

function parseShareItem(raw: Record<string, unknown>): ShareItem {
  return {
    token: readString(raw, 'token'),
    rootId: readString(raw, 'rootId'),
    relPath: readString(raw, 'relPath'),
    name: readString(raw, 'name'),
    isDir: readBoolean(raw, 'isDir'),
    hasPassword: readBoolean(raw, 'hasPassword'),
    allowDownload: readBoolean(raw, 'allowDownload', true),
    expiresAt: readNumber(raw, 'expiresAt'),
    createdAt: readNumber(raw, 'createdAt'),
  }
}

export function parseSharesList(payload: unknown): SharesListResult {
  if (!isRecord(payload)) {
    return { shares: [], publicBaseUrl: '' }
  }
  const listRaw = Array.isArray(payload.shares) ? payload.shares : []
  const shares: ShareItem[] = []
  for (const item of listRaw) {
    if (isRecord(item)) {
      shares.push(parseShareItem(item))
    }
  }
  return {
    shares,
    publicBaseUrl: readString(payload, 'publicBaseUrl'),
  }
}

export function parseCreatedShare(payload: unknown): { readonly share: ShareItem; readonly publicBaseUrl: string } {
  if (!isRecord(payload) || !isRecord(payload.share)) {
    throw new Error('공유 링크 생성 응답이 올바르지 않습니다.')
  }
  return {
    share: parseShareItem(payload.share),
    publicBaseUrl: readString(payload, 'publicBaseUrl'),
  }
}

export function parsePublicShare(payload: unknown): PublicShareState {
  if (!isRecord(payload)) {
    throw new Error('공유 링크 정보를 읽을 수 없습니다.')
  }
  const locked = readBoolean(payload, 'locked')
  const token = readString(payload, 'token')
  const expiresAt = readNumber(payload, 'expiresAt')
  if (locked) {
    return { kind: 'locked', token, expiresAt }
  }
  return {
    kind: 'unlocked',
    token,
    requiresPassword: readBoolean(payload, 'requiresPassword'),
    name: readString(payload, 'name'),
    isDir: readBoolean(payload, 'isDir'),
    size: readNumber(payload, 'size'),
    fileKind: parseFileKind(payload.kind),
    allowDownload: readBoolean(payload, 'allowDownload', true),
    expiresAt,
    subtitles: parseSubtitles(payload.subtitles),
  }
}

function parseDiskSummaries(raw: unknown): readonly DiskSummary[] {
  if (!Array.isArray(raw)) return []
  const disks: DiskSummary[] = []
  for (const d of raw) {
    if (!isRecord(d)) continue
    disks.push({
      id: readString(d, 'id'),
      name: readString(d, 'name'),
      totalBytes: readNumber(d, 'totalBytes'),
      freeBytes: readNumber(d, 'freeBytes'),
    })
  }
  return disks
}

function parseNumberArray(raw: unknown): readonly number[] {
  if (!Array.isArray(raw)) return []
  const out: number[] = []
  for (const item of raw) {
    if (typeof item === 'number' && Number.isFinite(item)) {
      out.push(item)
    }
  }
  return out
}

export function parseSystemSummary(payload: unknown): SystemSummary {
  if (!isRecord(payload)) {
    throw new Error('시스템 요약 응답이 올바르지 않습니다.')
  }
  return {
    accessTier: parseAccessTier(payload.accessTier),
    goOS: readString(payload, 'goOS'),
    goArch: readString(payload, 'goArch'),
    cpuCores: readNumber(payload, 'cpuCores'),
    uptimeSeconds: readNumber(payload, 'uptimeSeconds'),
    appMemoryBytes: readNumber(payload, 'appMemoryBytes'),
    disks: parseDiskSummaries(payload.disks),
  }
}

export function parseMonitorSnapshot(payload: unknown): MonitorSnapshot {
  if (!isRecord(payload)) {
    throw new Error('리소스 모니터 스냅샷 응답이 올바르지 않습니다.')
  }
  return {
    timestamp: readNumber(payload, 'timestamp'),
    hostname: readString(payload, 'hostname'),
    os: readString(payload, 'os'),
    platform: readString(payload, 'platform'),
    cpuTotalPercent: readNumber(payload, 'cpuTotalPercent'),
    cpuPerCore: parseNumberArray(payload.cpuPerCore),
    memTotalBytes: readNumber(payload, 'memTotalBytes'),
    memUsedBytes: readNumber(payload, 'memUsedBytes'),
    memUsedPercent: readNumber(payload, 'memUsedPercent'),
    swapTotalBytes: readNumber(payload, 'swapTotalBytes'),
    swapUsedBytes: readNumber(payload, 'swapUsedBytes'),
    netRxBytesPerSec: readNumber(payload, 'netRxBytesPerSec'),
    netTxBytesPerSec: readNumber(payload, 'netTxBytesPerSec'),
    uptimeSeconds: readNumber(payload, 'uptimeSeconds'),
    disks: parseDiskSummaries(payload.disks),
  }
}

export function parseTerminalTargets(payload: unknown): readonly TerminalTarget[] {
  const listRaw: unknown[] = Array.isArray(payload)
    ? payload
    : isRecord(payload) && Array.isArray(payload.targets)
      ? payload.targets
      : []
  const out: TerminalTarget[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const kind: TerminalTargetKind = item.kind === 'docker' ? 'docker' : 'host'
    const rawId = readString(item, 'id')
    const id = rawId !== '' ? rawId : kind === 'host' ? 'host' : ''
    if (id === '') continue
    const label = readString(item, 'label', kind === 'host' ? '호스트 쉘' : id)
    out.push({ kind, id, label })
  }
  return out
}

export function parseUploadInit(payload: unknown): UploadInitResult {
  if (!isRecord(payload)) {
    throw new Error('업로드 세션 초기화 실패')
  }
  return {
    uploadId: readString(payload, 'uploadId'),
    chunkSize: readNumber(payload, 'chunkSize', 8 * 1024 * 1024),
  }
}

export function parseUploadOffset(payload: unknown): number {
  if (!isRecord(payload)) {
    throw new Error('청크 업로드 응답 오류')
  }
  return readNumber(payload, 'offset')
}

function parseWargameMode(raw: unknown): WargameMode {
  switch (raw) {
    case 'compose':
    case 'dockerfile':
    case 'template':
      return raw
    default:
      return 'template'
  }
}

function parseWargameTemplateKind(raw: unknown): WargameTemplateKind {
  switch (raw) {
    case '':
    case 'pwn-socat':
    case 'python-flask':
    case 'php-apache':
    case 'node-express':
      return raw
    default:
      return ''
  }
}

function parseWargameStatus(raw: unknown): WargameStatus {
  switch (raw) {
    case 'detected':
    case 'building':
    case 'running':
    case 'stopped':
    case 'failed':
      return raw
    default:
      return 'detected'
  }
}

function parseWargameConnKind(raw: unknown): WargameConnKind {
  return raw === 'http' ? 'http' : 'nc'
}

function parseWargamePorts(raw: unknown): readonly WargamePortBinding[] {
  if (!Array.isArray(raw)) return []
  const out: WargamePortBinding[] = []
  for (const item of raw) {
    if (!isRecord(item)) continue
    out.push({
      service: readString(item, 'service'),
      containerPort: readNumber(item, 'containerPort'),
      hostPort: readNumber(item, 'hostPort'),
      protocol: readString(item, 'protocol', 'tcp'),
      connKind: parseWargameConnKind(item.connKind),
    })
  }
  return out
}

function parseStringArray(raw: unknown): readonly string[] {
  if (!Array.isArray(raw)) return []
  const out: string[] = []
  for (const item of raw) {
    if (typeof item === 'string' && item !== '') {
      out.push(item)
    }
  }
  return out
}

function parseWargameInstanceRecord(raw: Record<string, unknown>): WargameInstance {
  return {
    id: readString(raw, 'id'),
    name: readString(raw, 'name'),
    mode: parseWargameMode(raw.mode),
    templateKind: parseWargameTemplateKind(raw.templateKind),
    entrypointFile: readString(raw, 'entrypointFile'),
    composeFile: readString(raw, 'composeFile'),
    relaxSecurity: readBoolean(raw, 'relaxSecurity'),
    status: parseWargameStatus(raw.status),
    bindIp: readString(raw, 'bindIp'),
    ports: parseWargamePorts(raw.ports),
    warnings: parseStringArray(raw.warnings),
    expiresAt: readNumber(raw, 'expiresAt'),
    createdAt: readNumber(raw, 'createdAt'),
    lastError: readString(raw, 'lastError'),
  }
}

export function parseWargameInstance(payload: unknown): WargameInstance {
  if (!isRecord(payload)) {
    throw new Error('워게임 인스턴스 응답이 올바르지 않습니다.')
  }
  const target = isRecord(payload.instance) ? payload.instance : payload
  const instance = parseWargameInstanceRecord(target)
  if (instance.id === '') {
    throw new Error('워게임 인스턴스 ID가 누락되었습니다.')
  }
  return instance
}

export function parseWargameList(payload: unknown): WargameListResult {
  if (!isRecord(payload)) {
    throw new Error('워게임 목록 응답이 올바르지 않습니다.')
  }
  const instancesRaw = Array.isArray(payload.instances) ? payload.instances : []
  const instances: WargameInstance[] = []
  for (const item of instancesRaw) {
    if (!isRecord(item)) continue
    const parsed = parseWargameInstanceRecord(item)
    if (parsed.id !== '') {
      instances.push(parsed)
    }
  }
  return {
    dockerAvailable: readBoolean(payload, 'dockerAvailable'),
    dockerError: readString(payload, 'dockerError'),
    bindIp: readString(payload, 'bindIp'),
    instances,
  }
}

export function parseWargameLogs(payload: unknown): WargameLogsResult {
  if (!isRecord(payload)) {
    throw new Error('워게임 로그 응답이 올바르지 않습니다.')
  }
  return {
    buildLog: readString(payload, 'buildLog'),
    containerLog: readString(payload, 'containerLog'),
    status: parseWargameStatus(payload.status),
  }
}

function readStringOrNumber(obj: Record<string, unknown>, key: string, fallback = ''): string {
  const val = obj[key]
  if (typeof val === 'string') return val
  if (typeof val === 'number' && Number.isFinite(val)) return String(val)
  return fallback
}

function extractListArray(payload: unknown, wrapperKey: string): readonly unknown[] {
  if (Array.isArray(payload)) return payload
  if (isRecord(payload)) {
    const wrapped = payload[wrapperKey]
    if (Array.isArray(wrapped)) return wrapped
  }
  return []
}

export function parseDockerOverview(payload: unknown): DockerOverview {
  if (!isRecord(payload)) {
    throw new Error('Docker 개요 응답이 올바르지 않습니다.')
  }
  const usageRaw = Array.isArray(payload.diskUsage) ? payload.diskUsage : []
  const diskUsage: DockerDiskUsageItem[] = []
  for (const item of usageRaw) {
    if (!isRecord(item)) continue
    diskUsage.push({
      type: readString(item, 'type'),
      totalCount: readStringOrNumber(item, 'totalCount', readStringOrNumber(item, 'count', '0')),
      active: readStringOrNumber(item, 'active', '0'),
      size: readString(item, 'size', '0B'),
      reclaimable: readString(item, 'reclaimable', '0B'),
    })
  }
  return {
    available: readBoolean(payload, 'available'),
    error: readString(payload, 'error'),
    serverVersion: readString(payload, 'serverVersion'),
    containersRunning: readNumber(payload, 'containersRunning'),
    containersStopped: readNumber(payload, 'containersStopped'),
    imagesCount: readNumber(payload, 'imagesCount'),
    volumesCount: readNumber(payload, 'volumesCount'),
    networksCount: readNumber(payload, 'networksCount'),
    diskUsage,
  }
}

export function parseDockerContainers(payload: unknown): readonly DockerContainer[] {
  const listRaw = extractListArray(payload, 'containers')
  const out: DockerContainer[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const id = readString(item, 'id')
    if (id === '') continue
    out.push({
      id,
      name: readString(item, 'name', id.slice(0, 12)),
      image: readString(item, 'image'),
      state: readString(item, 'state'),
      status: readString(item, 'status'),
      ports: readString(item, 'ports'),
      createdAt: readStringOrNumber(item, 'createdAt'),
      isWargame: readBoolean(item, 'isWargame'),
      cpuPercent: readStringOrNumber(item, 'cpuPercent'),
      memUsage: readString(item, 'memUsage'),
      memPercent: readStringOrNumber(item, 'memPercent'),
    })
  }
  return out
}

export function parseDockerContainerInspect(payload: unknown): DockerContainerInspect {
  if (!isRecord(payload)) {
    throw new Error('컨테이너 상세 정보 응답이 올바르지 않습니다.')
  }
  const mountsRaw = Array.isArray(payload.mounts) ? payload.mounts : []
  const mounts: DockerContainerMount[] = []
  for (const m of mountsRaw) {
    if (typeof m === 'string' && m.trim() !== '') {
      mounts.push({ type: 'bind', source: m, destination: '', rw: true })
    } else if (isRecord(m)) {
      mounts.push({
        type: readString(m, 'type', 'bind'),
        source: readString(m, 'source'),
        destination: readString(m, 'destination'),
        rw: readBoolean(m, 'rw', true),
      })
    }
  }
  return {
    id: readString(payload, 'id'),
    name: readString(payload, 'name'),
    image: readString(payload, 'image'),
    state: readString(payload, 'state'),
    restartPolicy: readString(payload, 'restartPolicy'),
    env: parseStringArray(payload.env),
    mounts,
    networks: parseStringArray(payload.networks),
    ports: parseStringArray(payload.ports),
  }
}

export function parseDockerImages(payload: unknown): readonly DockerImage[] {
  const listRaw = extractListArray(payload, 'images')
  const out: DockerImage[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const id = readString(item, 'id')
    if (id === '') continue
    out.push({
      id,
      repository: readString(item, 'repository', '<none>'),
      tag: readString(item, 'tag', '<none>'),
      size: readStringOrNumber(item, 'size'),
      createdAt: readStringOrNumber(item, 'createdAt'),
      inUse: readBoolean(item, 'inUse'),
    })
  }
  return out
}

export function parseDockerStacks(payload: unknown): readonly DockerStack[] {
  const listRaw = extractListArray(payload, 'stacks')
  const out: DockerStack[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const name = readString(item, 'name')
    if (name === '') continue
    out.push({
      name,
      status: readString(item, 'status', 'stopped'),
      servicesCount: readNumber(item, 'servicesCount'),
      runningCount: readNumber(item, 'runningCount'),
      updatedAt: readStringOrNumber(item, 'updatedAt'),
    })
  }
  return out
}

export function parseDockerStackDetail(payload: unknown): DockerStackDetail {
  if (!isRecord(payload)) {
    throw new Error('스택 상세 정보 응답이 올바르지 않습니다.')
  }
  return {
    name: readString(payload, 'name'),
    composeYaml: readString(payload, 'composeYaml'),
    status: readString(payload, 'status'),
  }
}

export function parseDockerVolumes(payload: unknown): readonly DockerVolume[] {
  const listRaw = extractListArray(payload, 'volumes')
  const out: DockerVolume[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const name = readString(item, 'name')
    if (name === '') continue
    out.push({
      name,
      driver: readString(item, 'driver', 'local'),
      mountpoint: readString(item, 'mountpoint'),
      createdAt: readStringOrNumber(item, 'createdAt'),
    })
  }
  return out
}

export function parseDockerNetworks(payload: unknown): readonly DockerNetwork[] {
  const listRaw = extractListArray(payload, 'networks')
  const out: DockerNetwork[] = []
  for (const item of listRaw) {
    if (!isRecord(item)) continue
    const id = readString(item, 'id')
    const name = readString(item, 'name')
    if (id === '' && name === '') continue
    out.push({
      id: id !== '' ? id : name,
      name: name !== '' ? name : id,
      driver: readString(item, 'driver'),
      scope: readString(item, 'scope', 'local'),
    })
  }
  return out
}

export function parseDockerTextOutput(payload: unknown): string {
  if (typeof payload === 'string') return payload
  if (isRecord(payload)) {
    const logs = payload.logs
    if (typeof logs === 'string') return logs
    const output = payload.output
    if (typeof output === 'string') return output
    const message = payload.message
    if (typeof message === 'string') return message
  }
  return ''
}
