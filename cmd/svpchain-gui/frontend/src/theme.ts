import type { GlobalThemeOverrides } from 'naive-ui'
import type { ThemeMode } from './composables/useAppTheme'

const shared = {
  primaryColor: '#10a37f',
  primaryColorHover: '#1a7f64',
  primaryColorPressed: '#0d8c6d',
  primaryColorSuppl: '#10a37f',
  borderRadius: '10px',
  borderRadiusSmall: '8px',
  fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  fontFamilyMono: 'ui-monospace, SFMono-Regular, Menlo, monospace',
}

export function themeOverridesFor(mode: ThemeMode): GlobalThemeOverrides {
  const dark = mode === 'dark'
  return {
    common: {
      ...shared,
      bodyColor: dark ? '#0d0d0d' : '#ffffff',
      cardColor: dark ? '#212121' : '#ffffff',
      modalColor: dark ? '#212121' : '#ffffff',
      popoverColor: dark ? '#2a2a2a' : '#ffffff',
      tableColor: dark ? '#212121' : '#ffffff',
      inputColor: dark ? '#303030' : '#ffffff',
      hoverColor: dark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.04)',
      borderColor: dark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
      dividerColor: dark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
      textColor1: dark ? '#ececec' : '#0d0d0d',
      textColor2: dark ? '#a1a1a1' : '#5d5d5d',
      textColor3: dark ? '#6b6b6b' : '#8e8e8e',
    },
    Button: {
      borderRadiusMedium: '10px',
      borderRadiusSmall: '8px',
    },
    Input: {
      borderRadius: '12px',
      color: dark ? '#303030' : '#ffffff',
      colorFocus: dark ? '#303030' : '#ffffff',
    },
    Select: {
      peers: {
        InternalSelection: {
          borderRadius: '12px',
          color: dark ? '#303030' : '#ffffff',
        },
      },
    },
    DataTable: {
      borderRadius: '12px',
      thColor: dark ? '#1a1a1a' : '#f7f7f8',
      tdColor: dark ? '#212121' : '#ffffff',
    },
    Card: { borderRadius: '14px', color: dark ? '#212121' : '#ffffff' },
    Tag: { borderRadius: '8px' },
    Collapse: {
      titleTextColor: dark ? '#ececec' : '#0d0d0d',
      dividerColor: dark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
    },
    Tabs: {
      tabTextColorLine: dark ? '#6b6b6b' : '#8e8e8e',
      tabTextColorActiveLine: dark ? '#ececec' : '#0d0d0d',
      tabTextColorHoverLine: dark ? '#a1a1a1' : '#5d5d5d',
      barColor: '#10a37f',
    },
    Divider: {
      color: dark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.08)',
      textColor: dark ? '#6b6b6b' : '#8e8e8e',
    },
  }
}
