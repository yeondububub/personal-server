<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { Save, CheckCircle2, AlertCircle } from 'lucide-vue-next'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { basicSetup } from 'codemirror'
import { oneDark } from '@codemirror/theme-one-dark'
import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { html } from '@codemirror/lang-html'
import { css } from '@codemirror/lang-css'
import { python } from '@codemirror/lang-python'
import { markdown } from '@codemirror/lang-markdown'
import { yaml } from '@codemirror/lang-yaml'
import { sql } from '@codemirror/lang-sql'
import { api } from '../../api/client'
import { useDesktopStore } from '../../stores/desktop'
import { formatBytes } from '../../utils/format'

const props = defineProps<{
  readonly windowId: string
  readonly rootId: string
  readonly relPath: string
  readonly fileName: string
}>()

const desktopStore = useDesktopStore()

const editorHostRef = ref<HTMLDivElement | null>(null)
const loading = ref(true)
const saving = ref(false)
const dirty = ref(false)
const readOnly = ref(false)
const savedNotice = ref(false)
const errorMessage = ref('')
const byteSize = ref(0)

let view: EditorView | null = null
let savedBaseline = ''

function languageExtensionFor(name: string): Extension {
  const lower = name.toLowerCase()
  if (lower.endsWith('.ts') || lower.endsWith('.tsx')) return javascript({ typescript: true, jsx: lower.endsWith('.tsx') })
  if (lower.endsWith('.js') || lower.endsWith('.jsx') || lower.endsWith('.mjs')) return javascript({ jsx: lower.endsWith('.jsx') })
  if (lower.endsWith('.json')) return json()
  if (lower.endsWith('.html') || lower.endsWith('.vue') || lower.endsWith('.xml')) return html()
  if (lower.endsWith('.css') || lower.endsWith('.scss')) return css()
  if (lower.endsWith('.py')) return python()
  if (lower.endsWith('.md')) return markdown()
  if (lower.endsWith('.yaml') || lower.endsWith('.yml')) return yaml()
  if (lower.endsWith('.sql')) return sql()
  return []
}

watch(dirty, (isDirty) => {
  const prefix = isDirty ? '● ' : ''
  desktopStore.setWindowTitle({
    windowId: props.windowId,
    title: `${prefix}Code Editor - ${props.fileName}`,
  })
})

async function saveDocument(): Promise<void> {
  if (!view || saving.value || readOnly.value) return
  const content = view.state.doc.toString()
  saving.value = true
  errorMessage.value = ''
  try {
    await api.writeTextFile({
      rootId: props.rootId,
      relPath: props.relPath,
      content,
    })
    savedBaseline = content
    byteSize.value = new TextEncoder().encode(content).byteLength
    dirty.value = false
    savedNotice.value = true
    setTimeout(() => {
      savedNotice.value = false
    }, 2000)
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '파일 저장에 실패했습니다.'
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    const doc = await api.readTextFile({ rootId: props.rootId, relPath: props.relPath })
    savedBaseline = doc.content
    byteSize.value = doc.size
    readOnly.value = doc.readOnly

    if (editorHostRef.value) {
      const state = EditorState.create({
        doc: doc.content,
        extensions: [
          basicSetup,
          oneDark,
          languageExtensionFor(props.fileName),
          EditorState.readOnly.of(doc.readOnly),
          keymap.of([
            {
              key: 'Mod-s',
              run: () => {
                void saveDocument()
                return true
              },
            },
          ]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              dirty.value = update.state.doc.toString() !== savedBaseline
            }
          }),
          EditorView.theme({
            '&': { height: '100%', fontSize: '13px' },
            '.cm-scroller': { overflow: 'auto', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' },
          }),
        ],
      })
      view = new EditorView({
        state,
        parent: editorHostRef.value,
      })
    }
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '파일을 불러오지 못했습니다.'
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (view) {
    view.destroy()
    view = null
  }
})
</script>

<template>
  <div class="flex h-full w-full flex-col bg-[#282c34] text-xs text-slate-200">
    <!-- Editor Toolbar -->
    <div class="flex items-center justify-between border-b border-white/10 bg-slate-950/90 px-3 py-2">
      <div class="flex min-w-0 items-center gap-2">
        <span class="truncate font-mono text-xs text-slate-300">{{ rootId }}:/{{ relPath }}</span>
        <span class="text-[11px] text-slate-500">({{ formatBytes(byteSize) }})</span>
        <span v-if="dirty" class="rounded bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300">
          수정됨
        </span>
        <span
          v-else-if="savedNotice"
          class="inline-flex items-center gap-1 text-[11px] text-emerald-400"
        >
          <CheckCircle2 class="h-3.5 w-3.5" />
          저장 완료
        </span>
      </div>

      <button
        v-if="!readOnly"
        type="button"
        :disabled="saving || loading"
        class="inline-flex items-center gap-1.5 rounded-lg bg-emerald-600 px-3 py-1.5 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
        @click="saveDocument"
      >
        <Save class="h-3.5 w-3.5" />
        {{ saving ? '저장 중...' : '저장 (⌘/Ctrl+S)' }}
      </button>
      <span
        v-else
        class="rounded-lg border border-amber-400/30 bg-amber-500/15 px-2.5 py-1 text-[11px] font-semibold text-amber-300"
      >
        읽기 전용
      </span>
    </div>

    <div
      v-if="errorMessage"
      class="flex items-center gap-2 border-b border-rose-500/30 bg-rose-500/15 px-3 py-2 text-rose-200"
    >
      <AlertCircle class="h-4 w-4 shrink-0" />
      <span>{{ errorMessage }}</span>
    </div>

    <div v-if="loading" class="flex flex-1 items-center justify-center text-slate-400">
      파일을 불러오는 중...
    </div>

    <div v-show="!loading" ref="editorHostRef" class="min-h-0 flex-1 overflow-hidden" />
  </div>
</template>
