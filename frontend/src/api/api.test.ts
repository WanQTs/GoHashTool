// api 层纯函数测试：拖拽路由、清单判定、错误文案、跨任务事件过滤与会话订阅生命周期。
// 生成的 bindings 与 @wailsio/runtime 在 node 环境不可用，用 vi.mock 隔离（不触达真实后端）。
import { beforeEach, describe, expect, it, vi } from 'vitest'

// Events.On 按约定返回「仅取消该监听」的函数；用 spy 记录以便断言退订行为。
const mocks = vi.hoisted(() => {
  const offFns: Array<ReturnType<typeof vi.fn>> = []
  return {
    offFns,
    EventsOn: vi.fn(() => {
      const off = vi.fn()
      offFns.push(off)
      return off
    }),
    StartHashTask: vi.fn(async (): Promise<unknown> => ({ ok: true, taskId: 't9', total: 0, totalBytes: 0 })),
  }
})
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.EventsOn } }))
vi.mock('../../bindings/gohash/app', () => ({ StartHashTask: mocks.StartHashTask }))

import {
  acceptTaskEvent,
  consumeDrop,
  createTaskSession,
  dispatchDrop,
  dropPayload,
  errorText,
  isManifestPath,
  taskSeq,
} from './index'

describe('isManifestPath', () => {
  it('清单扩展名（大小写不敏感）', () => {
    for (const p of ['a.sha256', 'b.SHA1', 'c.md5', 'd.txt', 'e.sum', 'f.sums', 'g.sha512']) {
      expect(isManifestPath(p)).toBe(true)
    }
  })
  it('非清单扩展名', () => {
    for (const p of ['a.exe', 'b.bin', 'c', 'd.sha256.txt.bak']) {
      expect(isManifestPath(p)).toBe(false)
    }
  })
})

describe('dispatchDrop / consumeDrop', () => {
  beforeEach(() => {
    dropPayload.value = null
  })

  it('恰一个清单文件 → 批量校验', () => {
    expect(dispatchDrop(['D:\\sums\\a.sha256'])).toBe('/batch')
    expect(dropPayload.value).toEqual({ kind: 'manifest', paths: ['D:\\sums\\a.sha256'] })
    expect(consumeDrop('manifest')).toEqual(['D:\\sums\\a.sha256'])
    // 一次性消费：再取为 null
    expect(consumeDrop('manifest')).toBeNull()
  })

  it('多文件或单非清单 → 哈希计算', () => {
    expect(dispatchDrop(['a.bin', 'b.bin'])).toBe('/')
    expect(dropPayload.value?.kind).toBe('files')
    expect(dispatchDrop(['a.exe'])).toBe('/')
  })

  it('kind 不匹配不消费', () => {
    dispatchDrop(['a.sha256'])
    expect(consumeDrop('files')).toBeNull()
    expect(dropPayload.value).not.toBeNull()
  })
})

describe('errorText', () => {
  const err = { code: 'x', zh: '中文错误', en: 'English error', detail: 'd' }
  it('按语言取文案并拼接 detail', () => {
    expect(errorText(err, 'zh-CN')).toBe('中文错误: d')
    expect(errorText(err, 'en-US')).toBe('English error: d')
  })
  it('无 detail 与空错误兜底', () => {
    expect(errorText({ code: 'x', zh: '中', en: 'en' }, 'en-US')).toBe('en')
    expect(errorText(null, 'zh-CN')).toBe('未知错误')
    expect(errorText(undefined, 'en-US')).toBe('Unknown error')
  })
})

describe('taskSeq / acceptTaskEvent（跨任务事件过滤）', () => {
  it('taskSeq 解析', () => {
    expect(taskSeq('t12')).toBe(12)
    expect(taskSeq('x')).toBe(0)
    expect(taskSeq('')).toBe(0)
  })

  it('taskId 已知后按精确匹配', () => {
    expect(acceptTaskEvent('t3', 't3', false, 1)).toBe(true)
    expect(acceptTaskEvent('t2', 't3', false, 1)).toBe(false)
  })

  it('starting 窗口只接受序号更大的事件，过滤旧任务残余', () => {
    // 旧任务 t2 的迟到 done：序号 ≤ 基线，拒绝
    expect(acceptTaskEvent('t2', '', true, 2)).toBe(false)
    // 新任务 t3 的事件：序号 > 基线，接受
    expect(acceptTaskEvent('t3', '', true, 2)).toBe(true)
  })

  it('非 starting 且 taskId 未知时一律拒绝', () => {
    expect(acceptTaskEvent('t9', '', false, 0)).toBe(false)
  })
})

describe('createTaskSession 订阅生命周期', () => {
  beforeEach(() => {
    mocks.offFns.length = 0
    mocks.EventsOn.mockClear()
  })

  it('destroy 用 EventsOn 返回的取消函数逐个退订，且幂等', async () => {
    const session = createTaskSession()
    const ok = await session.startHash(['a.bin'], ['sha256'])
    expect(ok).toBe(true)
    expect(session.taskId.value).toBe('t9')
    // 三个事件各订阅一次
    expect(mocks.EventsOn).toHaveBeenCalledTimes(3)
    expect(mocks.offFns).toHaveLength(3)

    session.destroy()
    for (const off of mocks.offFns) expect(off).toHaveBeenCalledTimes(1)
    // 重复 destroy 不重复退订
    session.destroy()
    for (const off of mocks.offFns) expect(off).toHaveBeenCalledTimes(1)
  })

  it('startHash 失败时不留下订阅', async () => {
    mocks.StartHashTask.mockResolvedValueOnce({
      ok: false,
      total: 0,
      totalBytes: 0,
      error: { code: 'no_files', zh: '没有可计算的文件', en: 'No files to hash' },
    })
    const session = createTaskSession()
    const ok = await session.startHash([], ['sha256'])
    expect(ok).toBe(false)
    expect(session.error.value?.code).toBe('no_files')
    session.destroy()
    for (const off of mocks.offFns) expect(off).toHaveBeenCalledTimes(1)
  })
})
