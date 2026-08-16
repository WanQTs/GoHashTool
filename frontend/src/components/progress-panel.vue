<script setup lang="ts">
// 任务运行中的进度面板：总进度条（按字节）+ 当前文件 + 速度 + 已用时间 + 取消。
// 速度为后端 200ms 窗口瞬时值，前端做 EMA 平滑避免显示抖动。
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NProgress } from 'naive-ui'
import { StopOutline } from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import type { ProgressEvent } from '../api'
import { baseName, formatBytes, formatDuration, formatSpeed } from '../utils/format'

const props = defineProps<{
  progress: ProgressEvent | null
  running: boolean
}>()

const emit = defineEmits<{
  cancel: []
}>()

const { t } = useI18n()

const percent = computed(() => {
  const p = props.progress
  if (!p) return 0
  if (p.totalBytes > 0) return Math.min(100, (p.bytesDone / p.totalBytes) * 100)
  if (p.total > 0) return Math.min(100, (p.done / p.total) * 100)
  return 0
})

const currentName = computed(() => {
  const f = props.progress?.currentFile
  return f ? baseName(f) : t('progress.noFile')
})

// EMA 平滑速度（α=0.3）；任务重新开始时归零。
const smoothSpeed = ref(0)
watch(
  () => props.progress?.speedMBps,
  (v) => {
    const cur = v ?? 0
    smoothSpeed.value = smoothSpeed.value <= 0 ? cur : smoothSpeed.value * 0.7 + cur * 0.3
  },
)
watch(
  () => props.running,
  (r) => {
    if (r) smoothSpeed.value = 0
  },
)
</script>

<template>
  <div class="progress-panel">
    <n-progress
      type="line"
      :percentage="Number(percent.toFixed(1))"
      :height="8"
      :border-radius="4"
      :show-indicator="true"
      processing
    />
    <div class="progress-meta">
      <span class="progress-file mono" :title="progress?.currentFile">{{ currentName }}</span>
      <span class="progress-stat">{{ t('progress.items') }} {{ progress?.done ?? 0 }}/{{ progress?.total ?? 0 }}</span>
      <span v-if="(progress?.totalBytes ?? 0) > 0" class="progress-stat">
        {{ formatBytes(progress?.bytesDone ?? 0) }} / {{ formatBytes(progress?.totalBytes ?? 0) }}
      </span>
      <span class="progress-stat">{{ formatSpeed(smoothSpeed) }}</span>
      <span class="progress-stat">{{ formatDuration(progress?.elapsedMs ?? 0) }}</span>
      <span class="toolbar-spacer" />
      <n-button size="tiny" tertiary type="error" :disabled="!running" @click="emit('cancel')">
        <template #icon><n-icon><StopOutline /></n-icon></template>
        {{ t('common.cancel') }}
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.progress-panel {
  padding: 12px 14px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background-color: var(--panel-bg);
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease;
}

.progress-meta {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary);
  flex-wrap: wrap;
}

.progress-file {
  min-width: 0;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}

.progress-stat {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
</style>
