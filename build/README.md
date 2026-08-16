# Build Directory

本目录存放 Wails v3 的构建资产与编排文件（本项目目标平台仅 Windows 64 位）：

- `config.yml` — 项目元数据（公司/产品名/版本）与 `wails3 dev` 的开发模式配置。
  修改 `info` 后运行 `wails3 task common:update:build-assets` 重新生成资产（会覆盖手工修改）。
- `Taskfile.yml` — 通用构建任务（前端依赖/构建、绑定生成、图标）。
- `appicon.png` — 应用图标源图，`wails3 generate icons` 据此生成 `windows/icon.ico`。
- `windows/` — Windows 资产：`icon.ico`、`info.json`（exe 版本信息）、
  `wails.exe.manifest`（DPI / longPathAware 声明）、`Taskfile.yml`（syso + go build + 输出 bin/）。
- `darwin/` — macOS 资产（Wails 脚手架默认目录，本项目不构建 macOS 产物）。

构建产物输出到**项目根目录的 `bin/`**（`bin/gohash.exe`），不再输出到本目录。
