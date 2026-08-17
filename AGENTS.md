# AGENTS.md

本文件面向 AI 编码代理，介绍本项目的架构、构建、测试与开发约定。阅读前不需要任何项目背景知识。

## 项目概述

**GoHashTool（文件哈希工具 / File Hash Tool）**：Windows 64 位桌面工具，用于文件哈希值的获取与对比。技术栈为 **Wails v3（v3.0.0-beta.9）+ Go + Vue 3 + TypeScript**，产物是单文件 exe（`bin/gohash.exe`）。

四大功能页：

- **哈希计算（hash）**：多选文件、文件夹递归、整窗拖拽添加；MD5 / SHA-1 / SHA-256 / SHA-512 / CRC32 多算法一次扫描；结果表格虚拟滚动（十万级行数）；点击哈希值复制。遍历中不可读的子目录会生成为「无权限」结果行，不静默漏算。
- **单文件校验（verify）**：粘贴期望哈希，按长度自动识别算法（8=CRC32、32=MD5、40=SHA-1、64=SHA-256、128=SHA-512），给出「一致 / 不一致」结论。
- **双文件对比（compare）**：选两个文件同算法对比，并排展示。
- **批量校验（batch）**：导入 md5sum/sha256sum 标准清单（.sha256/.sha1/.sha512/.md5/.txt/.sum/.sums，支持 `#` 注释行），算法识别为扩展名与哈希长度交叉校验（冲突即报 `ext_algo_mismatch`），无已知扩展名时按长度推断；基准目录可切换；输出 通过/不一致/缺失/无法读取 四类统计与明细（占用/无权限等「存在但读不了」结论为 `error`，**不计入缺失**）；一键导出问题项。

导出：CSV（带 UTF-8 BOM，Excel 直开不乱码）与标准 SUM 格式（仅 MD5/SHA-1/SHA-256/SHA-512；**CRC32 不提供 SUM 导出**——其 8 位摘要不在清单识别范围内，导出后无法重新导入校验，后端以 `algo_not_exportable` 拒绝）；导出的 SUM 可被批量校验重新导入（闭环，有集成测试保障）。UI 为 Mica 云母窗口材质（Win11，低版本系统自动回退模糊/实色）、浅色/深色主题（默认跟随系统）、中英双语即时切换（默认跟随系统语言，无法判断时回退中文）。

系统集成（Wails v3 特性）：**单实例**（`SingleInstance`，重复启动聚焦已有窗口并把二实例参数转交前端）；**清单文件关联**（`FileAssociations` + `ApplicationOpenedWithFile`，双击 .sha256/.md5 等清单直接打开批量校验；注册表写 HKCU\Software\Classes 免管理员，已被其他程序占用的扩展名跳过不劫持，.txt 只识别不注册）；**任务完成提醒**（`hash:done` 时窗口不在前台则 `Flash(true)` 任务栏闪烁，TIMERNOFG 回前台自动停止）；**窗口置顶**（顶栏图钉 → `SetAlwaysOnTop`，会话级不持久化）；**结果行原生右键菜单**（前端在 tr 上声明 `--custom-contextmenu`/`--custom-contextmenu-data` CSS 变量，Go 侧注册 `result-row` 菜单：复制哈希/复制路径/在资源管理器中显示；动作反馈经 `context:copied`/`context:error` 事件回前端 toast）。

## 技术栈与关键配置

- `go.mod` / `go.sum`：Go module 名为 `gohash`，`go 1.25.0`，直接依赖 `github.com/wailsapp/wails/v3 v3.0.0-beta.9` 与 `golang.org/x/sys`（文件关联注册表写入，`x/sys/windows/registry`）。文件末尾有一条**注释掉的 `replace` 指令**（指向开发者本机模块缓存路径），保持注释状态，不要启用。
- **`Taskfile.yml`（根）+ `build/Taskfile.yml` + `build/windows/Taskfile.yml`**：Wails v3 的构建编排（go-task 内嵌于 wails3 CLI，`wails3 build` 即其封装）。`wails.json` 已于 v3 迁移时删除。
- `build/config.yml`：项目元数据（`info:` 段生成 exe 版本信息/图标/清单）与 `wails3 dev` 开发模式配置。修改 `info` 后跑 `wails3 task common:update:build-assets` 再生成资产（会覆盖手工修改；`windows/wails.exe.manifest` 中手工加入的 `longPathAware` 行需在再生成后补回）。
- `frontend/package.json`：依赖 Vue 3.5、Naive UI、vue-i18n、vue-router（hash 模式）、pinia、@vicons/ionicons5、**@wailsio/runtime（与 Go 端同版本 beta.9）**；`npm run build` = `vue-tsc --noEmit && vite build`；`npm run build:dev` 供 wails3 dev 使用；`npm run test` = `vitest run`。无独立 lint 配置。
- `frontend/tsconfig.json`：`strict: true`，`resolveJsonModule: true`（locales 的 JSON 直接 import，api 层也借此取双语文案）。
- `frontend/vite.config.ts`：仅 `@vitejs/plugin-vue`，无额外配置。
- `build/`：Wails v3 构建资产（`appicon.png`、`config.yml`、`Taskfile.yml`、`windows/` 下的图标/清单/`info.json`/Taskfile）。`darwin/` 是脚手架默认目录，本项目目标平台只有 Windows 64 位。
- `frontend/bindings/`：**`wails3 generate bindings -ts` 自动生成的绑定代码**（`gohash/app.ts` 按方法 ID 调用 + `models.ts`），重新生成于构建流程或由命令手动触发，**不要手改**。v2 时代的 `frontend/wailsjs/` 已删除。
- 输出目录为项目根 `bin/`（v3 默认），不是 v2 的 `build/bin/`。

## 构建与开发命令

前置条件：Go 1.25+、Node.js 20.19+（Vite 7 要求）、Wails CLI v3（`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9`）。构建 exe 时 **CGO 默认关闭、不需要 GCC**；仅 `go test -race` 需要 MSYS2 MinGW-w64 GCC（UCRT64，`C:\msys64\ucrt64\bin` 在 PATH 中；不要用 TDM-GCC 或 MSYS 环境的 gcc）。`wails3 doctor` 应全部通过。

```bash
# 安装前端依赖
cd frontend && npm install && cd ..

# 构建单文件 exe（production：-tags production -trimpath -ldflags "-w -s -H windowsgui"
# 已内置于 build/windows/Taskfile.yml；图标/清单/版本信息经 syso 嵌入），产物：bin/gohash.exe
wails3 build

# 可选：UPX 压缩体积
upx --best bin/gohash.exe

# 热重载开发
wails3 dev

# 后端方法签名变更后重新生成前端绑定
wails3 generate bindings -ts

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

# 性能基准（会创建 1GB 大文件 + 1 万个 1KB 小文件夹具，结束后由 TestMain 清理；
# 夹具跨 benchmark 复用，首轮后命中 OS 页缓存——成绩是热缓存吞吐，README 已注明）
go test -run='^$' -bench=. -benchtime=1x ./internal/hashcore/

# 冒烟测试：先跑 bin/gohash.exe --selftest（无界面功能自检，验退出码），再测启动耗时与 RSS、截图到 docs/screenshot-main.png
powershell -ExecutionPolicy Bypass -File tools/smoke-test.ps1
```

性能设计上依赖 Go 标准库 crypto/sha256、crypto/sha512 在 amd64 上的 SHA-NI / AVX2 汇编路径——构建时**不要加 `purego` 等禁用汇编的 tag**。

## 代码结构

```
main.go                  Wails v3 入口：application.New + NewWithOptions（1280x800，最小 960x640，
                         BackgroundTypeTranslucent + BackdropType Mica），//go:embed all:frontend/dist，
                         窗口级 EnableFileDrop + OnWindowEvent(WindowFilesDropped) 转发
                         "files-dropped" 事件给前端；OnShutdown 取消全部任务；
                         检测到 --selftest 参数时不开 GUI 转自检；
                         SingleInstance（二实例参数转交首实例并聚焦窗口）+ FileAssociations 声明 +
                         OnApplicationEvent(ApplicationOpenedWithFile) 转 notifyOpenWithFile +
                         registerFileAssociations（启动时注册清单扩展名关联）
app.go                   后端服务（v3 Service）方法、任务管理、进度事件（200ms 节流、结果行单次 ≤500 条推送）；
                         hash:done 发出后窗口不在前台则 Flash(true) 任务栏闪烁提醒；
                         SetAlwaysOnTop 窗口置顶绑定
openwith.go              「打开方式」接入：清单扩展名识别（assocExts 注册用 / openWithExts 识别用，
                         后者多 .txt）、manifestArgFromArgs（挑第一个实际存在的清单参数）、
                         notifyOpenWithFile（暂存 + 广播 open-with-file 事件）、
                         ConsumePendingOpenFile（前端挂载后拉取并清空，规避启动时事件先于订阅的丢失）
contextmenu.go           结果行原生右键菜单（result-row）：SetupResultContextMenu 按前端传入的
                         双语文案（重）建菜单（启动与切换语言时调用，旧菜单同名覆盖后 Destroy）、
                         decodeRowContext（对应前端 encodeURIComponent(JSON)）、
                         复制哈希/复制路径/资源管理器显示（explorer /select，只 Start 不 Wait——
                         explorer 成功也可能返回非零退出码；文件缺失时退而打开父目录）；
                         动作反馈经 context:copied / context:error 事件回前端
fileassoc_windows.go     清单扩展名关联注册到 HKCU\Software\Classes（免管理员）：
                         已被占用的扩展名跳过不劫持（读 HKCR 合并视图判断）、
                         ProgID=GoHashTool<ext>、写完后 SHChangeNotify 刷新外壳；
                         fileassoc_other.go 为非 Windows 平台空实现（保持可交叉编译）
selftest.go              --selftest 无界面核心自检（已知值/SUM 闭环/清单解析），退出码报告；
                         selftest_windows.go 用 AttachConsole 让 windowsgui 构建也能打印
                         （注意：部分管道/伪终端环境下控制台文本可能中途截断，属终端宿主的
                         控制台挂接时序问题；自检契约是退出码，冒烟脚本只依赖退出码）
Taskfile.yml             构建编排入口（wails3 build/dev/task）
build/config.yml         项目元数据 + dev 模式配置
internal/hashcore/       哈希核心：流式引擎、双缓冲流水线、worker pool（纯 Go，不依赖 Wails，可独立测试）
internal/checksum/       清单解析、算法识别、对比、CSV/SUM 导出（纯 Go，不依赖 Wails）
frontend/bindings/       wails3 生成的 TS 绑定（勿手改）
frontend/src/            Vue 3 + TypeScript + Naive UI + vue-i18n
  views/                 四个功能页：hash-view / verify-view / compare-view / batch-view
  components/            algo-chips / result-table（虚拟滚动，ResizeObserver 自适应高度，
                         工具栏支持路径筛选与「仅看问题行」）/ progress-panel
  stores/settings.ts     pinia 设置：主题与语言，localStorage 持久化（键 gohash.theme / gohash.locale）
  locales/               zh-cn.json / en-us.json + index.ts（默认语言检测：zh→中、en→英、其余回退中文）
  api/index.ts           绑定封装与事件会话（前端与后端交互的唯一入口）
  router/index.ts        hash 历史模式（桌面环境无服务端路由）
tools/smoke-test.ps1     冒烟测试脚本（selftest 退出码 / 启动耗时 / RSS / 截图）。
                         必须保持 UTF-8 带 BOM：PS 5.1 对无 BOM 文件按 ANSI(GBK) 解码，
                         中文注释会打乱解析（曾导致 param 默认值失效）
```

### 后端分层

- **`internal/hashcore`**（纯 Go）：`Algorithm` 类型与五种算法常量；`ParseAlgorithm`（不区分大小写、允许连字符）；`DetectByLength`；`CanonicalKey`（路径去重键：Clean 规范化 + Windows 大小写折叠，ExpandPathsDetailed 与 checksum.ResolveTargets 共用）；`ExpandPathsDetailedContext`（目录递归、跳过非常规文件、已删除文件以 Size=-1 保留、按 CanonicalKey 去重——重叠的文件夹/文件选择只算一次；遍历出错经 `walkErrAction`：目录不可读跳过子树**并记录到 skippedDirs 返回**、文件级错误只跳过该文件；**ctx 取消立即终止遍历并返回错误**，`onScan` 每纳入一个文件回调一次供扫描进度上报）；`ExpandPathsDetailed` / `ExpandPaths` 为其不可取消便捷封装（后者再丢弃跳过列表，仅供不需要展示的调用方/旧测试）；`HashFiles` 引擎——
  - 流式读取 + 自适应缓冲：<64MB 用 1MB 缓冲、大文件 16MB 缓冲，全部经 `sync.Pool`（存 `*[]byte` 避免装箱）复用，任意大小文件内存恒定；
  - 大文件（≥64MB）走单 worker 双缓冲流水线（prefetch goroutine 预读，IO 与哈希计算重叠）；小文件走 worker pool 并发 `min(NumCPU, 8)`，派发通道带缓冲（workers*4）减少十万级文件的交接阻塞；
  - 多算法经 `io.MultiWriter` 一次扫描；
  - 错误分类为状态码：`ok / canceled / not_found / no_permission / occupied / error`（Windows 的 ERROR_SHARING_VIOLATION(32) 与 ERROR_LOCK_VIOLATION(33) 用数值判断，归类为 occupied）。
  - 注意：流水线读 goroutine 的纯错误分块 `bp` 为 nil，消费端仅 `bp != nil` 时才回池（nil 回池会让后续 Get 到它的计算 nil 解引用 panic）。
- **`internal/checksum`**（纯 Go）：`ParseManifest` 解析 md5sum/sha256sum 标准格式，兼容 `*文件名` 二进制标记、GNU 行首 `\` 转义、文件名含空格、UTF-8 BOM、CRLF/LF 混用、`#` 注释行（直接跳过）；`DetectAlgorithm`（已知扩展名与哈希长度**交叉校验**：冲突报 `ext_algo_mismatch` 并给行号；无已知扩展名时按长度推断，长度混杂报 `mixed_length`）；`ResolveTargets`（条目 → 待算/缺失/期望值映射，相对路径按基准目录解析，按 CanonicalKey 去重且首条胜出）；`EqualHash`（忽略大小写）；`WriteSUM`（与解析互逆，闭环）与 `WriteCSV`（UTF-8 BOM，校验场景附加 expected/actual/verdict 三列）；`Error` 结构携带 Code 与行号。
- **`app.go`**（Wails v3 服务层）：`App` 经 `application.NewService` 注册，导出方法即绑定方法（生成的 TS 按方法 ID 调用）。`attach(app)` 由 main 注入应用句柄；事件经 `a.app.Event.Emit` 推送；对话框经 `a.app.Dialog.OpenFileWithOptions/SaveFileWithOptions`（**标题由前端按当前语言传入**）；剪贴板 `a.app.Clipboard.SetText`；日志 `a.app.Logger`。所有绑定方法返回统一的 `Result`（`ok` + `error` + 数据字段）；`StartHashTask` / `StartVerifyTask` 异步启动任务并立即返回 `taskId`（`t<N>` 序号单调递增，前端据此过滤跨任务残余事件），之后通过三个事件推送：`hash:progress`（200ms 节流）、`hash:items`（单次 ≤500 条）、`hash:done`；**目录展开在任务 goroutine 内进行**（`ExpandPathsDetailedContext`，可取消），扫描期间 `hash:progress` 带 `scanning` 标记、`done` 为已发现文件数，展开完成后总量才确定并随后续事件下发；展开后零文件的任务经 `hash:done` 的 `error` 字段（`no_files`）异步报错，由前端 toast；`CancelTask` 取消（1 秒内生效）——**取消句柄随 newTask 入表同步登记**，Start 返回即点取消不会落空；`ExportCSV` / `ExportSUM` 复用任务结束后保留在 `taskState.items` 中的结果，写出经 `writeExport` 在目标目录建唯一临时文件（`os.CreateTemp`）再 rename（原子替换），全程 `exportMu` 串行（Windows 上并发 rename 覆盖同一目标会间歇性 Access denied）；`ExportSUM` 先经 `exportableSUMCount` 预检，无可写行返回 `no_data`，CRC32 返回 `algo_not_exportable`；`runTask` 收尾统一在 defer 完成（recover 兜底 panic → `hash:done` 带 `fatal` 字段；发 `hash:done` 前先停并等待进度 ticker goroutine 退出再冲刷结果行，保证 done 之后不再有事件）；结果行 flush 全程持锁（置换+发射一体），worker 与 ticker 并发触发也不会乱序；已完成任务仅保留最近 4 个（`maxFinishedTasks`），超出淘汰最旧者。批量校验单行结论由纯函数 `verdictFor` 给出：OK→pass/fail、not_found→missing、其余（occupied/no_permission/error/canceled）→**error**。

### 前端约定

- **`frontend/src/api/index.ts` 是前端与 Go 后端交互的唯一入口**（文件头注释明确规定：视图不直接引用 bindings / @wailsio/runtime）。其中的 `Result` / `Item` / `ProgressEvent` / `ItemsEvent` / `DoneEvent` 接口与后端 JSON 契约一一对应，改后端结构体时必须同步修改。单文件校验的算法长度识别 `detectAlgoByHash`（8=CRC32、32=MD5、40=SHA-1、64=SHA-256、128=SHA-512）也在此层，供视图与 vitest 复用。
- 事件订阅用 `@wailsio/runtime` 的 `Events.On(name, cb)`（事件负载在 `ev.data`），**返回单个监听的取消函数**；`createTaskSession()` 封装一次任务的事件订阅（按 taskId 过滤）、结果累积与生命周期；**视图必须在 `onBeforeUnmount` 调用 `destroy()`**。**禁止用 `Events.Off(name)`**（会把其他会话的同名监听一并移除）。大列表用 `shallowRef`。事件归属由 `acceptTaskEvent` 判定：starting 窗口只接受序号大于基线的 `t<N>` 事件，防止跨任务残余事件串扰。
- 整窗拖拽链路：`index.html` 的 `<body data-file-drop-target>` 声明落点（v3 必需，落点外的拖放会被忽略）→ 悬停高亮由 Wails 自动给该元素加 `file-drop-target-active` class（见 style.css，v2 的 `--wails-drop-target`/`wails-drop-target-active` 已废弃）→ Go 侧 `OnWindowEvent(events.Common.WindowFilesDropped)` 取绝对路径并 `Event.Emit("files-dropped", paths)` → `app.vue` 订阅后 `dispatchDrop`：恰拖入一个清单文件 → 跳转 `/batch` 并自动开始校验；否则 → 跳 `/` 哈希计算。**「打开方式」（文件关联/单实例转交）复用同一路由**：`app.vue` 订阅 `open-with-file` 事件（运行中送达）并在挂载时 `consumePendingOpenFile()` 拉取（启动时送达，前端就绪前事件可能已发出，拉取兜底），二者都走 `routePaths → dispatchDrop`。
- 结果行原生右键菜单：result-table 在 tr 的 `row-props` 上写 CSS 变量 `--custom-contextmenu: result-row` 与 `--custom-contextmenu-data`（`encodeRowContext` = encodeURIComponent(JSON{path,hashes})，CSS 自定义属性随继承传递，runtime 在事件目标上经 getComputedStyle 读取）；菜单文案由 `app.vue` 在启动与切换语言时经 `setupResultContextMenu` 重建（原生菜单文案由 Go 持有，不能热更新）；动作反馈经 api 层 `onContextFeedback` 订阅 `context:copied`/`context:error` toast。顶栏图钉置顶走 `setAlwaysOnTop`（会话级不持久化）。
- **结果快照约定**：任务启动时把本次实际使用的输入存入快照（hash 页 `usedAlgos`、verify 页 `usedExpected/usedAlgo`、compare 页 `usedAlgos/usedPaths`），结果区/导出只读快照——任务完成后用户改输入不会污染旧结果展示。
- 对话框标题不走语言包键到 Go，而是 api 层调用时用 `i18n.global.t('dialog.*')` 即时取值传入绑定参数。
- 前端自发错误（IPC 失败、任务崩溃兜底）经 `frontendError()` 从两个 locale JSON 取文案，构造与后端同构的 `{code, zh, en}` AppError。
- 路由用 `createWebHashHistory`（Wails 桌面环境无服务端路由）。

## 代码风格与约定

- **注释语言为中文**；README 中英双语（中文在前）。新代码沿用这一惯例。
- 用户可见文案全部双语：后端经 `AppError{code, zh, en, detail}` 返回，前端按当前语言取 `zh`/`en`（`api/index.ts` 的 `errorText`）；界面文案走 vue-i18n 的 `locales/zh-cn.json` 与 `en-us.json`，两份 JSON 的键必须保持同步。
- 后端**禁止 panic、禁止静默吞错**：错误全部结构化返回，前端统一 toast。单个文件失败（被占用 / 无权限 / 已删除）只标记该行的 status，不中断整批任务；不可读子目录生成可见错误行。
- `context.Context` 贯穿所有任务，取消 1 秒内生效。
- Go 侧用标准 `gofmt` 与 `go vet`（CI 式检查：`gofmt -l .` 应为空）。
- 前端为 strict TypeScript，`npm run build` 中的 `vue-tsc --noEmit` 即类型检查；无独立 ESLint/Prettier 配置，改代码时匹配周边现有风格。
- 事件名、状态码、算法标识等契约字符串（`hash:progress`、`occupied`、`sha256`、`files-dropped` 等）前后端硬编码对应，改动需两侧同步。v3 事件名允许冒号，无前缀冲突（系统事件保留 `common:`/`windows:` 等前缀）。

## 测试

- `go test ./...`：三个包均有测试；涉及并发回调路径改动时须再跑 `go test -race ./...`（需要 CGO/gcc）。
  - 根包 `app_test.go`：绑定层纯逻辑（parseAlgos、manifestError 双语映射、countSummary、exportableSUMCount、verdictFor 结论映射、newTask 取消句柄登记、CRC32 导出拒绝、writeExport 并发导出唯一临时名、任务淘汰、isOpenWithManifest/manifestArgFromArgs「打开方式」参数挑选（含 .txt 不注册断言）、decodeRowContext 右键菜单行数据解码（encodeURIComponent 等价样例）、ConsumePendingOpenFile 拉取即清空、SetAlwaysOnTop 无应用实例结构化报错），不启动 Wails。
  - `internal/hashcore/hashcore_test.go`：单元测试，含 "abc"/空文件已知值、70MB 大文件（刚过 64MB 流水线阈值）与标准库一致性、流水线与流式两条路径的取消、错误分类、ExpandPaths/ExpandPathsDetailed 去重、ExpandPathsDetailedContext 取消与扫描回调、walkErrAction 遍历错误策略、CanonicalKey 大小写折叠等；`bench_test.go` 为性能基准（夹具：1GB 大文件 + 1 万个 1KB 小文件，跨 benchmark 复用，`TestMain` 负责清理）。**onItem 回调在多 worker goroutine 并发触发，测试中收集结果必须加锁**（曾因未加锁被 race 检测抓到）。
  - `internal/checksum/checksum_test.go`：清单解析（含 `#` 注释行）、算法识别（扩展名×长度交叉校验、mixed_length）、转义往返、CSV 校验列、ResolveTargets 去重/缺失归类/Windows 大小写折叠的单元测试；`integration_test.go` 的 `TestSUMRoundTrip` 覆盖 计算 → 导出 SUM → 重新导入解析 → 识别算法 → 重新计算 → 全部通过 的闭环。
- 新增纯逻辑优先放进 `internal/hashcore` / `internal/checksum`（不依赖 Wails，可直接单测）；`app.go` 只做绑定与编排，可拆出的纯函数（如 countSummary、exportableSUMCount、verdictFor）拆出以便测试。
- 前端用 vitest 跑纯函数单测（`npm run test`，覆盖 utils/format 与 api 的拖拽路由、错误文案、事件过滤、任务会话订阅退订、算法长度识别、encodeRowContext 右键菜单行数据编码）；视图层验证手段仍是 `vue-tsc` 类型检查 + `wails3 dev` 手测。vitest 对 `@wailsio/runtime` 与生成的 bindings 用 `vi.mock` 隔离。
- `selftest.go` 提供 exe 级功能自检：`gohash.exe --selftest` 不开 GUI 跑已知值/SUM 闭环/清单解析，以退出码报告；`tools/smoke-test.ps1` 先验 selftest 退出码，再测启动耗时（目标 <2s）、启动后 RSS（目标增量 <100MB）、截图。

## 安全与可靠性考虑

- 纯本地桌面应用，无网络请求、无遥测；仓库中不含密钥或凭据。
- 文件 IO 全部流式处理，任何大小的文件都不会整体读入内存；已完成任务结果只保留最近 4 个（供导出），超出淘汰。
- 导出先写 `目标.tmp` 再 rename，失败不留半截文件；任务 goroutine recover 兜底，panic 时前端收到带 `fatal` 的完成事件而非卡死。
- exe 清单声明 `longPathAware`（Win10 1607+，需系统 LongPathsEnabled 策略配合），可处理超 260 字符路径。
- 清单解析对格式错误逐行给出结构化错误（含行号），哈希比较忽略大小写（`strings.EqualFold`）。
- Windows 文件占用通过 syscall errno 数值（32/33）识别，避免引入平台专属常量。
- 文件关联注册只写 `HKCU\Software\Classes`（免管理员、可随时在系统设置解除），写前读 HKCR 合并视图判断，已被其他程序占用的扩展名跳过不劫持；`.txt` 只参与识别、不参与注册。
- 发布物只有 `wails3 build` 产出的单文件 exe（可选 UPX 压缩），无安装器、无服务端部署流程。
- 备份说明：v2→v3 迁移前的完整源码快照在 `../GoHashTool-v2-backup/`（无 git 仓库时的回滚点，确认 v3 稳定后可删）。
