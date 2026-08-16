<script setup lang="ts">
// 布局壳：侧边栏导航 + 顶栏（语言/主题切换）+ 工作区 router-view。
// 根元素即整窗拖拽目标（--wails-drop-target: drop）。
import { computed, onBeforeUnmount, onMounted } from 'vue'
import {
  darkTheme,
  dateEnUS,
  dateZhCN,
  enUS,
  zhCN,
  NConfigProvider,
  NIcon,
  NMessageProvider,
  NTooltip,
  type GlobalThemeOverrides,
} from 'naive-ui'
import {
  AlbumsOutline,
  DesktopOutline,
  FingerPrintOutline,
  GitCompareOutline,
  MoonOutline,
  ShieldCheckmarkOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { OnFileDrop, OnFileDropOff } from '../wailsjs/runtime/runtime'
import { dispatchDrop } from './api'
import { router } from './router'
import { useSettingsStore } from './stores/settings'

const settings = useSettingsStore()
const { t } = useI18n()

const naiveTheme = computed(() => (settings.isDark ? darkTheme : null))
const naiveLocale = computed(() => (settings.locale === 'zh-CN' ? zhCN : enUS))
const naiveDateLocale = computed(() => (settings.locale === 'zh-CN' ? dateZhCN : dateEnUS))

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#2f7bff',
    primaryColorHover: '#4d90ff',
    primaryColorPressed: '#2264d8',
    primaryColorSuppl: '#4d90ff',
    borderRadius: '6px',
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif",
    fontFamilyMono: "Consolas, 'JetBrains Mono', 'Courier New', monospace",
  },
}

const navItems = computed(() => [
  { path: '/', icon: FingerPrintOutline, label: t('nav.hash') },
  { path: '/verify', icon: ShieldCheckmarkOutline, label: t('nav.verify') },
  { path: '/compare', icon: GitCompareOutline, label: t('nav.compare') },
  { path: '/batch', icon: AlbumsOutline, label: t('nav.batch') },
])

const themeIcon = computed(() => {
  if (settings.themeMode === 'light') return SunnyOutline
  if (settings.themeMode === 'dark') return MoonOutline
  return DesktopOutline
})

const themeLabel = computed(() => {
  if (settings.themeMode === 'light') return t('topbar.themeLight')
  if (settings.themeMode === 'dark') return t('topbar.themeDark')
  return t('topbar.themeSystem')
})

const langLabel = computed(() => (settings.locale === 'zh-CN' ? 'EN' : '中'))

// 整窗拖拽：恰一个清单文件 → 批量校验（自动开始）；否则 → 哈希计算。
function onDrop(_x: number, _y: number, paths: string[]) {
  if (!paths || paths.length === 0) return
  const target = dispatchDrop(paths)
  if (router.currentRoute.value.path !== target) void router.push(target)
}

onMounted(() => OnFileDrop(onDrop, true))
onBeforeUnmount(() => OnFileDropOff())
</script>

<template>
  <n-config-provider
    :theme="naiveTheme"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
    :theme-overrides="themeOverrides"
  >
    <n-message-provider placement="bottom-right">
      <div id="app-root" :class="{ dark: settings.isDark }" style="--wails-drop-target: drop">
        <aside class="sidebar">
          <div class="sidebar-logo">H</div>
          <button
            v-for="item in navItems"
            :key="item.path"
            class="nav-item"
            :class="{ active: router.currentRoute.value.path === item.path }"
            @click="router.push(item.path)"
          >
            <n-icon :size="20"><component :is="item.icon" /></n-icon>
            <span>{{ item.label }}</span>
          </button>
        </aside>

        <div class="main">
          <header class="topbar">
            <div class="topbar-title">{{ t('app.title') }}</div>
            <div class="topbar-actions">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <button class="icon-btn" @click="settings.toggleLocale()">{{ langLabel }}</button>
                </template>
                {{ t('topbar.switchLang') }}
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <button class="icon-btn" @click="settings.cycleTheme()">
                    <n-icon :size="17"><component :is="themeIcon" /></n-icon>
                  </button>
                </template>
                {{ themeLabel }}
              </n-tooltip>
            </div>
          </header>

          <main class="workspace">
            <router-view v-slot="{ Component }">
              <transition name="fade" mode="out-in">
                <component :is="Component" />
              </transition>
            </router-view>
          </main>
        </div>
      </div>
    </n-message-provider>
  </n-config-provider>
</template>
