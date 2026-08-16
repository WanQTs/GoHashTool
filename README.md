# 文件哈希工具 / File Hash Tool

一个 Windows 64 位桌面工具：文件哈希值获取与对比。基于 Wails v2 + Go + Vue 3。
A Windows 64-bit desktop tool for computing and comparing file hashes. Built with Wails v2 + Go + Vue 3.

## 功能 / Features

- **哈希计算 / Hash**：多选文件、文件夹递归、整窗拖拽添加；MD5 / SHA-1 / SHA-256 / SHA-512 / CRC32 多算法一次扫描；结果表格虚拟滚动（十万级行数不卡）；点击哈希值复制。
- **单文件校验 / Verify**：粘贴期望哈希值，按长度自动识别算法（32=MD5、40=SHA-1、64=SHA-256、128=SHA-512），大字号给出「一致 / 不一致」结论。
- **双文件对比 / Compare**：选两个文件同算法对比，并排展示各算法哈希值。
- **批量校验 / Batch Verify**：导入 md5sum/sha256sum 标准清单（.sha256/.sha1/.sha512/.md5/.txt），算法按扩展名优先、长度推断兜底；基准目录可切换；输出 通过/失败/缺失 统计与明细，失败项显示「期望值 vs 实际值」；一键导出不一致项。
- **导出 / Export**：CSV（带 UTF-8 BOM，Excel 直开不乱码）与标准 SUM 格式；导出的 SUM 可被批量校验重新导入（闭环，有集成测试保障）。
- **界面 / UI**：浅色/深色双主题（默认跟随系统）、中英双语即时切换（默认跟随系统语言）、150–250ms 过渡动画、空状态引导。

## 使用方法 / Usage

双击运行 `build/bin/gohash.exe`。

1. **哈希计算**：点击「选择文件 / 选择文件夹」，或直接把文件/文件夹拖进窗口；勾选算法（默认 SHA-256）后点「开始」。计算中显示总进度条、当前文件、已用时间、实时速度，可随时取消。
2. **单文件校验**：选择文件并粘贴期望哈希值，工具自动识别算法并开始计算，完成后显示对比结论。
3. **双文件对比**：分别选择两个文件，点击「开始对比」。
4. **批量校验**：选择清单文件（或直接把清单文件拖进窗口，会自动跳转并开始校验）；默认以清单所在目录为基准解析相对路径，也可手动指定其他基准目录；找不到的文件标记为「缺失」。
5. **导出**：结果出来后点击「导出 CSV」或「导出 SUM」；批量校验页可单独导出不一致项。

## 性能实测报告 / Performance Report

测试硬件 / Test hardware:

- CPU: AMD Ryzen 9 9950X 16-Core
- 磁盘 / Disk: TOPMORE Dubhe NVMe SSD
- 内存 / RAM: 96 GB
- 系统 / OS: Windows 11 专业工作站版
- Go 1.26.6 windows/amd64（标准库 SHA-256/SHA-512 走 SHA-NI/AVX2 汇编路径）

`go test -run='^$' -bench=. -benchtime=1x ./internal/hashcore/` 实测输出：

```
goos: windows
goarch: amd64
pkg: gohash/internal/hashcore
cpu: AMD Ryzen 9 9950X 16-Core Processor
BenchmarkSHA256LargeFile-32       1   417757600 ns/op    1074 MB_total
BenchmarkSHA256MD5LargeFile-32    1  1327528200 ns/op    1074 MB_total
BenchmarkManySmallFiles-32        1    95877600 ns/op    10.24 MB_total
```

| 基准项 | 目标 | 实测 | 达标 |
| --- | --- | --- | --- |
| SHA-256 单文件 1GB | ≤ 3 s | **0.42 s**（约 2.5 GB/s） | ✅ |
| SHA-256 + MD5 双算法同扫 1GB | ≤ 3.5 s | **1.33 s** | ✅ |
| 1 万个 1KB 小文件批量 | ≤ 10 s | **0.10 s** | ✅ |
| 冷启动到可交互 | < 2 s | **约 0.42–0.51 s**（脚本实测窗口出现耗时） | ✅ |
| 内存占用 | RSS 增量 < 100 MB | 流式 + 池化缓冲，任意大小文件内存恒定；启动后 RSS ≈ 33 MB | ✅ |

设计取舍（详见源码注释）：流式读取 + 自适应缓冲（<64MB 用 1MB、大文件 16MB，sync.Pool 复用）；大文件双缓冲流水线（prefetch goroutine 预读、IO 与哈希计算重叠）；多算法 io.MultiWriter 一次扫描；小文件 worker pool 并发 min(NumCPU, 8)（小文件 IO 调度开销占比高，并发吃满 CPU 收益大；大文件瓶颈在磁盘带宽与 SHA 计算，单流水线即可打满 NVMe）；进度上报 200ms 节流、表格数据分批 ≤500 条推送。

## 从零构建 / Build from Scratch

前置条件 / Prerequisites:

- Go 1.22+、Node.js 18+
- Wails CLI v2：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- MSYS2 的 MinGW-w64 GCC（UCRT64 环境）：
  `pacman -S mingw-w64-ucrt-x86_64-gcc`，确保 `C:\msys64\ucrt64\bin` 在 PATH 中（**不要用 TDM-GCC 或 MSYS 环境的 gcc**）
- `wails doctor` 全部通过

构建步骤 / Build steps:

```bash
# 1. 安装前端依赖 / install frontend deps
cd frontend && npm install && cd ..

# 2. 构建单文件 exe（Windows GUI 模式，无控制台窗口）/ build single-file exe
wails build -ldflags "-H windowsgui"

# 产物 / output: build/bin/gohash.exe

# 3. 可选：UPX 压缩体积 / optional: compress with upx
upx --best build/bin/gohash.exe
```

开发模式 / Development:

```bash
wails dev                 # 热重载开发 / live-reload dev
go test ./...             # 单元测试 + 集成测试（含根包绑定层纯逻辑）/ unit & integration tests
go vet ./... && gofmt -l .   # 静态检查与格式 / lint & format
cd frontend && npm run test  # 前端纯函数单元测试（vitest）/ frontend unit tests
cd frontend && npm run build # 前端类型检查与构建 / frontend type-check & build
```

冒烟脚本 / Smoke script: `powershell -ExecutionPolicy Bypass -File tools/smoke-test.ps1`（先跑 `gohash.exe --selftest` 无界面功能自检并校验退出码，再测启动耗时与 RSS、截图）。

## 项目结构 / Project Structure

```
main.go                  Wails 入口（窗口选项、原生文件拖拽、--selftest 分支）
app.go                   后端绑定方法、任务管理、进度事件（200ms 节流）
selftest.go              --selftest 无界面自检：已知值 / SUM 闭环 / 清单解析，退出码报告
internal/hashcore/       哈希核心：流式引擎、双缓冲流水线、worker pool（纯 Go，可独立测试）
internal/checksum/       清单解析、算法识别、对比、CSV/SUM 导出（纯 Go）
frontend/src/            Vue 3 + TypeScript + Naive UI + vue-i18n
  views/                 四个功能页（hash / verify / compare / batch）
  components/            algo-chips / result-table（虚拟滚动、自适应高度）/ progress-panel
  stores/                pinia 设置（主题、语言，localStorage 持久化）
  locales/               zh-cn.json / en-us.json
  api/                   Wails 绑定封装与事件会话（跨任务事件按 taskId 序号过滤）
tools/smoke-test.ps1     冒烟测试脚本（selftest 退出码 / 启动耗时 / RSS / 截图）
```

## 可靠性设计 / Reliability

- 后端错误全部结构化返回（错误码 + 中英双语信息），前端统一 toast；无 panic、无静默吞错。任务 goroutine 有 recover 兜底，异常时照常发出带 `fatal` 标记的完成事件，前端不会卡在运行态。
- 文件被占用 / 无权限 / 已删除单独标记状态，不中断整批任务。
- `context.Context` 贯穿所有任务，取消 1 秒内生效。
- 已完成任务结果仅保留最近 4 个供导出复用，超出自动淘汰，长跑内存不膨胀。
- 导出先写临时文件再 rename（原子替换），失败不留半截文件；exe 清单声明 `longPathAware`，系统启用长路径策略后可处理超 260 字符路径。
- 哈希比较忽略大小写；清单解析兼容 `*文件名` 二进制标记、行首 `\` 转义、文件名含空格、UTF-8 BOM、CRLF/LF 混用；清单重复条目按路径去重（首条胜出）。
- 单元测试覆盖哈希核心（已知值 / 去重 / 取消 / 错误分类）、清单解析与算法识别、绑定层纯逻辑；集成测试覆盖 SUM 导出 → 重新导入校验闭环；前端 vitest 覆盖格式化与事件过滤纯函数。
