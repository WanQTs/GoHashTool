// vue-i18n 初始化与默认语言检测。
import { createI18n } from 'vue-i18n'
import zhCN from './zh-cn.json'
import enUS from './en-us.json'

export type AppLocale = 'zh-CN' | 'en-US'
export const LOCALE_KEY = 'gohash.locale'

/** 默认语言：持久化优先；系统语言为中文/英文则跟随，无法判断时按需求规格回退中文。 */
export function detectLocale(): AppLocale {
  try {
    const saved = localStorage.getItem(LOCALE_KEY)
    if (saved === 'zh-CN' || saved === 'en-US') return saved
  } catch {
    // localStorage 不可用时按浏览器语言处理
  }
  const nav = (typeof navigator !== 'undefined' ? navigator.language : '') || ''
  const l = nav.toLowerCase()
  if (l.startsWith('zh')) return 'zh-CN'
  if (l.startsWith('en')) return 'en-US'
  return 'zh-CN' // 无法判断（空值或中英之外的语言）时用中文
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})
