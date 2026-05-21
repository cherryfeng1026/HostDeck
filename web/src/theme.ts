import type { GlobalThemeOverrides } from 'naive-ui'
import type { ThemeMode } from './types'

const sharedThemeOverrides: GlobalThemeOverrides = {
  common: {
    borderRadius: '8px',
    borderRadiusSmall: '8px',
    fontFamily: '"Inter", "Public Sans", "Segoe UI", "PingFang SC", sans-serif',
    fontFamilyMono: '"Fira Code", "SFMono-Regular", "Consolas", monospace',
  },
  Button: {
    fontWeight: '600',
  },
  Card: {
    borderRadius: '8px',
  },
  Input: {
    borderRadius: '8px',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '8px',
      },
    },
  },
}

const darkThemeOverrides: GlobalThemeOverrides = {
  ...sharedThemeOverrides,
  common: {
    ...sharedThemeOverrides.common,
    primaryColor: '#4f83ff',
    primaryColorHover: '#6ba0ff',
    primaryColorPressed: '#2f65e8',
    primaryColorSuppl: '#4f83ff',
    infoColor: '#20d4ff',
    successColor: '#35d6a3',
    warningColor: '#e8b45f',
    errorColor: '#ff6b7d',
  },
  Layout: {
    color: '#08111f',
    siderColor: '#0b1424',
    headerColor: 'rgba(10, 19, 33, 0.94)',
  },
}

const lightThemeOverrides: GlobalThemeOverrides = {
  ...sharedThemeOverrides,
  common: {
    ...sharedThemeOverrides.common,
    primaryColor: '#c96a2d',
    primaryColorHover: '#da7c3e',
    primaryColorPressed: '#a54a1a',
    primaryColorSuppl: '#c96a2d',
    infoColor: '#0d9488',
    successColor: '#15956d',
    warningColor: '#ca8a04',
    errorColor: '#d75a55',
  },
  Layout: {
    color: '#f5efe6',
    siderColor: 'rgba(255, 249, 241, 0.94)',
    headerColor: 'rgba(255, 252, 247, 0.9)',
  },
}

export function getThemeOverrides(mode: ThemeMode): GlobalThemeOverrides {
  return mode === 'dark' ? darkThemeOverrides : lightThemeOverrides
}
