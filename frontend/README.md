# frontend

Vue 3 + TypeScript + Naive UI + vue-i18n 前端（Wails v3 桌面壳内运行）。

- `npm run dev` — Vite 开发服务器（通常经根目录 `wails3 dev` 联动启动）
- `npm run build` — `vue-tsc --noEmit && vite build`（类型检查 + 产物构建到 `dist/`，由 Go 侧 `//go:embed` 打包进 exe）
- `npm run build:dev` — 开发模式构建（供 `wails3 build DEV=true` 使用）
- `npm run test` — vitest 纯函数单测（api 层对 `@wailsio/runtime` 与 `bindings/` 用 `vi.mock` 隔离）

`bindings/` 由 `wails3 generate bindings -ts` 生成，不要手改；
`src/api/index.ts` 是前端与 Go 后端交互的唯一入口。
