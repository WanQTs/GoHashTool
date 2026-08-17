// 后端绑定调用封装、事件订阅工具与共享类型定义。
// 所有与 Go 后端的交互都必须经过本模块，视图不直接引用 bindings/@wailsio/runtime。
import { ref, shallowRef } from 'vue'
import { Events } from '@wailsio/runtime'
import * as App from '../../bindings/gohash/app'
import { i18n } from '../locales'
import zhCN from '../locales/zh-cn.json'
import enUS from '../locales/en-us.json'

// ---------- 类型定义（与后端 JSON 契约一致） ----------

export interface AppError {
  code: string
  zh: string
  en: string
  detail?: string
}

export interface Result {
  ok: boolean
  error?: AppError
  paths?: string[]
  path?: string
  taskId?: string
  total: number
  totalBytes: number
  algo?: string
  /** 任务先异步扫描目录：总量/字节数随首个非扫描态进度事件下发（此时为 0） */
  scanning?: boolean
}

export type ItemStatus = 'ok' | 'occupied' | 'no_permission' | 'not_found' | 'error' | 'canceled'
export type Verdict = 'pass' | 'fail' | 'missing' | 'error'

export interface Item {
  path: string
  name: string
  size: number
  hashes: Record<string, string>
  durationMs: number
  status: ItemStatus
  errCode?: string
  expected?: string
  actual?: string
  verdict?: Verdict
}

export interface ProgressEvent {
  taskId: string
  total: number
  done: number
  bytesDone: number
  totalBytes: number
  currentFile: string
  speedMBps: number
  elapsedMs: number
  /** 目录展开阶段：此时 done 是已发现文件数，total/字节字段尚无意义 */
  scanning?: boolean
}

export interface ItemsEvent {
  taskId: string
  items: Item[]
}

export interface DoneEvent {
  taskId: string
  total: number
  ok: number
  errors: number
  pass: number
  fail: number
  missing: number
  canceled: boolean
  elapsedMs: number
  bytesDone: number
  totalBytes: number
  /** 任务 goroutine panic 时的错误信息（后端兜底防护，正常为空） */
  fatal?: string
  /** 任务异步失败（如展开后没有可计算的文件）：toast 展示，不显示汇总条 */
  error?: AppError
}

// ---------- 算法常量 ----------

export const ALGO_LIST = ['md5', 'sha1', 'sha256', 'sha512', 'crc32'] as const
export type Algo = (typeof ALGO_LIST)[number]

export const ALGO_LABELS: Record<string, string> = {
  md5: 'MD5',
  sha1: 'SHA-1',
  sha256: 'SHA-256',
  sha512: 'SHA-512',
  crc32: 'CRC32',
}

export function algoLabel(a: string): string {
  return ALGO_LABELS[a] ?? a.toUpperCase()
}

/**
 * 按期望哈希的十六进制长度识别算法：8=CRC32、32=MD5、40=SHA-1、64=SHA-256、128=SHA-512。
 * 返回 null 表示未输入，'' 表示无法识别（非十六进制或长度不支持）。
 */
export function detectAlgoByHash(input: string): Algo | null | '' {
  const h = input.trim().toLowerCase()
  if (!h) return null
  if (!/^[0-9a-f]+$/.test(h)) return ''
  switch (h.length) {
    case 8:
      return 'crc32'
    case 32:
      return 'md5'
    case 40:
      return 'sha1'
    case 64:
      return 'sha256'
    case 128:
      return 'sha512'
    default:
      return ''
  }
}

// ---------- 错误展示 ----------

/** 前端自发错误（IPC 失败、任务崩溃兜底等）构造与后端同构的双语 AppError，文案取自语言包。 */
function frontendError(code: string, key: keyof typeof zhCN.error, detail?: string): AppError {
  const err: AppError = { code, zh: zhCN.error[key], en: enUS.error[key] }
  if (detail) err.detail = detail
  return err
}

/** 按当前语言取错误文案；哈希值、错误码不翻译。 */
export function errorText(err: AppError | undefined | null, locale: string): string {
  if (!err) return locale === 'zh-CN' ? zhCN.error.unknown : enUS.error.unknown
  const base = locale === 'zh-CN' ? err.zh : err.en
  return err.detail ? `${base}: ${err.detail}` : base
}

// ---------- 绑定调用封装（统一 try/catch，返回 Result） ----------

function ipcFailure(e: unknown): Result {
  return {
    ok: false,
    total: 0,
    totalBytes: 0,
    error: frontendError('ipc', 'ipc', e instanceof Error ? e.message : String(e)),
  }
}

async function call(p: Promise<unknown>): Promise<Result> {
  try {
    return (await p) as Result
  } catch (e) {
    return ipcFailure(e)
  }
}

// 原生对话框标题按当前语言即时取值（绑定调用时求值，切换语言后下次调用即生效）。
export const pickFiles = () => call(App.PickFiles(i18n.global.t('dialog.pickFiles')))
export const pickFolder = () => call(App.PickFolder(i18n.global.t('dialog.pickFolder')))
export const pickManifestFile = () => call(App.PickManifestFile(i18n.global.t('dialog.pickManifest')))
export const pickSavePath = (defaultName: string, filterName: string, pattern: string) =>
  call(App.PickSavePath(defaultName, filterName, pattern, i18n.global.t('dialog.export')))
export const exportCSV = (taskId: string, path: string, onlyFailed: boolean) =>
  call(App.ExportCSV(taskId, path, onlyFailed))
export const exportSUM = (taskId: string, path: string, algo: string) =>
  call(App.ExportSUM(taskId, path, algo))
export const copyText = (text: string) => call(App.CopyText(text))
export const cancelTask = (taskId: string) => call(App.CancelTask(taskId))
export const setAlwaysOnTop = (on: boolean) => call(App.SetAlwaysOnTop(on))
// 文件关联启动带入的清单路径：挂载后拉取一次（拉取模型规避前端就绪前事件丢失）
export const consumePendingOpenFile = () => call(App.ConsumePendingOpenFile())
// 结果行原生右键菜单：启动与切换语言时（重）建；文案按当前语言即时取值
export const setupResultContextMenu = (labels: {
  copyHash: string
  copyPath: string
  reveal: string
}) => call(App.SetupResultContextMenu(labels))

// ---------- 任务会话 ----------

const EVENT_PROGRESS = 'hash:progress'
const EVENT_ITEMS = 'hash:items'
const EVENT_DONE = 'hash:done'

/** 后端 taskId 形如 "t<N>"，序号单调递增；解析失败返回 0。 */
export function taskSeq(taskId: string): number {
  const m = /^t(\d+)$/.exec(taskId)
  return m ? Number(m[1]) : 0
}

/**
 * 事件归属判定。Start*Task 的 Promise 尚未 resolve 时事件可能已到达，
 * starting 窗口内只接受序号大于 minSeqAtStart 的事件——旧任务（如路由切换
 * 时被取消）迟到的残余事件序号必然更小，以此防止跨任务数据串扰。
 */
export function acceptTaskEvent(
  eventTaskId: string,
  currentTaskId: string,
  starting: boolean,
  minSeqAtStart: number,
): boolean {
  if (currentTaskId && eventTaskId === currentTaskId) return true
  if (starting) return taskSeq(eventTaskId) > minSeqAtStart
  return false
}

// 全局已发出的最大任务序号（跨会话单调递增），作为新任务 starting 窗口的过滤基线。
let maxIssuedTaskSeq = 0

/**
 * 一次哈希/校验任务的前端会话：封装事件订阅（按 taskId 过滤）、
 * 结果累积（shallowRef 避免十万级行的深度响应式开销）与生命周期。
 * 视图在 onBeforeUnmount 时必须调用 destroy()。
 */
export function createTaskSession() {
  const taskId = ref('')
  const starting = ref(false)
  const running = ref(false)
  const items = shallowRef<Item[]>([])
  const progress = shallowRef<ProgressEvent | null>(null)
  const summary = shallowRef<DoneEvent | null>(null)
  const algo = ref('')
  const error = shallowRef<AppError | null>(null)

  let subscribed = false
  let minSeqAtStart = 0
  // Events.On 返回的单个监听取消函数；destroy 时必须只退订本会话的监听，
  // 不能用 Events.Off(name)（会把其他视图会话的同名监听一并移除）。
  let offFns: Array<() => void> = []

  function matches(payload: { taskId: string }): boolean {
    return acceptTaskEvent(payload.taskId, taskId.value, starting.value, minSeqAtStart)
  }

  function onProgress(p: ProgressEvent) {
    if (matches(p)) progress.value = p
  }

  function onItems(e: ItemsEvent) {
    if (!matches(e) || !e.items?.length) return
    items.value = items.value.concat(e.items)
  }

  function onDone(s: DoneEvent) {
    if (!matches(s)) return
    summary.value = s
    running.value = false
    starting.value = false
    if (s.fatal) {
      error.value = frontendError('fatal', 'fatal', s.fatal)
    }
  }

  function subscribe() {
    if (subscribed) return
    subscribed = true
    offFns = [
      Events.On(EVENT_PROGRESS, (ev) => onProgress(ev.data as ProgressEvent)),
      Events.On(EVENT_ITEMS, (ev) => onItems(ev.data as ItemsEvent)),
      Events.On(EVENT_DONE, (ev) => onDone(ev.data as DoneEvent)),
    ]
  }

  function unsubscribe() {
    if (!subscribed) return
    subscribed = false
    for (const off of offFns) off()
    offFns = []
  }

  function reset() {
    taskId.value = ''
    starting.value = false
    running.value = false
    items.value = []
    progress.value = null
    summary.value = null
    algo.value = ''
    error.value = null
  }

  async function begin(p: Promise<unknown>): Promise<boolean> {
    reset()
    subscribe()
    minSeqAtStart = maxIssuedTaskSeq
    starting.value = true
    running.value = true
    const r = (await call(p)) as Result
    starting.value = false
    if (!r.ok) {
      running.value = false
      error.value = r.error ?? frontendError('unknown', 'startFailed')
      return false
    }
    taskId.value = r.taskId ?? ''
    algo.value = r.algo ?? ''
    const seq = taskSeq(taskId.value)
    if (seq > maxIssuedTaskSeq) maxIssuedTaskSeq = seq
    progress.value = {
      taskId: taskId.value,
      total: r.total,
      done: 0,
      bytesDone: 0,
      totalBytes: r.totalBytes,
      currentFile: '',
      speedMBps: 0,
      elapsedMs: 0,
      scanning: r.scanning ?? false,
    }
    return true
  }

  const startHash = (paths: string[], algos: string[]) => begin(App.StartHashTask(paths, algos))
  const startVerify = (manifestPath: string, baseDir: string) =>
    begin(App.StartVerifyTask(manifestPath, baseDir))

  async function cancel(): Promise<void> {
    if (taskId.value) await cancelTask(taskId.value)
  }

  function destroy() {
    unsubscribe()
  }

  return {
    taskId,
    running,
    items,
    progress,
    summary,
    algo,
    error,
    startHash,
    startVerify,
    cancel,
    reset,
    destroy,
  }
}

export type TaskSession = ReturnType<typeof createTaskSession>

// ---------- 结果行右键菜单 ----------

/**
 * 结果行的原生右键菜单数据编码：encodeURIComponent(JSON)——
 * 与 Go 侧 decodeRowContext（url.PathUnescape + json.Unmarshal）一一对应。
 * 写在表格行的 --custom-contextmenu-data CSS 变量上，由 Wails runtime 原样传给 Go。
 */
export function encodeRowContext(row: Item): string {
  return encodeURIComponent(JSON.stringify({ path: row.path, hashes: row.hashes ?? {} }))
}

/**
 * 订阅右键菜单动作反馈（复制成功 / 动作失败 toast），返回取消函数。
 * 菜单动作在 Go 侧闭环，反馈只能经事件回来（原生菜单没有返回值通道）。
 */
export function onContextFeedback(onCopied: () => void, onError: (err: AppError) => void) {
  const offCopied = Events.On('context:copied', () => onCopied())
  const offError = Events.On('context:error', (ev) => onError(ev.data as AppError))
  return () => {
    offCopied()
    offError()
  }
}

// ---------- 整窗拖拽路由 ----------

export interface DropPayload {
  kind: 'files' | 'manifest'
  paths: string[]
}

/** 全局拖拽载荷总线：app.vue 的 OnFileDrop 写入，目标视图按 kind 消费。 */
export const dropPayload = shallowRef<DropPayload | null>(null)

const MANIFEST_EXTS = ['.sha256', '.sha1', '.sha512', '.md5', '.txt', '.sum', '.sums']

export function isManifestPath(p: string): boolean {
  const lower = p.toLowerCase()
  return MANIFEST_EXTS.some((ext) => lower.endsWith(ext))
}

/** 拖拽路由规则：恰一个清单文件 → 批量校验；否则 → 哈希计算。返回目标路由路径。 */
export function dispatchDrop(paths: string[]): '/' | '/batch' {
  const isManifest = paths.length === 1 && isManifestPath(paths[0])
  dropPayload.value = { kind: isManifest ? 'manifest' : 'files', paths }
  return isManifest ? '/batch' : '/'
}

/** 消费匹配 kind 的拖拽载荷（一次性）。 */
export function consumeDrop(kind: 'files' | 'manifest'): string[] | null {
  const d = dropPayload.value
  if (!d || d.kind !== kind) return null
  dropPayload.value = null
  return d.paths
}
