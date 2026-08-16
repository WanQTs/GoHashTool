// 展示格式化工具的单元测试（纯函数，无 DOM 依赖）。
import { describe, expect, it } from 'vitest'
import { avgSpeedMBps, baseName, formatBytes, formatDuration, formatSpeed } from './format'

describe('formatBytes', () => {
  it('字节级直接显示', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
  })
  it('KB/MB/GB 进位与精度', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(1048576)).toBe('1.0 MB')
    expect(formatBytes(1073741824)).toBe('1.0 GB')
    expect(formatBytes(120 * 1024 * 1024)).toBe('120 MB')
  })
  it('负数与非法值显示占位符', () => {
    expect(formatBytes(-1)).toBe('—')
    expect(formatBytes(Number.NaN)).toBe('—')
  })
})

describe('formatDuration', () => {
  it('毫秒/秒/分钟三段格式', () => {
    expect(formatDuration(850)).toBe('850 ms')
    expect(formatDuration(2300)).toBe('2.3 s')
    expect(formatDuration(95000)).toBe('1 min 35 s')
  })
  it('非法值显示占位符', () => {
    expect(formatDuration(-1)).toBe('—')
    expect(formatDuration(Number.NaN)).toBe('—')
  })
})

describe('formatSpeed / avgSpeedMBps', () => {
  it('速度格式化', () => {
    expect(formatSpeed(0)).toBe('0 MB/s')
    expect(formatSpeed(3.14159)).toBe('3.1 MB/s')
    expect(formatSpeed(128.4)).toBe('128 MB/s')
  })
  it('平均速度计算与除零保护', () => {
    expect(avgSpeedMBps(1e6, 1000)).toBe(1)
    expect(avgSpeedMBps(100, 0)).toBe(0)
  })
})

describe('baseName', () => {
  it('兼容 Windows 与 POSIX 分隔符', () => {
    expect(baseName('C:\\data\\a.txt')).toBe('a.txt')
    expect(baseName('/home/u/a.txt')).toBe('a.txt')
    expect(baseName('C:\\mixed\\sep/a.txt')).toBe('a.txt')
  })
  it('尾部分隔符与空路径', () => {
    expect(baseName('C:\\data\\dir\\')).toBe('dir')
    expect(baseName('')).toBe('')
    expect(baseName('a.txt')).toBe('a.txt')
  })
})
