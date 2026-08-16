// 应用设置：主题（亮/暗/跟随系统）与语言，均持久化到 localStorage。
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { i18n, LOCALE_KEY, type AppLocale } from '../locales'

export type ThemeMode = 'light' | 'dark' | 'system'

const THEME_KEY = 'gohash.theme'

function loadThemeMode(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    // localStorage 不可用时使用默认
  }
  return 'system'
}

const mediaQuery: MediaQueryList | null =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)')
    : null

export const useSettingsStore = defineStore('settings', () => {
  const themeMode = ref<ThemeMode>(loadThemeMode())
  const systemDark = ref<boolean>(mediaQuery ? mediaQuery.matches : false)

  if (mediaQuery) {
    mediaQuery.addEventListener('change', (e: MediaQueryListEvent) => {
      systemDark.value = e.matches
    })
  }

  const isDark = computed(() =>
    themeMode.value === 'system' ? systemDark.value : themeMode.value === 'dark',
  )

  const locale = computed<AppLocale>(() => i18n.global.locale.value as AppLocale)

  function setThemeMode(mode: ThemeMode) {
    themeMode.value = mode
    try {
      localStorage.setItem(THEME_KEY, mode)
    } catch {
      // 忽略持久化失败
    }
  }

  /** 亮 → 暗 → 跟随系统 循环切换。 */
  function cycleTheme() {
    const order: ThemeMode[] = ['light', 'dark', 'system']
    const idx = order.indexOf(themeMode.value)
    setThemeMode(order[(idx + 1) % order.length])
  }

  function setLocale(l: AppLocale) {
    i18n.global.locale.value = l
    try {
      localStorage.setItem(LOCALE_KEY, l)
    } catch {
      // 忽略持久化失败
    }
  }

  function toggleLocale() {
    setLocale(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
  }

  return { themeMode, systemDark, isDark, locale, setThemeMode, cycleTheme, setLocale, toggleLocale }
})
