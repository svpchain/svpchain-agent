<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { setLocale } from './i18n'
import { useAppTheme } from './composables/useAppTheme'
import * as App from '../wailsjs/go/desktop/App'
import { WindowToggleMaximise } from '../wailsjs/runtime/runtime'
import AssistantTab from './tabs/AssistantTab.vue'
import ConfigTab from './tabs/ConfigTab.vue'
import KeysTab from './tabs/KeysTab.vue'
import SecurityTab from './tabs/SecurityTab.vue'
import SettingsTab from './tabs/SettingsTab.vue'
import AboutTab from './tabs/AboutTab.vue'
import UpdateModals from './UpdateModals.vue'
import type { Entry } from './types'

const { t } = useI18n()
const { isDark, sidebarCollapsed, toggleTheme, toggleSidebar } = useAppTheme()

const isMac = /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent)

type TabId = 'assistant' | 'config' | 'keys' | 'security' | 'settings' | 'about'

const navItems: { id: TabId; labelKey: string; icon: string }[] = [
  { id: 'assistant', labelKey: 'tab.assistant', icon: 'chat' },
  { id: 'config', labelKey: 'tab.config', icon: 'config' },
  { id: 'keys', labelKey: 'tab.keys', icon: 'key' },
  { id: 'security', labelKey: 'tab.security', icon: 'shield' },
  { id: 'settings', labelKey: 'tab.settings', icon: 'settings' },
  { id: 'about', labelKey: 'tab.about', icon: 'info' },
]

const activeTab = ref<TabId>('assistant')
const status = ref('')
const entries = ref<Entry[]>([])
const defaultChainIds = ref<string[]>([])

const updateModalsRef = ref<InstanceType<typeof UpdateModals> | null>(null)

function setStatus(msg: string) {
  status.value = msg
}

watch(activeTab, () => {
  status.value = ''
})

function toggleMaximise() {
  WindowToggleMaximise()
}

onMounted(async () => {
  const lang = await App.Language()
  setLocale(lang)
  defaultChainIds.value = (await App.DefaultChainIDs()) || []
  await updateModalsRef.value?.maybeCheckUpdate()
})
</script>

<template>
  <div class="app-shell">
    <div
      class="titlebar-drag titlebar"
      :class="{ 'titlebar--mac': isMac }"
      @dblclick="toggleMaximise"
    >
      <span class="titlebar-brand">
        <span class="brand-mark">S</span>
        <span v-show="!sidebarCollapsed" class="titlebar-text">SVPChain Agent</span>
      </span>
    </div>

    <div class="app-body">
      <aside class="sidebar" :class="{ 'sidebar--collapsed': sidebarCollapsed }">
        <nav class="sidebar-nav">
          <button
            v-for="item in navItems"
            :key="item.id"
            type="button"
            class="nav-item"
            :class="{ 'nav-item--active': activeTab === item.id }"
            :title="sidebarCollapsed ? t(item.labelKey) : undefined"
            @click="activeTab = item.id"
          >
            <span class="nav-icon" aria-hidden="true">
              <svg v-if="item.icon === 'chat'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <path d="M12 3c5.523 0 10 3.582 10 8 0 2.4-1.2 4.56-3.12 6.08L21 21l-4.28-1.42C15.56 20.18 13.82 20.5 12 20.5 6.477 20.5 2 16.918 2 11.5S6.477 3 12 3z" stroke-linejoin="round" />
              </svg>
              <svg v-else-if="item.icon === 'config'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <path d="M10 13a2 2 0 1 0 4 0 2 2 0 0 0-4 0z" />
                <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4l1.4-1.4M17 7l1.4-1.4" stroke-linecap="round" />
              </svg>
              <svg v-else-if="item.icon === 'key'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <circle cx="8" cy="15" r="4" />
                <path d="M11.5 11.5L21 2M16 2h5v5" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              <svg v-else-if="item.icon === 'shield'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <path d="M12 2l8 4v6c0 5.25-3.5 9.74-8 10-4.5-.26-8-4.75-8-10V6l8-4z" stroke-linejoin="round" />
              </svg>
              <svg v-else-if="item.icon === 'settings'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" stroke-linejoin="round" />
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
                <circle cx="12" cy="12" r="9" />
                <path d="M12 10v6M12 7h.01" stroke-linecap="round" />
              </svg>
            </span>
            <span v-show="!sidebarCollapsed" class="nav-label">{{ t(item.labelKey) }}</span>
          </button>
        </nav>

        <div class="sidebar-footer">
          <button
            type="button"
            class="footer-btn"
            :title="isDark ? t('shell.themeLight') : t('shell.themeDark')"
            @click="toggleTheme"
          >
            <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
              <circle cx="12" cy="12" r="4" />
              <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" stroke-linecap="round" />
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
              <path d="M21 14.5A8.5 8.5 0 0 1 9.5 3 7 7 0 1 0 21 14.5z" stroke-linejoin="round" />
            </svg>
            <span v-show="!sidebarCollapsed" class="footer-label">
              {{ isDark ? t('shell.themeLight') : t('shell.themeDark') }}
            </span>
          </button>
          <button
            type="button"
            class="footer-btn"
            :title="sidebarCollapsed ? t('shell.expandSidebar') : t('shell.collapseSidebar')"
            @click="toggleSidebar"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75">
              <path v-if="sidebarCollapsed" d="M9 6l6 6-6 6" stroke-linecap="round" stroke-linejoin="round" />
              <path v-else d="M15 6l-6 6 6 6" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
            <span v-show="!sidebarCollapsed" class="footer-label">
              {{ t('shell.collapseSidebar') }}
            </span>
          </button>
        </div>
      </aside>

      <main class="main-content">
        <div v-show="activeTab === 'assistant'" class="tab-panel tab-panel--assistant">
          <AssistantTab :entries="entries" @status="setStatus" />
        </div>
        <div v-show="activeTab === 'config'" class="tab-panel tab-panel--scroll">
          <ConfigTab :entries="entries" @status="setStatus" />
        </div>
        <div v-show="activeTab === 'keys'" class="tab-panel tab-panel--scroll">
          <KeysTab
            :entries="entries"
            :default-chain-ids="defaultChainIds"
            @status="setStatus"
            @update:entries="entries = $event"
          />
        </div>
        <div v-show="activeTab === 'security'" class="tab-panel tab-panel--scroll">
          <SecurityTab :default-chain-ids="defaultChainIds" @status="setStatus" />
        </div>
        <div v-show="activeTab === 'settings'" class="tab-panel tab-panel--scroll">
          <SettingsTab :entries="entries" @status="setStatus" />
        </div>
        <div v-show="activeTab === 'about'" class="tab-panel tab-panel--scroll">
          <AboutTab />
        </div>
      </main>
    </div>

    <div class="statusbar">
      <span v-if="status" class="status-dot" :class="{ 'status-dot--active': !!status }" />
      <span class="status-text">{{ status }}</span>
    </div>

    <UpdateModals ref="updateModalsRef" />
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bg-base);
}

.titlebar {
  flex-shrink: 0;
  height: var(--titlebar-height);
  display: flex;
  align-items: center;
  justify-content: center;
  border-bottom: 1px solid var(--border-subtle);
  user-select: none;
  box-sizing: border-box;
  background: var(--bg-surface);
}

.titlebar--mac {
  padding-left: 78px;
}

.titlebar-brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  letter-spacing: -0.01em;
}

.titlebar-text {
  transition: opacity 0.2s ease;
}

.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  background: linear-gradient(135deg, var(--accent) 0%, #0d8c6d 100%);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  flex-shrink: 0;
}

.app-body {
  flex: 1;
  min-height: 0;
  display: flex;
}

.sidebar {
  flex-shrink: 0;
  width: var(--sidebar-width);
  background: var(--bg-surface);
  border-right: 1px solid var(--border-subtle);
  padding: 12px 10px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
  overflow: hidden;
}

.sidebar--collapsed {
  width: var(--sidebar-width-collapsed);
  padding-left: 8px;
  padding-right: 8px;
}

.sidebar-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-height: 0;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 12px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 13px;
  font-weight: 500;
  text-align: left;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.sidebar--collapsed .nav-item {
  justify-content: center;
  padding: 10px 8px;
}

.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.nav-item--active {
  background: var(--bg-elevated);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}

.nav-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  opacity: 0.85;
}

.nav-icon svg {
  width: 18px;
  height: 18px;
}

.nav-item--active .nav-icon {
  color: var(--accent);
  opacity: 1;
}

.nav-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-footer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
  margin-top: 8px;
}

.footer-btn {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 12px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-family: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.sidebar--collapsed .footer-btn {
  justify-content: center;
  padding: 9px 8px;
}

.footer-btn:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
}

.footer-btn svg {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.footer-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-content {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-base);
}

.tab-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 20px 24px;
  overflow: hidden;
}

.tab-panel--scroll {
  overflow-y: auto;
  overflow-x: hidden;
}

.tab-panel--assistant {
  padding: 0;
}

.statusbar {
  flex-shrink: 0;
  height: var(--statusbar-height);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  font-size: 11px;
  color: var(--text-muted);
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-surface);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  flex-shrink: 0;
  opacity: 0.4;
}

.status-dot--active {
  background: var(--accent);
  opacity: 1;
  box-shadow: 0 0 6px rgba(16, 163, 127, 0.6);
}

.status-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
