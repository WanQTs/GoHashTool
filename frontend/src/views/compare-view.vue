<script setup lang="ts">
// 双文件对比页：两个文件 + 算法多选 → 各算法哈希并排比对。
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton, NCard, NEmpty, NIcon, useMessage } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloseCircleOutline,
  DocumentOutline,
  PlayOutline,
  StopOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import {
  algoLabel,
  copyText,
  createTaskSession,
  errorText,
  pickFiles,
  type Result,
} from '../api'
import { useSettingsStore } from '../stores/settings'
import AlgoChips from '../components/algo-chips.vue'
import ProgressPanel from '../components/progress-panel.vue'

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const session = createTaskSession()
const { running, items, progress, summary } = session

const fileA = ref('')
const fileB = ref('')
const algos = ref<string[]>(['sha256'])
/** 本次任务实际使用的算法与文件快照（运行后用户改动不影响结果展示）。 */
const usedAlgos = ref<string[]>([])
const usedPaths = ref<[string, string]>(['', ''])

const canStart = computed(
  () => !!fileA.value && !!fileB.value && algos.value.length > 0 && !running.value,
)

const itemA = computed(() => items.value.find((i) => i.path === usedPaths.value[0]) ?? null)
const itemB = computed(() => items.value.find((i) => i.path === usedPaths.value[1]) ?? null)

function hashOf(item: { hashes: Record<string, string> } | null, algo: string): string {
  return item?.hashes?.[algo] ?? ''
}

/** 全部所选算法都一致才算一致。 */
const verdict = computed<'match' | 'mismatch' | 'error' | null>(() => {
  if (!summary.value) return null
  const a = itemA.value
  const b = itemB.value
  if (!a || !b || a.status !== 'ok' || b.status !== 'ok') return 'error'
  const same = usedAlgos.value.every(
    (al) => hashOf(a, al).toLowerCase() === hashOf(b, al).toLowerCase(),
  )
  return same ? 'match' : 'mismatch'
})

function algoDiffers(algo: string): boolean {
  return (
    hashOf(itemA.value, algo).toLowerCase() !== hashOf(itemB.value, algo).toLowerCase()
  )
}

function showError(r: Result) {
  message.error(errorText(r.error, settings.locale))
}

/** 点击哈希值复制（与结果表格行为一致）；空值不响应。 */
async function copyHash(v: string) {
  if (!v) return
  const r = await copyText(v)
  if (r.ok) message.success(t('common.copied'))
  else message.error(errorText(r.error, settings.locale))
}

async function pick(which: 'a' | 'b') {
  const r = await pickFiles()
  if (!r.ok) return showError(r)
  if (!r.paths?.length) return
  if (which === 'a') fileA.value = r.paths[0]
  else fileB.value = r.paths[0]
}

async function start() {
  if (!fileA.value || !fileB.value) return message.warning(t('compare.needBoth'))
  if (!algos.value.length) return message.warning(t('hash.needAlgo'))
  usedAlgos.value = [...algos.value]
  usedPaths.value = [fileA.value, fileB.value]
  const ok = await session.startHash([fileA.value, fileB.value], usedAlgos.value)
  if (!ok && session.error.value) message.error(errorText(session.error.value, settings.locale))
}

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
          <span class="form-label">{{ t('compare.fileA') }}</span>
          <n-button :disabled="running" @click="pick('a')">
            <template #icon><n-icon><DocumentOutline /></n-icon></template>
            {{ t('compare.pickFile') }}
          </n-button>
          <span v-if="fileA" class="file-path-text mono" :title="fileA">{{ fileA }}</span>
        </div>
        <div class="form-row">
          <span class="form-label">{{ t('compare.fileB') }}</span>
          <n-button :disabled="running" @click="pick('b')">
            <template #icon><n-icon><DocumentOutline /></n-icon></template>
            {{ t('compare.pickFile') }}
          </n-button>
          <span v-if="fileB" class="file-path-text mono" :title="fileB">{{ fileB }}</span>
        </div>
        <div class="form-row">
          <span class="form-label" />
          <algo-chips v-model="algos" :disabled="running" />
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

    <template v-if="verdict">
      <div class="verdict-banner" :class="verdict">
        <n-icon :size="40" class="verdict-icon" :color="verdict === 'match' ? '#18a058' : verdict === 'mismatch' ? '#d03050' : '#f0a020'">
          <CheckmarkCircleOutline v-if="verdict === 'match'" />
          <CloseCircleOutline v-else-if="verdict === 'mismatch'" />
          <WarningOutline v-else />
        </n-icon>
        <div>
          <div class="verdict-text">
            {{ verdict === 'error' ? t('compare.computeFailed') : t(`verdict.${verdict}`) }}
          </div>
          <div class="verdict-desc">
            <template v-if="verdict === 'match'">{{ t('compare.matchDesc') }}</template>
            <template v-else-if="verdict === 'mismatch'">{{ t('compare.mismatchDesc') }}</template>
          </div>
        </div>
      </div>

      <div v-if="verdict !== 'error'" class="compare-table">
        <div class="compare-row compare-head">
          <span>{{ t('compare.colAlgo') }}</span>
          <span>{{ t('compare.fileA') }}</span>
          <span>{{ t('compare.fileB') }}</span>
        </div>
        <div
          v-for="al in usedAlgos"
          :key="al"
          class="compare-row"
          :class="{ differ: algoDiffers(al) }"
        >
          <span class="algo-name">{{ algoLabel(al) }}</span>
          <span
            class="mono hash-text"
            :class="{ copyable: !!hashOf(itemA, al) }"
            :title="hashOf(itemA, al) ? `${hashOf(itemA, al)}\n${t('table.clickToCopy')}` : ''"
            @click="copyHash(hashOf(itemA, al))"
          >{{ hashOf(itemA, al) || '—' }}</span>
          <span
            class="mono hash-text"
            :class="{ copyable: !!hashOf(itemB, al) }"
            :title="hashOf(itemB, al) ? `${hashOf(itemB, al)}\n${t('table.clickToCopy')}` : ''"
            @click="copyHash(hashOf(itemB, al))"
          >{{ hashOf(itemB, al) || '—' }}</span>
        </div>
      </div>
    </template>

    <div v-else-if="!running" class="empty-wrap">
      <n-empty :description="t('compare.empty')">
        <template #extra>
          <span class="empty-desc">{{ t('compare.emptyDesc') }}</span>
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
  flex: 0 0 64px;
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.compare-table {
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background-color: var(--panel-bg);
  overflow: hidden;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease;
}

.compare-row {
  display: grid;
  grid-template-columns: 110px 1fr 1fr;
  gap: 12px;
  padding: 9px 14px;
  align-items: center;
  border-top: 1px solid var(--border-color);
  font-size: 13px;
}

.compare-row:first-child {
  border-top: none;
}

.compare-head {
  font-size: 12px;
  color: var(--text-secondary);
}

.compare-row.differ {
  background-color: var(--row-fail);
}

.algo-name {
  font-weight: 600;
  white-space: nowrap;
}

.hash-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}

.hash-text.copyable {
  cursor: pointer;
  border-radius: 4px;
}

.hash-text.copyable:hover {
  background-color: rgba(47, 123, 255, 0.12);
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
