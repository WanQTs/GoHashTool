<script setup lang="ts">
// 设置入口（顶栏齿轮弹层）：文件关联显式开关（默认关）。
// 注册/解除只写/删当前用户 HKCU 下本应用自有 ProgID（免管理员），
// 被其他程序占用的扩展名不劫持；exe 移动后由启动时的自愈更新打开命令。
import { onMounted, ref } from 'vue'
import { NCheckbox, NIcon, NPopover, NTooltip, useMessage } from 'naive-ui'
import { SettingsOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import {
  errorText,
  getFileAssocStatus,
  registerFileAssociations,
  unregisterFileAssociations,
} from '../api'
import { useSettingsStore } from '../stores/settings'

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const assocOn = ref(false)
const busy = ref(false)

onMounted(async () => {
  const r = await getFileAssocStatus()
  if (r.ok) assocOn.value = (r.count ?? 0) > 0
})

/** 开关切换：调用对应绑定，失败时回滚勾选状态并 toast 错误。 */
async function onToggle(on: boolean) {
  busy.value = true
  const r = on ? await registerFileAssociations() : await unregisterFileAssociations()
  busy.value = false
  if (!r.ok) {
    assocOn.value = !on
    message.error(errorText(r.error, settings.locale))
    return
  }
  message.success(t(on ? 'assoc.registered' : 'assoc.unregistered'))
}
</script>

<template>
  <n-popover trigger="click" placement="bottom-end" :show-arrow="false">
    <template #trigger>
      <n-tooltip trigger="hover" :disabled="busy">
        <template #trigger>
          <button class="icon-btn" aria-label="settings">
            <n-icon :size="17"><SettingsOutline /></n-icon>
          </button>
        </template>
        {{ t('topbar.settings') }}
      </n-tooltip>
    </template>
    <div class="settings-panel">
      <div class="settings-title">{{ t('assoc.title') }}</div>
      <n-checkbox v-model:checked="assocOn" :disabled="busy" @update:checked="onToggle">
        {{ t('assoc.enable') }}
      </n-checkbox>
      <div class="settings-desc">{{ t('assoc.desc') }}</div>
    </div>
  </n-popover>
</template>

<style scoped>
.settings-panel {
  max-width: 280px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.settings-title {
  font-weight: 600;
  font-size: 13px;
}

.settings-desc {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}
</style>
