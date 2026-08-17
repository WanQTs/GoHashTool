<script setup lang="ts">
// 单文件校验页：选一个文件 + 粘贴期望哈希（按长度自动识别算法）→ 比对。
import { computed, onBeforeUnmount, ref } from 'vue'
import { NButton, NCard, NEmpty, NIcon, NInput, NTag, useMessage } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloseCircleOutline,
  DocumentOutline,
  PlayOutline,
  WarningOutline,
} from '@vicons/ionicons5'
import { useI18n } from 'vue-i18n'
import {
  algoLabel,
  createTaskSession,
  detectAlgoByHash,
  errorText,
  pickFiles,
  type Result,
} from '../api'
import { useSettingsStore } from '../stores/settings'
import ProgressPanel from '../components/progress-panel.vue'

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const session = createTaskSession()
const { running, items, progress, summary } = session

const filePath = ref('')
const expected = ref('')
/** 本次校验实际使用的输入快照：任务完成后用户改输入不影响已展示的结论。 */
const usedExpected = ref('')
const usedAlgo = ref('')

/** 粘贴值按长度识别算法（含 8 位 CRC32），识别逻辑见 api.detectAlgoByHash。 */
const normalized = computed(() => expected.value.trim().toLowerCase())
const detectedAlgo = computed(() => detectAlgoByHash(expected.value))

const canStart = computed(
  () => !!filePath.value && !!detectedAlgo.value && !running.value,
)

const resultItem = computed(() => (items.value.length ? items.value[0] : null))

/** 结论只由任务快照推导：完成后修改文件/期望哈希输入不会污染旧结论。 */
const verdict = computed<'match' | 'mismatch' | 'error' | null>(() => {
  if (!summary.value || !resultItem.value) return null
  const item = resultItem.value
  if (item.status !== 'ok') return 'error'
  const actual = (item.hashes?.[usedAlgo.value] ?? '').toLowerCase()
  if (!actual || !usedExpected.value) return 'error'
  return actual === usedExpected.value ? 'match' : 'mismatch'
})

const actualHash = computed(() => {
  if (!usedAlgo.value || !resultItem.value) return ''
  return resultItem.value.hashes?.[usedAlgo.value] ?? ''
})

function showError(r: Result) {
  message.error(errorText(r.error, settings.locale))
}

async function onPickFile() {
  const r = await pickFiles()
  if (!r.ok) return showError(r)
  if (r.paths?.length) filePath.value = r.paths[0]
}

async function start() {
  if (!filePath.value) return message.warning(t('verify.needFile'))
  const algo = detectedAlgo.value
  if (!algo) return message.warning(t('verify.needHash'))
  usedExpected.value = normalized.value
  usedAlgo.value = algo
  const ok = await session.startHash([filePath.value], [algo])
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
          <span class="form-label">{{ t('verify.fileLabel') }}</span>
          <n-button :disabled="running" @click="onPickFile">
            <template #icon><n-icon><DocumentOutline /></n-icon></template>
            {{ t('verify.pickFile') }}
          </n-button>
          <span v-if="filePath" class="file-path-text mono" :title="filePath">{{ filePath }}</span>
        </div>

        <div class="form-row">
          <span class="form-label">{{ t('verify.expectedLabel') }}</span>
          <n-input
            v-model:value="expected"
            class="hash-input mono"
            :placeholder="t('verify.expectedPlaceholder')"
            :disabled="running"
            clearable
            spellcheck="false"
          />
        </div>

        <div class="form-row detect-row">
          <span class="form-label" />
          <n-tag v-if="detectedAlgo" size="small" type="success" :bordered="false">
            {{ t('verify.detected') }}: {{ algoLabel(detectedAlgo) }}
          </n-tag>
          <span v-else-if="detectedAlgo === ''" class="detect-warn">{{ t('verify.unknown') }}</span>
          <span class="toolbar-spacer" />
          <n-button type="primary" :disabled="!canStart" :loading="running" @click="start">
            <template #icon><n-icon><PlayOutline /></n-icon></template>
            {{ t('common.start') }}
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
            {{ verdict === 'error' ? t('verify.computeFailed') : t(`verdict.${verdict}`) }}
          </div>
          <div class="verdict-desc">
            <template v-if="verdict === 'match'">{{ t('verify.matchDesc') }}</template>
            <template v-else-if="verdict === 'mismatch'">{{ t('verify.mismatchDesc') }}</template>
            <template v-else>{{ t(`status.${resultItem?.status ?? 'error'}`) }}</template>
          </div>
        </div>
      </div>

      <div v-if="verdict !== 'error'" class="hash-pair">
        <div class="pair-item">
          <div class="pair-label">{{ t('verify.expected') }}</div>
          <div class="pair-value mono">{{ usedExpected }}</div>
        </div>
        <div class="pair-item">
          <div class="pair-label">{{ t('verify.actual') }} · {{ algoLabel(usedAlgo) }}</div>
          <div class="pair-value mono">{{ actualHash }}</div>
        </div>
      </div>
    </template>

    <div v-else-if="!running" class="empty-wrap">
      <n-empty :description="t('verify.empty')">
        <template #extra>
          <span class="empty-desc">{{ t('verify.emptyDesc') }}</span>
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

.hash-input {
  flex: 1;
  min-width: 0;
  font-size: 13px;
}

.detect-row {
  min-height: 24px;
}

.detect-warn {
  font-size: 12px;
  color: #f0a020;
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
