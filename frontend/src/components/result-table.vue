<script setup lang="ts">
// 结果表格：哈希计算（mode=hash）与批量校验（mode=verify）共用。
// 十万级行数依赖 n-data-table 的 virtual-scroll；哈希单元格点击复制。
// 工具栏支持按路径关键字筛选与「仅看问题行」，大结果集下快速定位失败项。
// 高度自适应：ResizeObserver 实测表体容器高度喂给 max-height（virtual-scroll 需具体像素值）。
import { computed, h, onBeforeUnmount, onMounted, ref, type CSSProperties } from 'vue'
import { NCheckbox, NDataTable, NInput, NTag, useMessage, type DataTableColumns } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  algoLabel,
  copyText,
  encodeRowContext,
  errorText,
  onContextFeedback,
  type AppError,
  type Item,
} from '../api'
import { baseName, formatBytes, formatDuration } from '../utils/format'
import { useSettingsStore } from '../stores/settings'

const props = withDefaults(
  defineProps<{
    items: Item[]
    algos: string[]
    mode?: 'hash' | 'verify'
    maxHeight?: number
  }>(),
  { mode: 'hash', maxHeight: 480 },
)

const { t } = useI18n()
const message = useMessage()
const settings = useSettingsStore()

const wrapRef = ref<HTMLElement | null>(null)
const tableMaxHeight = ref(props.maxHeight)
let resizeObserver: ResizeObserver | null = null

// 右键菜单动作反馈（复制成功/失败 toast）；菜单动作在 Go 侧闭环，反馈经事件回来
let offFeedback: (() => void) | null = null

onMounted(() => {
  offFeedback = onContextFeedback(
    () => message.success(t('common.copied')),
    (err: AppError) => message.error(errorText(err, settings.locale)),
  )
  if (!wrapRef.value || typeof ResizeObserver === 'undefined') return
  resizeObserver = new ResizeObserver(() => {
    const h = wrapRef.value?.clientHeight ?? 0
    if (h > 0) tableMaxHeight.value = Math.max(240, h)
  })
  resizeObserver.observe(wrapRef.value)
})

onBeforeUnmount(() => {
  offFeedback?.()
  resizeObserver?.disconnect()
})

// ---------- 行筛选（路径关键字 + 仅看问题行） ----------

const filterText = ref('')
const onlyProblems = ref(false)

/** 问题行判定：校验模式看结论（不一致/缺失/无法读取），哈希模式看状态（非 ok）。 */
function isProblem(row: Item): boolean {
  if (props.mode === 'verify') {
    return row.verdict === 'fail' || row.verdict === 'missing' || row.verdict === 'error'
  }
  return row.status !== 'ok'
}

const filteredItems = computed(() => {
  const q = filterText.value.trim().toLowerCase()
  if (!q && !onlyProblems.value) return props.items
  return props.items.filter((it) => {
    if (onlyProblems.value && !isProblem(it)) return false
    if (q && !it.path.toLowerCase().includes(q)) return false
    return true
  })
})

const filtering = computed(() => !!filterText.value.trim() || onlyProblems.value)

async function doCopy(text: string) {
  const r = await copyText(text)
  if (r.ok) {
    message.success(t('common.copied'))
  } else {
    message.error(errorText(r.error, settings.locale))
  }
}

/** 等宽字体 + 省略号 + 悬浮完整值 + 点击复制的哈希单元格。 */
function hashCell(value: string | undefined) {
  if (!value) return h('span', { style: 'opacity:0.4' }, '—')
  return h(
    'span',
    {
      class: 'hash-cell',
      title: `${value}\n${t('table.clickToCopy')}`,
      onClick: () => void doCopy(value),
    },
    value,
  )
}

function fileCell(row: Item) {
  return h('div', { class: 'cell-file', title: row.path }, [
    h('div', { class: 'cell-name' }, row.name || baseName(row.path)),
    h('div', { class: 'cell-path mono' }, row.path),
  ])
}

const STATUS_TAG: Record<string, 'success' | 'warning' | 'error' | 'info' | 'default'> = {
  ok: 'success',
  occupied: 'warning',
  no_permission: 'warning',
  not_found: 'info',
  error: 'error',
  canceled: 'default',
}

function statusCell(row: Item) {
  return h(
    NTag,
    { size: 'small', bordered: false, type: STATUS_TAG[row.status] ?? 'default' },
    { default: () => t(`status.${row.status}`) },
  )
}

function verdictCell(row: Item) {
  if (!row.verdict) return h('span', { style: 'opacity:0.4' }, '—')
  // pass 绿 / missing 橙 / fail 与 error（存在但不可读）红
  const type = row.verdict === 'pass' ? 'success' : row.verdict === 'missing' ? 'warning' : 'error'
  return h(NTag, { size: 'small', bordered: false, type }, { default: () => t(`verdict.${row.verdict}`) })
}

const columns = computed<DataTableColumns<Item>>(() => {
  const cols: DataTableColumns<Item> = [
    {
      title: t('table.file'),
      key: 'name',
      minWidth: 200,
      render: fileCell,
    },
    {
      title: t('table.size'),
      key: 'size',
      width: 90,
      align: 'right',
      sorter: (a, b) => a.size - b.size,
      render: (row) => formatBytes(row.size),
    },
  ]

  if (props.mode === 'verify') {
    cols.push(
      {
        title: t('table.expected'),
        key: 'expected',
        width: 220,
        render: (row) => hashCell(row.expected),
      },
      {
        title: t('table.actual'),
        key: 'actual',
        width: 220,
        render: (row) => hashCell(row.actual),
      },
      {
        title: t('table.result'),
        key: 'verdict',
        width: 90,
        render: verdictCell,
      },
    )
  } else {
    for (const a of props.algos) {
      cols.push({
        title: algoLabel(a),
        key: `hash-${a}`,
        width: 200,
        render: (row) => hashCell(row.hashes?.[a]),
      })
    }
    cols.push({
      title: t('table.duration'),
      key: 'durationMs',
      width: 90,
      align: 'right',
      sorter: (a, b) => a.durationMs - b.durationMs,
      render: (row) => formatDuration(row.durationMs),
    })
  }

  cols.push({
    title: t('table.status'),
    key: 'status',
    width: 100,
    render: statusCell,
  })
  return cols
})

function rowClassName(row: Item): string {
  if (props.mode === 'verify') {
    if (row.verdict === 'pass') return 'row-pass'
    if (row.verdict === 'fail') return 'row-fail'
    if (row.verdict === 'missing') return 'row-missing'
    if (row.verdict === 'error') return 'row-error'
    return ''
  }
  return row.status === 'ok' ? '' : 'row-error'
}

// 行级原生右键菜单：在 tr 上声明 CSS 变量（自定义属性随继承传递，
// runtime 在右键事件目标上经 getComputedStyle 读到的就是所在行的菜单名与行数据）。
function rowProps(row: Item) {
  return {
    style: {
      '--custom-contextmenu': 'result-row',
      '--custom-contextmenu-data': encodeRowContext(row),
    } as CSSProperties,
  }
}

const rowKey = (row: Item) => row.path
</script>

<template>
  <div class="table-wrap">
    <div class="table-toolbar">
      <n-input
        v-model:value="filterText"
        size="small"
        clearable
        :placeholder="t('table.filterPlaceholder')"
        class="filter-input"
      />
      <n-checkbox v-model:checked="onlyProblems" size="small">
        {{ t('table.onlyProblems') }}
      </n-checkbox>
      <span v-if="filtering" class="filter-count">
        {{ t('table.rowCount', { shown: filteredItems.length, total: items.length }) }}
      </span>
    </div>
    <div ref="wrapRef" class="table-body">
      <n-data-table
        :columns="columns"
        :data="filteredItems"
        :row-key="rowKey"
        :row-class-name="rowClassName"
        :row-props="rowProps"
        virtual-scroll
        :min-row-height="38"
        :max-height="tableMaxHeight"
        size="small"
        :bordered="false"
        class="result-table"
      />
    </div>
  </div>
</template>

<style scoped>
.table-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.table-toolbar {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 12px;
}

.filter-input {
  max-width: 280px;
}

.filter-count {
  font-size: 12px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.table-body {
  flex: 1;
  min-height: 0;
}
</style>
