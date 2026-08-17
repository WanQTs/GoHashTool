<script setup lang="ts">
// 布局壳：侧边栏导航 + 顶栏（语言/主题切换）+ 工作区 router-view。
// 整窗拖拽：index.html 的 <body data-file-drop-target> 为落点；
// Go 侧接收系统拖拽事件并广播 'files-dropped'（载荷为绝对路径数组）。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
  PinOutline,
  ShieldCheckmarkOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { consumePendingOpenFile, dispatchDrop, setAlwaysOnTop, setupResultContextMenu } from './api'
import { router } from './router'
import { useSettingsStore } from './stores/settings'
import SettingsMenu from './components/settings-menu.vue'

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

// 拖拽与「打开方式」（文件关联/单实例转交）共用路由：
// 恰一个清单文件 → 批量校验（自动开始）；否则 → 哈希计算。
function routePaths(paths: string[]) {
  if (!paths || paths.length === 0) return
  const target = dispatchDrop(paths)
  if (router.currentRoute.value.path !== target) void router.push(target)
}

let offDrop: (() => void) | null = null
let offOpen: (() => void) | null = null

onMounted(async () => {
  offDrop = Events.On('files-dropped', (ev) => routePaths(ev.data as string[]))
  // 运行中双击清单文件：单实例把二实例参数转交为首实例的 open-with-file 事件
  offOpen = Events.On('open-with-file', (ev) => {
    const p = ev.data as string
    if (p) routePaths([p])
  })
  // 文件关联冷启动带入的清单：前端就绪前事件可能已发出，挂载后拉取兜底
  const r = await consumePendingOpenFile()
  if (r.ok && r.path) routePaths([r.path])
})
onBeforeUnmount(() => {
  offDrop?.()
  offOpen?.()
})

// 窗口置顶（图钉）：会话级开关，不持久化
const pinned = ref(false)
async function togglePin() {
  const next = !pinned.value
  const r = await setAlwaysOnTop(next)
  if (r.ok) pinned.value = next
}

// 结果行原生右键菜单：启动时注册一次；切换语言后按新文案重建
// （原生菜单文案由 Go 侧持有，无法随语言包热更新，只能重建）
function syncContextMenu() {
  void setupResultContextMenu({
    copyHash: t('ctx.copyHash'),
    copyPath: t('ctx.copyPath'),
    reveal: t('ctx.reveal'),
  })
}
onMounted(syncContextMenu)
watch(() => settings.locale, syncContextMenu)
</script>

<template>
  <n-config-provider
    :theme="naiveTheme"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
    :theme-overrides="themeOverrides"
  >
    <n-message-provider placement="bottom-right">
      <div id="app-root" :class="{ dark: settings.isDark }">
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
                  <button class="icon-btn" :class="{ 'is-on': pinned }" @click="togglePin()">
                    <n-icon :size="17"><component :is="PinOutline" /></n-icon>
                  </button>
                </template>
                {{ pinned ? t('topbar.unpin') : t('topbar.pin') }}
              </n-tooltip>
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
              <settings-menu />
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
