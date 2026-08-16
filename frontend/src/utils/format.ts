// 字节数、耗时、速度等展示格式化工具。

/** 人性化字节格式化：1234 -> "1.2 KB"；负数（未知大小）显示占位符。 */
export function formatBytes(bytes: number): string {
  if (bytes == null || Number.isNaN(bytes) || bytes < 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

/** 耗时格式化：850 -> "850 ms"，2300 -> "2.3 s"，95000 -> "1 min 35 s"。 */
export function formatDuration(ms: number): string {
  if (ms == null || Number.isNaN(ms) || ms < 0) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)} s`
  const m = Math.floor(s / 60)
  const rs = Math.round(s % 60)
  return `${m} min ${rs} s`
}

/** 实时/平均速度格式化（MB/s）。 */
export function formatSpeed(mbps: number): string {
  if (!mbps || Number.isNaN(mbps) || mbps < 0) return '0 MB/s'
  return `${mbps >= 100 ? mbps.toFixed(0) : mbps.toFixed(1)} MB/s`
}

/** 由字节数与耗时计算平均速度（MB/s）。 */
export function avgSpeedMBps(bytesDone: number, elapsedMs: number): number {
  if (elapsedMs <= 0) return 0
  return bytesDone / (elapsedMs / 1000) / 1e6
}

/** 取路径的文件名部分（兼容 Windows 与 POSIX 分隔符）。 */
export function baseName(p: string): string {
  if (!p) return ''
  const norm = p.replace(/\\/g, '/').replace(/\/+$/, '')
  const idx = norm.lastIndexOf('/')
  return idx >= 0 ? norm.slice(idx + 1) : norm
}
