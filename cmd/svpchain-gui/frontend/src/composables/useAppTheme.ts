import { computed, ref, watch } from 'vue'

export type ThemeMode = 'light' | 'dark'

const THEME_KEY = 'svpchain-gui-theme'
const SIDEBAR_KEY = 'svpchain-gui-sidebar-collapsed'

function loadTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* private mode */
  }
  return 'light'
}

function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_KEY) === '1'
  } catch {
    return false
  }
}

const mode = ref<ThemeMode>(loadTheme())
const sidebarCollapsed = ref(loadSidebarCollapsed())

watch(
  mode,
  (v) => {
    document.documentElement.setAttribute('data-theme', v)
    document.documentElement.style.colorScheme = v
    try {
      localStorage.setItem(THEME_KEY, v)
    } catch {
      /* ignore */
    }
  },
  { immediate: true },
)

watch(sidebarCollapsed, (v) => {
  try {
    localStorage.setItem(SIDEBAR_KEY, v ? '1' : '0')
  } catch {
    /* ignore */
  }
})

export function useAppTheme() {
  const isDark = computed(() => mode.value === 'dark')

  function toggleTheme() {
    mode.value = mode.value === 'dark' ? 'light' : 'dark'
  }

  function setTheme(next: ThemeMode) {
    mode.value = next
  }

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  return {
    mode,
    isDark,
    sidebarCollapsed,
    toggleTheme,
    setTheme,
    toggleSidebar,
  }
}
