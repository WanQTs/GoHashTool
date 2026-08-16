<script setup lang="ts">
// 批量校验页：清单文件 + 可选自定义基准目录 → 校验 → 统计 + 结果表格 + 导出。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NCard, NEmpty, NIcon, NTag, useMessage } from 'naive-ui'
import {
  DocumentTextOutline,
  DownloadOutline,
  FolderOpenOutline,
  PlayOutline,
  StopOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import {
  algoLabel,
  consumeDrop,
  createTaskSession,
  dropPayload,
  errorText,
  exportCSV,
  pickFolder,
  pickManifestFile,
  pickSavePath,
  type Result,
} from '../api'
import { avgSpeedMBps, formatDuration, formatSpeed } from '../utils/format'
import { useSettingsStore } from '../stores/settings'
import ResultTable from '../components/result-table.vue'
import ProgressPanel from '../components/progress-panel.vue'

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const session = createTaskSession()
const { running, items, progress, summary, taskId, algo } = session

const manifestPath = ref('')
const baseDir = ref('')
const exporting = ref(false)

const canStart = computed(() => !!manifestPath.value && !running.value)

/** 顶部统计：由明细行实时累计（任务结束时全部行已冲刷完毕，与服务端汇总一致）。
 *  结论四分类：pass / fail / missing / error（存在但不可读，与缺失区分）。 */
const stats = computed(() => {
  let pass = 0
  let fail = 0
  let missing = 0
  let unreadable = 0
  for (const it of items.value) {
    if (it.verdict === 'pass') pass++
    else if (it.verdict === 'fail') fail++
    else if (it.verdict === 'missing') missing++
    else if (it.verdict === 'error') unreadable++
  }
  return {
    pass,
    fail,
    missing,
    error: unreadable,
    total: summary.value?.total ?? progress.value?.total ?? items.value.length,
  }
})

const avgSpeed = computed(() =>
  summary.value ? formatSpeed(avgSpeedMBps(summary.value.bytesDone, summary.value.elapsedMs)) : '',
)

const tableAlgos = computed(() => (algo.value ? [algo.value] : []))

function showError(r: Result) {
  message.error(errorText(r.error, settings.locale))
}

async function onPickManifest() {
  const r = await pickManifestFile()
  if (!r.ok) return showError(r)
  if (r.path) manifestPath.value = r.path
}

async function onPickBaseDir() {
  const r = await pickFolder()
  if (!r.ok) return showError(r)
  if (r.path) baseDir.value = r.path
}

async function start() {
  if (!manifestPath.value) return message.warning(t('batch.needManifest'))
  const ok = await session.startVerify(manifestPath.value, baseDir.value)
  if (!ok && session.error.value) message.error(errorText(session.error.value, settings.locale))
}

async function doExport(onlyFailed: boolean) {
  const defaultName = onlyFailed ? 'mismatches.csv' : 'verify-result.csv'
  const r = await pickSavePath(defaultName, 'CSV (*.csv)', '*.csv')
  if (!r.ok) return showError(r)
  if (!r.path) return
  exporting.value = true
  const er = await exportCSV(taskId.value, r.path, onlyFailed)
  exporting.value = false
  if (!er.ok) return showError(er)
  message.success(t('hash.exported', { path: er.path ?? r.path }))
}

// 拖入清单文件 → 自动开始校验；运行中拖入则先取消当前任务再启动（事件过滤见 api 层）。
async function tryConsumeDrop() {
  const dropped = consumeDrop('manifest')
  if (dropped?.length) {
    if (running.value) await session.cancel()
    manifestPath.value = dropped[0]
    baseDir.value = ''
    void start()
  }
}
onMounted(() => void tryConsumeDrop())
watch(dropPayload, () => void tryConsumeDrop())

onBeforeUnmount(() => {
  if (running.value) void session.cancel()
  session.destroy()
})
</script>

<template>
  <div class="view">
    <n-card size="small" class="control-card">
      <div class="form-block">
        <div class="form-row">
          <span class="form-label">{{ t('batch.manifestLabel') }}</span>
          <n-button :disabled="running" @click="onPickManifest">
            <template #icon><n-icon><DocumentTextOutline /></n-icon></template>
            {{ t('batch.pickManifest') }}
          </n-button>
          <span v-if="manifestPath" class="file-path-text mono" :title="manifestPath">
            {{ manifestPath }}
          </span>
          <n-tag v-if="algo" size="small" type="success" :bordered="false">
            {{ t('batch.detectedAlgo') }}: {{ algoLabel(algo) }}
          </n-tag>
        </div>
        <div class="form-row">
          <span class="form-label">{{ t('batch.baseDirLabel') }}</span>
          <n-button :disabled="running" tertiary @click="onPickBaseDir">
            <template #icon><n-icon><FolderOpenOutline /></n-icon></template>
            {{ t('batch.pickBaseDir') }}
          </n-button>
          <span class="file-path-text mono" :title="baseDir">
            {{ baseDir || t('batch.baseDirDefault') }}
          </span>
          <n-button v-if="baseDir" text size="tiny" :disabled="running" @click="baseDir = ''">
            {{ t('batch.resetBaseDir') }}
          </n-button>
          <span class="toolbar-spacer" />
          <n-button v-if="!running" type="primary" :disabled="!canStart" @click="start">
            <template #icon><n-icon><PlayOutline /></n-icon></template>
            {{ t('common.start') }}
          </n-button>
          <n-button v-else type="error" tertiary @click="session.cancel">
            <template #icon><n-icon><StopOutline /></n-icon></template>
            {{ t('common.cancel') }}
          </n-button>
        </div>
      </div>
    </n-card>

    <progress-panel v-if="running" :progress="progress" :running="running" @cancel="session.cancel" />

    <div v-if="summary || items.length" class="summary-bar">
      <div class="stat-card">
        <span class="stat-label">{{ t('batch.statTotal') }}</span>
        <span class="stat-value">{{ stats.total }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('batch.statPass') }}</span>
        <span class="stat-value ok">{{ stats.pass }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('batch.statFail') }}</span>
        <span class="stat-value" :class="{ err: stats.fail > 0 }">{{ stats.fail }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('batch.statMissing') }}</span>
        <span class="stat-value" :class="{ warn: stats.missing > 0 }">{{ stats.missing }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">{{ t('batch.statError') }}</span>
        <span class="stat-value" :class="{ err: stats.error > 0 }">{{ stats.error }}</span>
      </div>
      <template v-if="summary">
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
      </template>
    </div>

    <div v-if="summary && !summary.canceled" class="export-bar">
      <n-button size="small" :loading="exporting" @click="doExport(true)">
        <template #icon><n-icon><DownloadOutline /></n-icon></template>
        {{ t('batch.exportFailedOnly') }}
      </n-button>
      <n-button size="small" :loading="exporting" @click="doExport(false)">
        <template #icon><n-icon><DownloadOutline /></n-icon></template>
        {{ t('batch.exportAll') }}
      </n-button>
    </div>

    <result-table v-if="items.length" :items="items" :algos="tableAlgos" mode="verify" />

    <div v-else-if="!running && !summary" class="empty-wrap">
      <n-empty :description="t('batch.empty')">
        <template #extra>
          <span class="empty-desc">{{ t('batch.emptyDesc') }}</span>
        </template>
      </n-empty>
    </div>
  </div>
</template>

<style scoped>
.form-block {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-label {
  flex: 0 0 84px;
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.export-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
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
