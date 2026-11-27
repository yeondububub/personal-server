export interface SubtitleCue {
  readonly startSec: number
  readonly endSec: number
  readonly text: string
}

const HTML_ENTITIES: Readonly<Record<string, string>> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: ' ',
}

function decodeHtmlEntities(input: string): string {
  return input.replace(/&(#x?[0-9a-fA-F]+|[a-zA-Z]+);/g, (match, body: string) => {
    if (body.startsWith('#x') || body.startsWith('#X')) {
      const code = Number.parseInt(body.slice(2), 16)
      return Number.isFinite(code) && code > 0 ? String.fromCodePoint(code) : match
    }
    if (body.startsWith('#')) {
      const code = Number.parseInt(body.slice(1), 10)
      return Number.isFinite(code) && code > 0 ? String.fromCodePoint(code) : match
    }
    const lower = body.toLowerCase()
    const mapped = HTML_ENTITIES[lower]
    return mapped ?? match
  })
}

function cleanCueText(raw: string): string {
  const withBreaks = raw
    .replace(/\r\n?/g, '\n')
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<\/p\s*>/gi, '\n')
    .replace(/\{\\[^}]*\}/g, '')
    .replace(/<[^>]+>/g, '')
  const decoded = decodeHtmlEntities(withBreaks)
  const lines = decoded
    .split('\n')
    .map((line) => line.replace(/[ \t]+/g, ' ').trim())
    .filter((line) => line !== '')
  return lines.join('\n')
}

function parseTimestampSec(raw: string): number | null {
  const trimmed = raw.trim()
  const match = /^(?:(\d+):)?(\d{1,2}):(\d{2})(?:[.,](\d{1,3}))?$/.exec(trimmed)
  if (!match) return null
  const hours = match[1] !== undefined ? Number.parseInt(match[1], 10) : 0
  const mins = Number.parseInt(match[2] ?? '0', 10)
  const secs = Number.parseInt(match[3] ?? '0', 10)
  const msRaw = match[4] ?? '0'
  const ms = Number.parseInt(msRaw.padEnd(3, '0'), 10)
  if (!Number.isFinite(hours) || !Number.isFinite(mins) || !Number.isFinite(secs) || !Number.isFinite(ms)) {
    return null
  }
  return hours * 3600 + mins * 60 + secs + ms / 1000
}

function sortCues(cues: SubtitleCue[]): readonly SubtitleCue[] {
  return cues.sort((a, b) => a.startSec - b.startSec || a.endSec - b.endSec)
}

export function decodeSubtitleBuffer(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  if (bytes.length >= 2) {
    const b0 = bytes[0]
    const b1 = bytes[1]
    if (b0 === 0xff && b1 === 0xfe) {
      return new TextDecoder('utf-16le').decode(bytes.subarray(2))
    }
    if (b0 === 0xfe && b1 === 0xff) {
      return new TextDecoder('utf-16be').decode(bytes.subarray(2))
    }
  }
  try {
    const decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
    return decoded.startsWith('\uFEFF') ? decoded.slice(1) : decoded
  } catch {
    return new TextDecoder('euc-kr').decode(bytes)
  }
}

export function parseWebVTT(vtt: string): readonly SubtitleCue[] {
  const normalized = vtt.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const blocks = normalized.split(/\n{2,}/)
  const cues: SubtitleCue[] = []

  for (const block of blocks) {
    const lines = block
      .split('\n')
      .map((l) => l.trimEnd())
      .filter((l) => l.trim() !== '')
    if (lines.length === 0) continue

    const first = (lines[0] ?? '').trim()
    if (
      first.startsWith('WEBVTT') ||
      first.startsWith('NOTE') ||
      first.startsWith('STYLE') ||
      first.startsWith('REGION')
    ) {
      continue
    }

    let timingIndex = 0
    if (!first.includes('-->')) {
      timingIndex = 1
    }
    const timingLine = lines[timingIndex]
    if (!timingLine || !timingLine.includes('-->')) continue

    const arrowIdx = timingLine.indexOf('-->')
    const startRaw = timingLine.slice(0, arrowIdx).trim()
    const endSegment = timingLine.slice(arrowIdx + 3).trim()
    const endRaw = endSegment.split(/\s+/)[0] ?? ''

    const startSec = parseTimestampSec(startRaw)
    const endSec = parseTimestampSec(endRaw)
    if (startSec === null || endSec === null || endSec <= startSec) continue

    const text = cleanCueText(lines.slice(timingIndex + 1).join('\n'))
    if (text === '') continue

    cues.push({ startSec, endSec, text })
  }

  return sortCues(cues)
}

export function parseSRT(srt: string): readonly SubtitleCue[] {
  const normalized = srt.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const blocks = normalized.split(/\n{2,}/)
  const cues: SubtitleCue[] = []

  for (const block of blocks) {
    const lines = block
      .split('\n')
      .map((l) => l.trimEnd())
      .filter((l) => l.trim() !== '')
    if (lines.length < 2) continue

    let timingIndex = 0
    const first = lines[0] ?? ''
    if (!first.includes('-->')) {
      timingIndex = 1
    }
    const timingLine = lines[timingIndex]
    if (!timingLine || !timingLine.includes('-->')) continue

    const arrowIdx = timingLine.indexOf('-->')
    const startRaw = timingLine.slice(0, arrowIdx).trim()
    const endSegment = timingLine.slice(arrowIdx + 3).trim()
    const endRaw = endSegment.split(/\s+/)[0] ?? ''

    const startSec = parseTimestampSec(startRaw)
    const endSec = parseTimestampSec(endRaw)
    if (startSec === null || endSec === null || endSec <= startSec) continue

    const text = cleanCueText(lines.slice(timingIndex + 1).join('\n'))
    if (text === '') continue

    cues.push({ startSec, endSec, text })
  }

  return sortCues(cues)
}

export function parseSMI(smi: string): readonly SubtitleCue[] {
  const normalized = smi.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const bodyMatch = /<body[^>]*>([\s\S]*?)<\/body>/i.exec(normalized)
  const source = bodyMatch?.[1] ?? normalized

  const syncRegex = /<sync\b[^>]*\bstart\s*=\s*["']?(-?\d+)["']?[^>]*>/gi
  const rawSyncs: { readonly startSec: number; readonly contentStart: number; readonly tagStart: number }[] = []

  let match = syncRegex.exec(source)
  while (match !== null) {
    const ms = Number.parseInt(match[1] ?? '', 10)
    if (Number.isFinite(ms) && ms >= 0) {
      rawSyncs.push({
        startSec: ms / 1000,
        tagStart: match.index,
        contentStart: match.index + match[0].length,
      })
    }
    match = syncRegex.exec(source)
  }

  if (rawSyncs.length === 0) return []

  const cues: SubtitleCue[] = []
  for (let i = 0; i < rawSyncs.length; i += 1) {
    const current = rawSyncs[i]
    if (!current) continue
    const next = rawSyncs[i + 1]
    const rawSlice = next ? source.slice(current.contentStart, next.tagStart) : source.slice(current.contentStart)
    const text = cleanCueText(rawSlice)
    if (text === '') continue

    const endSec = next && next.startSec > current.startSec ? next.startSec : current.startSec + 5
    cues.push({
      startSec: current.startSec,
      endSec,
      text,
    })
  }

  return sortCues(cues)
}

export function parseSubtitleFile(filename: string, raw: string): readonly SubtitleCue[] {
  const lower = filename.trim().toLowerCase()
  if (lower.endsWith('.vtt')) return parseWebVTT(raw)
  if (lower.endsWith('.srt')) return parseSRT(raw)
  if (lower.endsWith('.smi') || lower.endsWith('.sami')) return parseSMI(raw)

  const head = raw.replace(/^\uFEFF/, '').trimStart().slice(0, 256).toUpperCase()
  if (head.startsWith('WEBVTT')) return parseWebVTT(raw)
  if (head.includes('<SAMI') || head.includes('<SYNC')) return parseSMI(raw)
  if (raw.includes('-->')) return parseSRT(raw)
  return []
}

export function activeCuesAt(
  cues: readonly SubtitleCue[],
  videoTimeSec: number,
  offsetSec: number,
): readonly SubtitleCue[] {
  if (cues.length === 0 || !Number.isFinite(videoTimeSec)) return []
  const shifted = videoTimeSec - (Number.isFinite(offsetSec) ? offsetSec : 0)
  const active: SubtitleCue[] = []
  for (const cue of cues) {
    if (cue.startSec > shifted) break
    if (shifted >= cue.startSec && shifted < cue.endSec) {
      active.push(cue)
    }
  }
  return active
}
