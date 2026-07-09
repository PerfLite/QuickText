<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, nextTick, watch, computed } from 'vue'
import * as monaco from 'monaco-editor'
import { marked } from 'marked'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { EventsOn } from '../wailsjs/runtime/runtime'

// --- i18n ---
const lang = ref(localStorage.getItem('lang') || 'ru')
const i18n = {
  ru: {
    open: 'Открыть', save: 'Сохранить', saveAs: 'Сохранить как...', openFolder: 'Открыть папку',
    settings: 'Настройки', fontSize: 'Размер шрифта', welcome: 'Приветствие', language: 'Язык',
    sidebar: 'Переключить сайдбар', menu: 'Меню', terminal: 'Терминал',
    openHint: 'Ctrl+O для открытия файла', saveQuestion: 'Сохранить изменения?',
    saveMsg: 'Есть несохранённые файлы', cancel: 'Отмена', discard: 'Выйти', saveAndExit: 'Сохранить и выйти',
    saved: 'Сохранено...', savedAs: 'Сохранено как: ', readingError: 'Ошибка чтения файла: ',
    saveError: 'Ошибка сохранения: ', folderError: 'Ошибка загрузки дерева: ',
    opened: 'Открыт: ', preview: 'Превью', editorView: 'Редактор', splitView: 'Разделить',
  },
  en: {
    open: 'Open', save: 'Save', saveAs: 'Save as...', openFolder: 'Open folder',
    settings: 'Settings', fontSize: 'Font size', welcome: 'Welcome tab', language: 'Language',
    sidebar: 'Toggle sidebar', menu: 'Menu', terminal: 'Terminal',
    openHint: 'Ctrl+O to open a file', saveQuestion: 'Save changes?',
    saveMsg: 'You have unsaved files', cancel: 'Cancel', discard: 'Exit', saveAndExit: 'Save and exit',
    saved: 'Saved...', savedAs: 'Saved as: ', readingError: 'Read error: ',
    saveError: 'Save error: ', folderError: 'Folder tree error: ',
    opened: 'Opened: ', preview: 'Preview', editorView: 'Editor', splitView: 'Split',
  }
}
watch(lang, (v) => localStorage.setItem('lang', v))
const t = computed(() => i18n[lang.value] || i18n.ru)

// --- State ---
const sidebarVisible = ref(localStorage.getItem('sidebar_open') === '1')
const isZenMode = ref(false)
const fontSize = ref(14)
const showWelcomeTab = ref(localStorage.getItem('showWelcomeTab') !== 'false')
const renderedHtml = ref('')
let renderTimer = null
let autoSaveTimer = null

// --- View mode for markdown: 'split' | 'editor' | 'preview' ---
const viewMode = ref('split')
const editorWidthPercent = ref(50)
let isResizing = false

// --- Terminal ---
const showTerminal = ref(false)
const terminalContainer = ref(null)
let xterm = null
let fitAddon = null
const menuOpen = ref(false)
const settingsOpen = ref(false)
const showCloseDialog = ref(false)
const statusMessage = ref('')
const statusTimeout = ref(null)
const rootPath = ref('')
const sidebarWidth = ref(220)

const tabs = ref([])
const activeTabId = ref(null)
const editorContainer = ref(null)

let editor = null
const models = {}
const viewStates = {}

// --- Computed ---
const activeTab = computed(() => tabs.value.find(t => t.id === activeTabId.value))
const hasDirtyTabs = computed(() => tabs.value.some(t => t.isDirty))
const isMarkdown = computed(() => {
  if (!activeTab.value) return false
  const name = activeTab.value.name.toLowerCase()
  return name.endsWith('.md') || name.endsWith('.markdown')
})

// --- File tree ---
const folderTree = ref([])

// --- Helpers ---
let tabIdCounter = 0
function nextTabId() { return ++tabIdCounter }

function setStatus(msg, duration = 2000) {
  statusMessage.value = msg
  if (statusTimeout.value) clearTimeout(statusTimeout.value)
  statusTimeout.value = setTimeout(() => { statusMessage.value = '' }, duration)
}

// --- Tab operations ---
function activateTab(id) {
  if (!editor || !models[id]) return
  if (activeTabId.value && viewStates[activeTabId.value]) {
    viewStates[activeTabId.value] = editor.saveViewState()
  }
  activeTabId.value = id
  editor.setModel(models[id])
  if (viewStates[id]) {
    editor.restoreViewState(viewStates[id])
  }
}

function closeTab(id) {
  const idx = tabs.value.findIndex(t => t.id === id)
  if (idx === -1) return
  const tab = tabs.value[idx]

  if (tab.isDirty) {
    if (!confirm(`Файл "${tab.name}" не сохранён. Закрыть без сохранения?`)) return
  }

  if (models[id]) {
    models[id].dispose()
    delete models[id]
  }
  delete viewStates[id]

  tabs.value.splice(idx, 1)

  if (activeTabId.value === id) {
    if (tabs.value.length > 0) {
      const nextIdx = idx < tabs.value.length ? idx : idx - 1
      activateTab(tabs.value[Math.max(0, nextIdx)].id)
    } else {
      activeTabId.value = null
    }
  }
}

async function openFile(path, name) {
  const existing = tabs.value.find(t => t.path === path)
  if (existing) {
    activateTab(existing.id)
    return
  }

  try {
    const content = await window.go.main.App.OpenFile(path)
    const id = nextTabId()
    const tab = { id, name: name || path.split('/').pop(), path, isDirty: false }
    tabs.value.push(tab)

    const model = monaco.editor.createModel(content, undefined, monaco.Uri.file(path))
    models[id] = model

    model.onDidChangeContent(() => {
      tab.isDirty = model.getValue() !== (tab._savedContent ?? content)
      scheduleAutoSave()
    })

    tab._savedContent = content
    activateTab(id)
  } catch (e) {
    setStatus(t.value.readingError + e)
  }
}

// --- Backend wrappers ---
async function handleOpenFile() {
  menuOpen.value = false
  const path = await window.go.main.App.OpenFileChooser()
  if (path) await openFile(path)
}

async function handleSave() {
  menuOpen.value = false
  const tab = activeTab.value
  if (!tab) return
  const content = models[tab.id]?.getValue() ?? ''
  try {
    await window.go.main.App.SaveFile(tab.path, content)
    tab._savedContent = content
    tab.isDirty = false
    setStatus(t.value.saved)
    window.go.main.App.ClearAutoSave()
  } catch (e) {
    setStatus(t.value.saveError + e)
  }
}

async function handleSaveAs() {
  menuOpen.value = false
  const tab = activeTab.value
  if (!tab) return
  const path = await window.go.main.App.SaveFileChooser()
  if (!path) return

  const content = models[tab.id]?.getValue() ?? ''
  try {
    await window.go.main.App.SaveFile(path, content)
    tab.path = path
    tab.name = path.split('/').pop()
    tab._savedContent = content
    tab.isDirty = false

    if (models[tab.id]) {
      const oldModel = models[tab.id]
      const newModel = monaco.editor.createModel(content, undefined, monaco.Uri.file(path))
      models[tab.id] = newModel
      if (editor && activeTabId.value === tab.id) {
        editor.setModel(newModel)
      }
      oldModel.dispose()
    }
    setStatus(t.value.savedAs + tab.name)
  } catch (e) {
    setStatus(t.value.saveError + e)
  }
}

async function loadFolderTree(path) {
  rootPath.value = path
  try {
    folderTree.value = await window.go.main.App.GetFolderTree(path)
  } catch (e) {
    setStatus(t.value.folderError + e)
  }
}

async function handleOpenFolder() {
  menuOpen.value = false
  const path = await window.go.main.App.OpenFileChooser()
  if (path) {
    const folder = path.substring(0, path.lastIndexOf('/'))
    await loadFolderTree(folder)
  }
}

function toggleMenu() {
  menuOpen.value = !menuOpen.value
  settingsOpen.value = false
}

function toggleSettings() {
  settingsOpen.value = !settingsOpen.value
  menuOpen.value = false
}

function closeDialogSave() {
  showCloseDialog.value = false
  saveAllDirty().then(() => {
    window.go.main.App.ForceClose()
  })
}

function closeDialogDiscard() {
  showCloseDialog.value = false
  window.go.main.App.ForceClose()
}

function closeDialogCancel() {
  showCloseDialog.value = false
}

async function saveAllDirty() {
  for (const tab of tabs.value) {
    if (tab.isDirty && models[tab.id]) {
      try {
        await window.go.main.App.SaveFile(tab.path, models[tab.id].getValue())
        tab.isDirty = false
      } catch (e) { /* ignore */ }
    }
  }
  window.go.main.App.ClearAutoSave()
}

// --- Font size / zoom ---
function updateFontSize(newSize) {
  fontSize.value = Math.max(8, Math.min(40, newSize))
  if (editor) {
    editor.updateOptions({ fontSize: fontSize.value })
  }
}

// --- Sidebar tree item click ---
function onTreeItemClick(node) {
  if (!node.isDir) {
    openFile(node.path, node.name)
  }
}

// --- Markdown Preview ---
function updatePreview() {
  if (!editor || !isMarkdown.value) {
    renderedHtml.value = ''
    return
  }
  const content = editor.getValue()
  renderedHtml.value = marked.parse(content)
}

function schedulePreviewUpdate() {
  clearTimeout(renderTimer)
  renderTimer = setTimeout(updatePreview, 200)
}

// --- Autosave ---
function scheduleAutoSave() {
  clearTimeout(autoSaveTimer)
  autoSaveTimer = setTimeout(doAutoSave, 10000)
}

async function doAutoSave() {
  const dirtyTabs = tabs.value.filter(t => t.isDirty && models[t.id])
  if (dirtyTabs.length === 0) return
  const data = dirtyTabs.map(t => ({
    path: t.path,
    name: t.name,
    content: models[t.id].getValue(),
  }))
  try {
    await window.go.main.App.SaveAutoSave(JSON.stringify(data))
  } catch (e) { /* ignore */ }
}

async function loadAutoSave() {
  try {
    const raw = await window.go.main.App.LoadAutoSave()
    if (!raw) return
    const data = JSON.parse(raw)
    for (const item of data) {
      if (item.path && item.content) {
        await openFile(item.path, item.name)
        if (models[tabs.value[tabs.value.length - 1]?.id]) {
          const id = tabs.value[tabs.value.length - 1].id
          models[id].setValue(item.content)
        }
      }
    }
  } catch (e) { /* ignore */ }
}

// --- Search toggle ---
function toggleSearch() {
  if (!editor) return
  const controller = editor.getContribution('editor.contrib.findController')
  if (controller) {
    if (controller.getState()?.matchFind) {
      controller.closeFindWidget()
    } else {
      editor.getAction('actions.find').run()
    }
  }
}

// --- Resize split ---
function startResize(e) {
  isResizing = true
  const container = e.target.parentElement
  const rect = container.getBoundingClientRect()
  const startX = e.clientX
  const startPercent = editorWidthPercent.value

  function onMove(ev) {
    if (!isResizing) return
    const dx = ev.clientX - startX
    const pct = startPercent + (dx / rect.width) * 100
    editorWidthPercent.value = Math.max(20, Math.min(80, pct))
  }
  function onUp() {
    isResizing = false
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

// --- Terminal ---
async function toggleTerminal() {
  showTerminal.value = !showTerminal.value
  if (showTerminal.value) {
    await nextTick()
    initTerminal()
  } else {
    destroyTerminal()
  }
}

function initTerminal() {
  if (xterm) return

  xterm = new Terminal({
    fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace",
    fontSize: 13,
    theme: {
      background: '#141416',
      foreground: '#d4d4d8',
      cursor: '#d4d4d8',
      selectionBackground: '#3f3f46',
      black: '#18181b',
      red: '#f87171',
      green: '#4ade80',
      yellow: '#facc15',
      blue: '#60a5fa',
      magenta: '#c084fc',
      cyan: '#22d3ee',
      white: '#d4d4d8',
    },
    cursorBlink: true,
    scrollback: 5000,
  })

  fitAddon = new FitAddon()
  xterm.loadAddon(fitAddon)

  xterm.open(terminalContainer.value)
  fitAddon.fit()

  xterm.onData((data) => {
    window.go.main.App.WriteTerminal(data)
  })

  xterm.onResize(({ cols, rows }) => {
    window.go.main.App.ResizeTerminal(cols, rows)
  })

  EventsOn('terminal-output', (data) => {
    xterm?.write(data)
  })

  EventsOn('terminal-closed', () => {
    xterm?.write('\r\n\x1b[33m[Terminal closed]\x1b[0m\r\n')
  })

  window.go.main.App.StartTerminal().then(() => {
    const { cols, rows } = xterm
    window.go.main.App.ResizeTerminal(cols, rows)
  })

  const resizeObserver = new ResizeObserver(() => {
    if (showTerminal.value && fitAddon && xterm) {
      fitAddon.fit()
    }
  })
  resizeObserver.observe(terminalContainer.value)
}

function destroyTerminal() {
  if (xterm) {
    window.go.main.App.CloseTerminal()
    xterm.dispose()
    xterm = null
    fitAddon = null
  }
}

watch(showWelcomeTab, (val) => {
  localStorage.setItem('showWelcomeTab', val)
})

// --- Init ---
watch(activeTabId, () => {
  nextTick(updatePreview)
})

onMounted(async () => {
  EventsOn('user_wants_to_close', async () => {
    if (hasDirtyTabs.value) {
      showCloseDialog.value = true
    } else {
      window.go.main.App.ForceClose()
    }
  })

  const pendingFile = await window.go.main.App.GetPendingFile()
  if (pendingFile) {
    openFile(pendingFile)
  }

  await nextTick()

  editor = monaco.editor.create(editorContainer.value, {
    value: '',
    language: 'plaintext',
    fontSize: fontSize.value,
    fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', monospace",
    minimap: { enabled: true, scale: 1 },
    scrollBeyondLastLine: false,
    renderWhitespace: 'selection',
    tabSize: 4,
    automaticLayout: true,
    smoothScrolling: true,
    cursorBlinking: 'smooth',
    cursorSmoothCaretAnimation: 'on',
    bracketPairColorization: { enabled: false },
    matchBrackets: false,
    guides: { bracketPairs: false },
    unicodeHighlight: { ambiguousCharacters: false, includeComments: false, includeStrings: false },
    autoClosingBrackets: 'always',
    autoClosingQuotes: 'always',
    autoSurround: 'languageDefined',
    mouseWheelZoom: true,
    wordWrap: 'off',
    lineNumbers: 'on',
    roundedSelection: true,
    padding: { top: 8 },
    theme: 'vs-dark',
    scrollbar: {
      verticalScrollbarSize: 10,
      horizontalScrollbarSize: 10,
    },
  })

  editor.onDidChangeModelContent(() => {
    schedulePreviewUpdate()
  })

  if (showWelcomeTab.value) {
    const welcomeId = nextTabId()
    const welcomeModel = monaco.editor.createModel(
      '# QuickText\n\nДобро пожаловать в QuickText — быстрый и минималистичный текстовый редактор.\n\n**Горячие клавиши:**\n- `Ctrl+O` — Открыть файл\n- `Ctrl+S` — Сохранить файл\n- `Ctrl+F` — Поиск\n- `Ctrl+Колесико` — Масштаб\n- `F11` — Zen-режим\n- `Ctrl+\\` — Терминал\n',
      'markdown'
    )
    models[welcomeId] = welcomeModel
    tabs.value.push({
      id: welcomeId,
      name: 'Добро пожаловать',
      path: '',
      isDirty: false,
      _savedContent: welcomeModel.getValue()
    })
    activateTab(welcomeId)
  }

  document.addEventListener('keydown', handleGlobalKey)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKey)
  clearTimeout(autoSaveTimer)
  destroyTerminal()
  if (editor) editor.dispose()
  Object.values(models).forEach(m => m.dispose())
})

function handleGlobalKey(e) {
  const code = e.code
  if ((e.ctrlKey || e.metaKey) && code === 'KeyS') {
    e.preventDefault()
    handleSave()
  }
  if ((e.ctrlKey || e.metaKey) && code === 'KeyO') {
    e.preventDefault()
    handleOpenFile()
  }
  if ((e.ctrlKey || e.metaKey) && code === 'KeyF') {
    e.preventDefault()
    toggleSearch()
  }
  if (code === 'F11') {
    e.preventDefault()
    isZenMode.value = !isZenMode.value
  }
  if ((e.ctrlKey || e.metaKey) && code === 'Backslash') {
    e.preventDefault()
    toggleTerminal()
  }
}
</script>

<template>
  <div class="flex flex-col h-screen bg-[#18181b] text-zinc-400 select-none">
    <!-- Title Bar -->
    <div
      class="flex items-center h-9 bg-[#141416] border-b border-zinc-800 px-1 shrink-0 transition-all duration-300"
      :class="isZenMode ? 'h-0 border-b-0 opacity-0 overflow-hidden pointer-events-none' : 'h-9'"
    >
      <!-- Menu Button -->
      <div class="relative">
        <button
          @click.stop="toggleMenu"
          class="flex items-center justify-center w-8 h-8 rounded hover:bg-zinc-800 text-zinc-400 hover:text-zinc-200 transition-colors"
          :title="t.menu"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/>
          </svg>
        </button>
        <div
          v-if="menuOpen"
          class="absolute top-full left-0 mt-1 w-56 bg-[#1e1e22] border border-zinc-700 rounded-lg shadow-2xl py-1 z-50"
        >
          <button @click="handleOpenFile" class="w-full px-4 py-2 text-left text-sm hover:bg-zinc-700 flex items-center gap-3">
            <span>{{ t.open }}</span>
            <span class="ml-auto text-zinc-600 text-xs">Ctrl+O</span>
          </button>
          <button @click="handleSave" class="w-full px-4 py-2 text-left text-sm hover:bg-zinc-700 flex items-center gap-3">
            <span>{{ t.save }}</span>
            <span class="ml-auto text-zinc-600 text-xs">Ctrl+S</span>
          </button>
          <button @click="handleSaveAs" class="w-full px-4 py-2 text-left text-sm hover:bg-zinc-700 flex items-center gap-3">
            <span>{{ t.saveAs }}</span>
          </button>
          <div class="border-t border-zinc-700 my-1"></div>
          <button @click="handleOpenFolder" class="w-full px-4 py-2 text-left text-sm hover:bg-zinc-700 flex items-center gap-3">
            <span>{{ t.openFolder }}</span>
          </button>
        </div>
      </div>

      <!-- Sidebar Toggle -->
      <button
        @click="sidebarVisible = !sidebarVisible; localStorage.setItem('sidebar_open', sidebarVisible ? '1' : '0')"
        class="flex items-center justify-center w-8 h-8 rounded hover:bg-zinc-800 text-zinc-400 hover:text-zinc-200 transition-colors"
        :title="t.sidebar"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h12M4 18h16"/>
        </svg>
      </button>

      <!-- Tabs -->
      <div class="flex items-center flex-1 h-full overflow-x-auto">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          @click="activateTab(tab.id)"
          class="group flex items-center gap-2 h-8 px-3 text-xs cursor-pointer border-r border-zinc-800 shrink-0 transition-colors"
          :class="activeTabId === tab.id
            ? 'bg-[#18181b] text-zinc-200 border-t-2 border-t-blue-500'
            : 'hover:bg-zinc-800 text-zinc-500 hover:text-zinc-300 border-t-2 border-t-transparent'"
        >
          <span v-if="tab.isDirty" class="w-2 h-2 rounded-full bg-blue-500 shrink-0"></span>
          <span class="truncate max-w-[140px]">{{ tab.name }}</span>
          <button
            @click.stop="closeTab(tab.id)"
            class="opacity-0 group-hover:opacity-100 w-4 h-4 flex items-center justify-center rounded hover:bg-zinc-600 text-zinc-400 hover:text-zinc-200 shrink-0"
          >
            <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- View mode toggle for markdown -->
      <div v-if="isMarkdown && !isZenMode" class="flex items-center gap-0.5 mr-2 bg-zinc-800 rounded p-0.5">
        <button
          @click="viewMode = 'editor'"
          class="px-2 py-0.5 text-xs rounded transition-colors"
          :class="viewMode === 'editor' ? 'bg-blue-600 text-white' : 'text-zinc-400 hover:text-zinc-200'"
          :title="t.editorView"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/>
          </svg>
        </button>
        <button
          @click="viewMode = 'split'"
          class="px-2 py-0.5 text-xs rounded transition-colors"
          :class="viewMode === 'split' ? 'bg-blue-600 text-white' : 'text-zinc-400 hover:text-zinc-200'"
          :title="t.splitView"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7"/>
          </svg>
        </button>
        <button
          @click="viewMode = 'preview'"
          class="px-2 py-0.5 text-xs rounded transition-colors"
          :class="viewMode === 'preview' ? 'bg-blue-600 text-white' : 'text-zinc-400 hover:text-zinc-200'"
          :title="t.preview"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
          </svg>
        </button>
      </div>

      <!-- Settings Button -->
      <button
        @click.stop="toggleSettings"
        class="flex items-center justify-center w-8 h-8 rounded hover:bg-zinc-800 text-zinc-400 hover:text-zinc-200 transition-colors"
        :title="t.settings"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
        </svg>
      </button>

      <!-- Settings Popup -->
      <div
        v-if="settingsOpen"
        class="absolute top-10 right-4 w-64 bg-[#1e1e22] border border-zinc-700 rounded-lg shadow-2xl p-4 z-50"
        @click.stop
      >
        <div class="text-xs text-zinc-500 uppercase tracking-wider mb-3">{{ t.settings }}</div>
        <div class="flex items-center justify-between mb-3">
          <label class="text-sm text-zinc-300">{{ t.fontSize }}</label>
          <div class="flex items-center gap-2">
            <button @click="updateFontSize(fontSize - 1)" class="w-6 h-6 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-300 text-xs flex items-center justify-center">−</button>
            <span class="text-sm text-zinc-200 w-8 text-center">{{ fontSize }}</span>
            <button @click="updateFontSize(fontSize + 1)" class="w-6 h-6 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-300 text-xs flex items-center justify-center">+</button>
          </div>
        </div>
        <div class="flex items-center justify-between mb-3">
          <label class="text-sm text-zinc-300">{{ t.welcome }}</label>
          <button
            @click="showWelcomeTab = !showWelcomeTab"
            class="relative w-9 h-5 rounded-full transition-colors"
            :class="showWelcomeTab ? 'bg-blue-600' : 'bg-zinc-700'"
          >
            <span
              class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform"
              :class="showWelcomeTab ? 'translate-x-4' : ''"
            ></span>
          </button>
        </div>
        <div class="flex items-center justify-between">
          <label class="text-sm text-zinc-300">{{ t.language }}</label>
          <div class="flex gap-1">
            <button
              @click="lang = 'ru'"
              class="px-2 py-0.5 text-xs rounded transition-colors"
              :class="lang === 'ru' ? 'bg-blue-600 text-white' : 'bg-zinc-700 text-zinc-400 hover:bg-zinc-600'"
            >RU</button>
            <button
              @click="lang = 'en'"
              class="px-2 py-0.5 text-xs rounded transition-colors"
              :class="lang === 'en' ? 'bg-blue-600 text-white' : 'bg-zinc-700 text-zinc-400 hover:bg-zinc-600'"
            >EN</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Sidebar -->
      <div
        v-if="sidebarVisible && !isZenMode"
        class="bg-[#141416] border-r border-zinc-800 overflow-y-auto shrink-0 transition-all duration-300"
        :style="{ width: sidebarWidth + 'px' }"
      >
        <div class="p-2">
          <div v-if="folderTree.length === 0" class="text-xs text-zinc-600 px-2 py-4 text-center">
            {{ lang === 'ru' ? 'Откройте папку через меню' : 'Open folder from menu' }}
          </div>
          <template v-for="node in folderTree" :key="node.path">
            <FolderNode :node="node" :depth="0" @open-file="onTreeItemClick" />
          </template>
        </div>
      </div>

      <!-- Editor + Preview -->
      <div class="flex-1 overflow-hidden flex">
        <!-- Editor -->
        <div
          v-show="viewMode !== 'preview'"
          class="overflow-hidden relative"
          :style="isMarkdown && !isZenMode && viewMode === 'split'
            ? { width: editorWidthPercent + '%', minWidth: '200px' }
            : { width: '100%' }"
        >
          <div ref="editorContainer" class="w-full h-full"></div>
          <div v-if="!activeTab" class="absolute inset-0 flex items-center justify-center pointer-events-none">
            <div class="text-center text-zinc-600">
              <svg class="w-16 h-16 mx-auto mb-4 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
              </svg>
              <p class="text-sm">{{ t.openHint }}</p>
            </div>
          </div>
        </div>

        <!-- Resize Handle -->
        <div
          v-if="isMarkdown && !isZenMode && viewMode === 'split'"
          class="w-1 bg-zinc-800 hover:bg-blue-600 cursor-col-resize shrink-0 transition-colors"
          @mousedown="startResize"
        ></div>

        <!-- Preview -->
        <div
          v-if="isMarkdown && !isZenMode && viewMode !== 'editor'"
          class="overflow-y-auto bg-[#141416] border-l border-zinc-800 p-6"
          :style="viewMode === 'split'
            ? { width: (100 - editorWidthPercent) + '%', minWidth: '200px' }
            : { width: '100%' }"
        >
          <div class="prose prose-invert prose-zinc max-w-none markdown-preview" v-html="renderedHtml"></div>
        </div>
      </div>
    </div>

    <!-- Terminal Panel -->
    <div
      v-if="showTerminal && !isZenMode"
      class="border-t border-zinc-800 bg-[#141416] transition-all duration-300"
      :style="{ height: '250px' }"
    >
      <div class="flex items-center justify-between px-3 py-1 bg-[#18181b] border-b border-zinc-800">
        <span class="text-xs text-zinc-500">{{ t.terminal }}</span>
        <button
          @click="toggleTerminal"
          class="w-5 h-5 flex items-center justify-center rounded hover:bg-zinc-700 text-zinc-400 hover:text-zinc-200 transition-colors"
        >
          <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
          </svg>
        </button>
      </div>
      <div ref="terminalContainer" class="h-[calc(250px-28px)] p-1"></div>
    </div>

    <!-- Status Bar -->
    <div
      class="flex items-center h-6 bg-[#141416] border-t border-zinc-800 px-3 text-xs shrink-0 transition-all duration-300 overflow-hidden"
      :class="isZenMode ? 'h-0 border-t-0 opacity-0' : 'h-6'"
    >
      <div class="flex items-center gap-4 flex-1">
        <span class="text-zinc-500">{{ activeTab?.path || '' }}</span>
      </div>
      <div class="flex items-center gap-4">
        <span v-if="statusMessage" class="text-blue-400 transition-opacity">{{ statusMessage }}</span>
        <button
          v-if="!isZenMode"
          @click="toggleTerminal"
          class="text-zinc-500 hover:text-zinc-300 transition-colors cursor-pointer"
          :title="t.terminal + ' (Ctrl+\\)'"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
          </svg>
        </button>
        <span class="text-zinc-600">UTF-8</span>
        <span class="text-zinc-600">Ln 1, Col 1</span>
        <span class="text-zinc-600">QuickText</span>
      </div>
    </div>

    <!-- Click outside handler for menus -->
    <div v-if="menuOpen || settingsOpen" class="fixed inset-0 z-40" @click="menuOpen = false; settingsOpen = false"></div>

    <!-- Close Confirmation Dialog -->
    <div v-if="showCloseDialog" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60">
      <div class="bg-[#1e1e22] border border-zinc-700 rounded-xl shadow-2xl w-96 p-6">
        <div class="flex items-center gap-3 mb-4">
          <div class="w-10 h-10 rounded-full bg-yellow-500/10 flex items-center justify-center shrink-0">
            <svg class="w-5 h-5 text-yellow-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"/>
            </svg>
          </div>
          <div>
            <h3 class="text-zinc-200 font-medium">{{ t.saveQuestion }}</h3>
            <p class="text-sm text-zinc-500">{{ t.saveMsg }}</p>
          </div>
        </div>
        <div class="flex gap-2 justify-end">
          <button
            @click="closeDialogCancel"
            class="px-4 py-2 text-sm rounded-lg bg-zinc-700 hover:bg-zinc-600 text-zinc-300 transition-colors"
          >
            {{ t.cancel }}
          </button>
          <button
            @click="closeDialogDiscard"
            class="px-4 py-2 text-sm rounded-lg bg-red-600/20 hover:bg-red-600/30 text-red-400 border border-red-600/30 transition-colors"
          >
            {{ t.discard }}
          </button>
          <button
            @click="closeDialogSave"
            class="px-4 py-2 text-sm rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors"
          >
            {{ t.saveAndExit }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
const FolderNode = {
  name: 'FolderNode',
  props: {
    node: Object,
    depth: { type: Number, default: 0 }
  },
  emits: ['open-file'],
  data() {
    return { expanded: false }
  },
  template: `
    <div>
      <div
        @click="node.isDir ? expanded = !expanded : $emit('open-file', node)"
        class="flex items-center gap-1 py-0.5 px-1 rounded cursor-pointer hover:bg-zinc-800 text-xs"
        :style="{ paddingLeft: (depth * 12 + 4) + 'px' }"
      >
        <svg v-if="node.isDir" class="w-3.5 h-3.5 shrink-0 transition-transform" :class="expanded ? 'rotate-90' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
        </svg>
        <span v-else class="w-3.5 shrink-0"></span>
        <svg v-if="node.isDir" class="w-3.5 h-3.5 shrink-0 text-yellow-600" fill="currentColor" viewBox="0 0 24 24">
          <path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
        </svg>
        <svg v-else class="w-3.5 h-3.5 shrink-0 text-zinc-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
        </svg>
        <span class="truncate text-zinc-400">{{ node.name }}</span>
      </div>
      <div v-if="node.isDir && expanded && node.children">
        <FolderNode
          v-for="child in node.children"
          :key="child.path"
          :node="child"
          :depth="depth + 1"
          @open-file="(n) => $emit('open-file', n)"
        />
      </div>
    </div>
  `
}

export default {
  name: 'App',
  components: { FolderNode }
}
</script>
