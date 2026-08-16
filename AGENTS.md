# AGENTS.md

本文件面向 AI 编码代理，介绍本项目的架构、构建、测试与开发约定。阅读前不需要任何项目背景知识。

## 项目概述

**GoHashTool（文件哈希工具 / File Hash Tool）**：Windows 64 位桌面工具，用于文件哈希值的获取与对比。技术栈为 **Wails v2 + Go + Vue 3 + TypeScript**，产物是单文件 exe（`build/bin/gohash.exe`）。

四大功能页：

- **哈希计算（hash）**：多选文件、文件夹递归、整窗拖拽添加；MD5 / SHA-1 / SHA-256 / SHA-512 / CRC32 多算法一次扫描；结果表格虚拟滚动（十万级行数）；点击哈希值复制。
- **单文件校验（verify）**：粘贴期望哈希，按长度自动识别算法（32=MD5、40=SHA-1、64=SHA-256、128=SHA-512），给出「一致 / 不一致」结论。
- **双文件对比（compare）**：选两个文件同算法对比，并排展示。
- **批量校验（batch）**：导入 md5sum/sha256sum 标准清单（.sha256/.sha1/.sha512/.md5/.txt/.sum/.sums，支持 `#` 注释行），算法识别为扩展名与哈希长度交叉校验（冲突即报 `ext_algo_mismatch`），无已知扩展名时按长度推断；基准目录可切换；输出 通过/失败/缺失 统计与明细；一键导出不一致项。

导出：CSV（带 UTF-8 BOM，Excel 直开不乱码）与标准 SUM 格式；导出的 SUM 可被批量校验重新导入（闭环，有集成测试保障）。UI 支持浅色/深色主题（默认跟随系统）、中英双语即时切换（默认跟随系统语言）。

## 技术栈与关键配置

- `go.mod` / `go.sum`：Go module 名为 `gohash`，`go 1.25.0`，唯一直接依赖 `github.com/wailsapp/wails/v2 v2.14.0`。文件末尾有一条**注释掉的 `replace` 指令**（指向开发者本机模块缓存路径），保持注释状态，不要启用。
- `wails.json`：Wails 项目配置，输出文件名 `gohash`，前端安装/构建/开发命令分别映射到 `npm install` / `npm run build` / `npm run dev`。
- `frontend/package.json`：依赖 Vue 3.5、Naive UI、vue-i18n、vue-router（hash 模式）、pinia、@vicons/ionicons5；`npm run build` = `vue-tsc --noEmit && vite build`（类型检查 + 构建一起完成，`vue-tsc` 同时检查 `src/**/*.test.ts`）；`npm run test` = `vitest run`（纯函数单测）。无独立 lint 配置。
- `frontend/tsconfig.json`：`strict: true`，`resolveJsonModule: true`（locales 的 JSON 直接 import）。
- `frontend/vite.config.ts`：仅 `@vitejs/plugin-vue`，无额外配置。
- `frontend/package.json.md5`：Wails 生成的依赖变更检测文件，不要手动编辑。
- `build/`：Wails 构建资源（`appicon.png`、`windows/` 下的图标与清单）。`darwin/` 是脚手架默认目录，本项目目标平台只有 Windows 64 位。
- `frontend/wailsjs/`：Wails 自动生成的绑定代码（`go/main/App.js/.d.ts` 与 `runtime/`），由 `wails dev` / `wails build` 重新生成，**不要手改**。

## 构建与开发命令

前置条件：Go 1.22+、Node.js 18+、Wails CLI v2（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）、MSYS2 MinGW-w64 GCC（UCRT64 环境：`pacman -S mingw-w64-ucrt-x86_64-gcc`，确保 `C:\msys64\ucrt64\bin` 在 PATH 中；**不要用 TDM-GCC 或 MSYS 环境的 gcc**）。`wails doctor` 应全部通过。

```bash
# 安装前端依赖
cd frontend && npm install && cd ..

# 构建单文件 exe（Windows GUI 模式，无控制台窗口），产物：build/bin/gohash.exe
wails build -ldflags "-H windowsgui"

# 可选：UPX 压缩体积
upx --best build/bin/gohash.exe

# 热重载开发
wails dev

# 单元测试 + 集成测试（根包 gohash 含绑定层纯逻辑测试 app_test.go）
go test ./...

# 竞态检测（回调并发路径必跑；需要 CGO/gcc，勿加 purego tag）
go test -race ./...

# 静态检查与格式（应均无输出）
go vet ./... && gofmt -l .

# 前端纯函数单元测试（vitest）
cd frontend && npm run test

# 前端类型检查与构建
cd frontend && npm run build

# 性能基准（会创建 1GB 大文件 + 1 万个 1KB 小文件夹具，结束后由 TestMain 清理）
go test -run='^$' -bench=. -benchtime=1x ./internal/hashcore/

# 冒烟测试：先跑 gohash.exe --selftest（无界面功能自检，验退出码），再测启动耗时与 RSS、截图到 docs/screenshot-main.png
powershell -ExecutionPolicy Bypass -File tools/smoke-test.ps1
```

性能设计上依赖 Go 标准库 crypto/sha256、crypto/sha512 在 amd64 上的 SHA-NI / AVX2 汇编路径——构建时**不要加 `purego` 等禁用汇编的 tag**。

## 代码结构

```
main.go                  Wails 入口：窗口选项（1280x800，最小 960x640），
                         //go:embed all:frontend/dist，原生文件拖拽（EnableFileDrop，
                         悬停高亮靠 wails-drop-target-active class + style.css），
                         检测到 --selftest 参数时不开 GUI 转自检
app.go                   后端绑定方法、任务管理、进度事件（200ms 节流、结果行单次 ≤500 条推送）
selftest.go              --selftest 无界面核心自检（已知值/SUM 闭环/清单解析），退出码报告；
                         selftest_windows.go 用 AttachConsole 让 windowsgui 构建也能打印
internal/hashcore/       哈希核心：流式引擎、双缓冲流水线、worker pool（纯 Go，不依赖 Wails，可独立测试）
internal/checksum/       清单解析、算法识别、对比、CSV/SUM 导出（纯 Go，不依赖 Wails）
frontend/src/            Vue 3 + TypeScript + Naive UI + vue-i18n
  views/                 四个功能页：hash-view / verify-view / compare-view / batch-view
  components/            algo-chips / result-table（虚拟滚动，ResizeObserver 自适应高度，
                         工具栏支持路径筛选与「仅看问题行」）/ progress-panel
  stores/settings.ts     pinia 设置：主题与语言，localStorage 持久化（键 gohash.theme / gohash.locale）
  locales/               zh-cn.json / en-us.json + index.ts（默认语言检测）
  api/index.ts           Wails 绑定封装与事件会话（前端与后端交互的唯一入口）
  router/index.ts        hash 历史模式（桌面环境无服务端路由）
tools/smoke-test.ps1     冒烟测试脚本（selftest 退出码 / 启动耗时 / RSS / 截图）
```

### 后端分层

- **`internal/hashcore`**（纯 Go）：`Algorithm` 类型与五种算法常量；`ParseAlgorithm`（不区分大小写、允许连字符）；`DetectByLength`；`CanonicalKey`（路径去重键：Clean 规范化 + Windows 大小写折叠，ExpandPaths 与 checksum.ResolveTargets 共用）；`ExpandPaths`（目录递归、跳过非常规文件、已删除文件以 Size=-1 保留、按 CanonicalKey 去重——重叠的文件夹/文件选择只算一次；遍历出错的处理经 `walkErrAction`：目录不可读跳过子树、文件级错误只跳过该文件）；`HashFiles` 引擎——
  - 流式读取 + 自适应缓冲：<64MB 用 1MB 缓冲、大文件 16MB 缓冲，全部经 `sync.Pool`（存 `*[]byte` 避免装箱）复用，任意大小文件内存恒定；
  - 大文件（≥64MB）走单 worker 双缓冲流水线（prefetch goroutine 预读，IO 与哈希计算重叠）；小文件走 worker pool 并发 `min(NumCPU, 8)`，派发通道带缓冲（workers*4）减少十万级文件的交接阻塞；
  - 多算法经 `io.MultiWriter` 一次扫描；
  - 错误分类为状态码：`ok / canceled / not_found / no_permission / occupied / error`（Windows 的 ERROR_SHARING_VIOLATION(32) 与 ERROR_LOCK_VIOLATION(33) 用数值判断，归类为 occupied）。
- **`internal/checksum`**（纯 Go）：`ParseManifest` 解析 md5sum/sha256sum 标准格式，兼容 `*文件名` 二进制标记、GNU 行首 `\` 转义、文件名含空格、UTF-8 BOM、CRLF/LF 混用、`#` 注释行（直接跳过）；`DetectAlgorithm`（已知扩展名与哈希长度**交叉校验**：冲突报 `ext_algo_mismatch` 并给行号；无已知扩展名时按长度推断，长度混杂报 `mixed_length`）；`ResolveTargets`（条目 → 待算/缺失/期望值映射，相对路径按基准目录解析，按 CanonicalKey 去重且首条胜出）；`EqualHash`（忽略大小写）；`WriteSUM`（与解析互逆，闭环）与 `WriteCSV`（UTF-8 BOM，校验场景附加 expected/actual/verdict 三列）；`Error` 结构携带 Code 与行号。
- **`app.go`**（Wails 绑定层）：所有绑定方法返回统一的 `Result`（`ok` + `error` + 数据字段）；`StartHashTask` / `StartVerifyTask` 异步启动任务并立即返回 `taskId`（`t<N>` 序号单调递增，前端据此过滤跨任务残余事件），之后通过三个事件推送：`hash:progress`（200ms 节流）、`hash:items`（单次 ≤500 条）、`hash:done`；`CancelTask` 取消（1 秒内生效）；`ExportCSV` / `ExportSUM` 复用任务结束后保留在 `taskState.items` 中的结果，写出经 `writeExport` 先写临时文件再 rename（原子替换）；`ExportSUM` 先经 `exportableSUMCount` 预检，无可写行时返回 `no_data` 而非产出空清单；`runTask` 收尾统一在 defer 完成（recover 兜底 panic → `hash:done` 带 `fatal` 字段；发 `hash:done` 前先停并等待进度 ticker goroutine 退出再冲刷结果行，保证 done 之后不再有事件），已完成任务仅保留最近 4 个（`maxFinishedTasks`），超出淘汰最旧者。

### 前端约定

- **`frontend/src/api/index.ts` 是前端与 Go 后端交互的唯一入口**（文件头注释明确规定：视图不直接引用 wailsjs）。其中的 `Result` / `Item` / `ProgressEvent` / `ItemsEvent` / `DoneEvent` 接口与后端 JSON 契约一一对应，改后端结构体时必须同步修改。
- `createTaskSession()` 封装一次任务的事件订阅（按 taskId 过滤）、结果累积与生命周期；**视图必须在 `onBeforeUnmount` 调用 `destroy()`**。退订只调 `EventsOn` 返回的单个监听取消函数，**禁止用全局 `EventsOff`**（会把其他会话的同名监听一并移除）。大列表用 `shallowRef`，避免十万级行的深度响应式开销。事件归属由 `acceptTaskEvent` 判定：starting 窗口（Start*Task 未 resolve）只接受序号大于基线的 `t<N>` 事件，防止被路由切换取消的旧任务残余事件串扰新任务。
- 整窗拖拽路由（`app.vue` 的 `OnFileDrop` → `dispatchDrop`）：恰拖入一个清单文件 → 跳转 `/batch` 并自动开始校验；否则 → 跳 `/` 哈希计算。拖拽悬停高亮由 Wails 自动给 `--wails-drop-target: drop` 元素加 `wails-drop-target-active` class 实现（见 style.css），无需 JS。
- 路由用 `createWebHashHistory`（Wails 桌面环境无服务端路由）。

## 代码风格与约定

- **注释语言为中文**；README 中英双语（中文在前）。新代码沿用这一惯例。
- 用户可见文案全部双语：后端经 `AppError{code, zh, en, detail}` 返回，前端按当前语言取 `zh`/`en`（`api/index.ts` 的 `errorText`）；界面文案走 vue-i18n 的 `locales/zh-cn.json` 与 `en-us.json`，两份 JSON 的键必须保持同步。
- 后端**禁止 panic、禁止静默吞错**：错误全部结构化返回，前端统一 toast。单个文件失败（被占用 / 无权限 / 已删除）只标记该行的 status，不中断整批任务。
- `context.Context` 贯穿所有任务，取消 1 秒内生效。
- Go 侧用标准 `gofmt` 与 `go vet`（CI 式检查：`gofmt -l .` 应为空）。
- 前端为 strict TypeScript，`npm run build` 中的 `vue-tsc --noEmit` 即类型检查；无独立 ESLint/Prettier 配置，改代码时匹配周边现有风格。
- 事件名、状态码、算法标识等契约字符串（`hash:progress`、`occupied`、`sha256` 等）前后端硬编码对应，改动需两侧同步。

## 测试

- `go test ./...`：三个包均有测试；涉及并发回调路径改动时须再跑 `go test -race ./...`（需要 CGO/gcc）。
  - 根包 `app_test.go`：绑定层纯逻辑（parseAlgos、manifestError 双语映射、countSummary、exportableSUMCount、任务淘汰），不启动 Wails。
  - `internal/hashcore/hashcore_test.go`：单元测试，含 "abc"/空文件已知值、70MB 大文件（刚过 64MB 流水线阈值）与标准库一致性、流水线与流式两条路径的取消、错误分类、ExpandPaths 去重、walkErrAction 遍历错误策略、CanonicalKey 大小写折叠等；`bench_test.go` 为性能基准（夹具：1GB 大文件 + 1 万个 1KB 小文件，跨 benchmark 复用，`TestMain` 负责清理）。**onItem 回调在多 worker goroutine 并发触发，测试中收集结果必须加锁**（曾因未加锁被 race 检测抓到）。
  - `internal/checksum/checksum_test.go`：清单解析（含 `#` 注释行）、算法识别（扩展名×长度交叉校验、mixed_length）、转义往返、CSV 校验列、ResolveTargets 去重/缺失归类/Windows 大小写折叠的单元测试；`integration_test.go` 的 `TestSUMRoundTrip` 覆盖 计算 → 导出 SUM → 重新导入解析 → 识别算法 → 重新计算 → 全部通过 的闭环。
- 新增纯逻辑优先放进 `internal/hashcore` / `internal/checksum`（不依赖 Wails，可直接单测）；`app.go` 只做绑定与编排，可拆出的纯函数（如 countSummary、exportableSUMCount）拆出以便测试。
- 前端用 vitest 跑纯函数单测（`npm run test`，覆盖 utils/format 与 api 的拖拽路由、错误文案、事件过滤、任务会话订阅退订）；视图层验证手段仍是 `vue-tsc` 类型检查 + `wails dev` 手测。
- `selftest.go` 提供 exe 级功能自检：`gohash.exe --selftest` 不开 GUI 跑已知值/SUM 闭环/清单解析，以退出码报告；`tools/smoke-test.ps1` 先验 selftest 退出码，再测启动耗时（目标 <2s）、启动后 RSS（目标增量 <100MB）、截图。

## 安全与可靠性考虑

- 纯本地桌面应用，无网络请求、无遥测；仓库中不含密钥或凭据。
- 文件 IO 全部流式处理，任何大小的文件都不会整体读入内存；已完成任务结果只保留最近 4 个（供导出），超出淘汰。
- 导出先写 `目标.tmp` 再 rename，失败不留半截文件；任务 goroutine recover 兜底，panic 时前端收到带 `fatal` 的完成事件而非卡死。
- exe 清单声明 `longPathAware`（Win10 1607+，需系统 LongPathsEnabled 策略配合），可处理超 260 字符路径。
- 清单解析对格式错误逐行给出结构化错误（含行号），哈希比较忽略大小写（`strings.EqualFold`）。
- Windows 文件占用通过 syscall errno 数值（32/33）识别，避免引入平台专属常量。
- 发布物只有 `wails build` 产出的单文件 exe（可选 UPX 压缩），无安装器、无服务端部署流程。
