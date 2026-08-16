<script setup lang="ts">
// 哈希计算页：添加文件/文件夹 → 选算法 → 计算 → 表格结果 + 导出。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NCard, NEmpty, NIcon, NSelect, NTag, NTooltip, useMessage } from 'naive-ui'
import {
  DocumentOutline,
  DownloadOutline,
  FolderOpenOutline,
  PlayOutline,
  StopOutline,
  TrashOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import {
  algoLabel,
  consumeDrop,
  createTaskSession,
  dropPayload,
  errorText,
  exportCSV,
  exportSUM,
  pickFiles,
  pickFolder,
  pickSavePath,
  type Result,
} from '../api'
import { avgSpeedMBps, formatDuration, formatSpeed } from '../utils/format'
import { useSettingsStore } from '../stores/settings'
import AlgoChips from '../components/algo-chips.vue'
import ResultTable from '../components/result-table.vue'
import ProgressPanel from '../components/progress-panel.vue'

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const session = createTaskSession()
const { running, items, progress, summary, taskId } = session

const paths = ref<string[]>([])
const algos = ref<string[]>(['sha256'])
/** 本次任务实际使用的算法快照：任务完成后用户改 chips 不影响结果列与导出。 */
const usedAlgos = ref<string[]>([])
const sumAlgo = ref<string>('sha256')
const exporting = ref(false)

const canStart = computed(() => paths.value.length > 0 && algos.value.length > 0 && !running.value)

/** 结果表格的列算法：存在任务快照后用快照（行数据是按快照算法算出的），否则跟随当前选择。 */
const tableAlgos = computed(() => (usedAlgos.value.length ? usedAlgos.value : algos.value))

/** SUM 导出算法选项：取自任务快照；CRC32 摘要无法重新导入校验，不提供导出。 */
const sumOptions = computed(() =>
  usedAlgos.value.filter((a) => a !== 'crc32').map((a) => ({ label: algoLabel(a), value: a })),
)

watch(sumOptions, (opts) => {
  if (!opts.some((o) => o.value === sumAlgo.value)) sumAlgo.value = opts[0]?.value ?? ''
})

const avgSpeed = computed(() =>
  summary.value ? formatSpeed(avgSpeedMBps(summary.value.bytesDone, summary.value.elapsedMs)) : '',
)

function showError(r: Result) {
  message.error(errorText(r.error, settings.locale))
}

function addPaths(list: string[]) {
  const set = new Set(paths.value)
  for (const p of list) set.add(p)
  paths.value = [...set]
}

function removePath(p: string) {
  paths.value = paths.value.filter((x) => x !== p)
}

async function onPickFiles() {
  const r = await pickFiles()
  if (!r.ok) return showError(r)
  if (r.paths?.length) addPaths(r.paths)
}

async function onPickFolder() {
  const r = await pickFolder()
  if (!r.ok) return showError(r)
  if (r.path) addPaths([r.path])
}

async function start() {
  if (!paths.value.length) return message.warning(t('hash.needPaths'))
  if (!algos.value.length) return message.warning(t('hash.needAlgo'))
  usedAlgos.value = [...algos.value]
  const ok = await session.startHash(paths.value, algos.value)
  if (!ok && session.error.value) message.error(errorText(session.error.value, settings.locale))
}

async function onCancel() {
  await session.cancel()
}

async function doExportCsv() {
  const r = await pickSavePath('result.csv', 'CSV (*.csv)', '*.csv')
  if (!r.ok) return showError(r)
  if (!r.path) return // 用户取消
  exporting.value = true
  const er = await exportCSV(taskId.value, r.path, false)
  exporting.value = false
  if (!er.ok) return showError(er)
  message.success(t('hash.exported', { path: er.path ?? r.path }))
}

async function doExportSum() {
  const algo = sumOptions.value.length === 1 ? sumOptions.value[0].value : sumAlgo.value
  if (!algo) return
  const r = await pickSavePath(`checksums.${algo}`, `SUM (*.${algo})`, `*.${algo}`)
  if (!r.ok) return showError(r)
  if (!r.path) return
  exporting.value = true
  const er = await exportSUM(taskId.value, r.path, algo)
  exporting.value = false
  if (!er.ok) return showError(er)
  message.success(t('hash.exported', { path: er.path ?? r.path }))
}

// 拖拽消费：drop 发生时若已在当前页由 watch 触发，否则挂载时消费一次。
function tryConsumeDrop() {
  const dropped = consumeDrop('files')
  if (dropped?.length) addPaths(dropped)
}
onMounted(tryConsumeDrop)
watch(dropPayload, tryConsumeDrop)

onBeforeUnmount(() => {
  if (running.value) void session.cancel()
  session.destroy()
})
</script>

<template>
  <div class="view">
    <n-card size="small" :bordered="true" class="control-card">
      <div class="control-row">
        <n-button :disabled="running" @click="onPickFiles">
          <template #icon><n-icon><DocumentOutline /></n-icon></template>
          {{ t('hash.pickFiles') }}
        </n-button>
        <n-button :disabled="running" @click="onPickFolder">
          <template #icon><n-icon><FolderOpenOutline /></n-icon></template>
          {{ t('hash.pickFolder') }}
        </n-button>
        <n-button
          v-if="paths.length"
          text
          :disabled="running"
          class="clear-btn"
          @click="paths = []"
        >
          <template #icon><n-icon><TrashOutline /></n-icon></template>
          {{ t('common.clear') }}
        </n-button>
        <span class="toolbar-spacer" />
        <algo-chips v-model="algos" :disabled="running" />
        <n-button v-if="!running" type="primary" :disabled="!canStart" @click="start">
          <template #icon><n-icon><PlayOutline /></n-icon></template>
          {{ t('common.start') }}
        </n-button>
        <n-button v-else type="error" tertiary @click="onCancel">
          <template #icon><n-icon><StopOutline /></n-icon></template>
          {{ t('common.cancel') }}
        </n-button>
      </div>

      <div v-if="paths.length" class="path-list">
        <span class="path-list-label">{{ t('hash.pendingList') }} · {{ paths.length }}</span>
        <n-tag
          v-for="p in paths"
          :key="p"
          size="small"
          :closable="!running"
          :title="p"
          class="path-chip"
          @close="removePath(p)"
        >
          {{ p }}
        </n-tag>
      </div>
    </n-card>

    <progress-panel v-if="running" :progress="progress" :running="running" @cancel="onCancel" />

    <template v-if="summary">
      <div class="summary-bar">
        <div class="stat-card">
          <span class="stat-label">{{ t('common.total') }}</span>
          <span class="stat-value">{{ summary.total }}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">{{ t('common.ok') }}</span>
          <span class="stat-value ok">{{ summary.ok }}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">{{ t('common.failed') }}</span>
          <span class="stat-value" :class="{ err: summary.errors > 0 }">{{ summary.errors }}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">{{ t('common.elapsed') }}</span>
          <span class="stat-value">{{ formatDuration(summary.elapsedMs) }}</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">{{ t('common.avgSpeed') }}</span>
          <span class="stat-value info">{{ avgSpeed }}</span>
        </div>
        <div v-if="summary.canceled" class="stat-card">
          <span class="stat-label">&nbsp;</span>
          <n-tag type="warning" size="small" :bordered="false">{{ t('common.canceled') }}</n-tag>
        </div>
        <div v-if="summary.fatal" class="stat-card">
          <span class="stat-label">&nbsp;</span>
          <n-tag type="error" size="small" :bordered="false">{{ t('common.taskError') }}</n-tag>
        </div>
      </div>

      <div class="export-bar">
        <n-button size="small" :loading="exporting" @click="doExportCsv">
          <template #icon><n-icon><DownloadOutline /></n-icon></template>
          {{ t('hash.exportCsv') }}
        </n-button>
        <template v-if="sumOptions.length">
          <n-select
            v-if="sumOptions.length > 1"
            v-model:value="sumAlgo"
            :options="sumOptions"
            size="small"
            class="sum-select"
            :consistent-menu-width="false"
          />
          <n-button size="small" :loading="exporting" @click="doExportSum">
            <template #icon><n-icon><DownloadOutline /></n-icon></template>
            {{ t('hash.exportSum') }}
          </n-button>
        </template>
        <n-tooltip v-else trigger="hover">
          <template #trigger>
            <span class="disabled-btn-wrap">
              <n-button size="small" disabled>{{ t('hash.exportSum') }}</n-button>
            </span>
          </template>
          {{ t('hash.sumNoCrc32') }}
        </n-tooltip>
      </div>
    </template>

    <result-table v-if="items.length" :items="items" :algos="tableAlgos" mode="hash" />

    <div v-else-if="!running && !summary" class="empty-wrap">
      <n-empty :description="t('hash.empty')">
        <template #extra>
          <span class="empty-desc">{{ t('hash.emptyDesc') }}</span>
        </template>
      </n-empty>
    </div>
  </div>
</template>

<style scoped>
.control-card {
  flex: 0 0 auto;
}

.control-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.clear-btn {
  color: var(--text-secondary);
}

.path-list {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 10px;
  max-height: 84px;
  overflow-y: auto;
}

.path-list-label {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.path-chip {
  max-width: 320px;
}

.path-chip :deep(.n-tag__content) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.export-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.sum-select {
  width: 130px;
}

/* disabled 按钮不触发 hover，外包一层 span 供 tooltip 挂接 */
.disabled-btn-wrap {
  display: inline-flex;
  cursor: not-allowed;
}

.empty-wrap {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-desc {
  font-size: 12px;
  color: var(--text-secondary);
}
</style>
