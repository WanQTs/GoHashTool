<div align="center">

<img src="build/appicon.png" alt="GoHashTool logo" width="128" />

# GoHashTool

**File Hash Tool — compute, verify, compare, and batch-verify against manifests**

[![Release](https://img.shields.io/github/v/release/WanQTs/GoHashTool)](https://github.com/WanQTs/GoHashTool/releases)
[![CI](https://github.com/WanQTs/GoHashTool/actions/workflows/ci.yml/badge.svg)](https://github.com/WanQTs/GoHashTool/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v3.0.0--beta.9-E03C31)](https://v3.wails.io/)
[![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11%20x64-0078D6)](https://github.com/WanQTs/GoHashTool)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[中文](README.md) · **English**

[✨ Features](#-features) · [📸 Screenshots](#-screenshots) · [📥 Download](#-download) · [🚀 Usage](#-usage) · [⚡ Performance](#-performance) · [🛠️ Build](#-build-from-scratch)

</div>

---

A Windows 64-bit desktop tool to compute, verify and compare file hashes. Built with **Wails v3 + Go + Vue 3 + TypeScript** — fully local, no network requests, no telemetry — and delivered as a **single-file exe**.

## ✨ Features

- **Hash**: multi-select files, recursive folders, drop files anywhere in the window; MD5 / SHA-1 / SHA-256 / SHA-512 / CRC32 in a single scan; virtual-scrolled result table (handles 100k+ rows); click a hash to copy it. Unreadable subdirectories produce visible "no permission" rows — nothing is skipped silently.
- **Verify**: paste an expected hash; the algorithm is auto-detected by length (32=MD5, 40=SHA-1, 64=SHA-256, 128=SHA-512) and a large banner states "Identical / Different".
- **Compare**: pick two files and compare them side by side across the selected algorithms.
- **Batch Verify**: import standard md5sum/sha256sum manifests (.sha256/.sha1/.sha512/.md5/.txt/.sum/.sums, `#` comment lines supported); the algorithm is cross-validated by file extension and hash length; switchable base directory; per-file verdicts grouped as passed / failed / missing / unreadable, with one-click export of problem rows.
- **Export**: CSV (UTF-8 BOM, opens cleanly in Excel) and standard SUM format. Exported SUM files can be re-imported by Batch Verify (round-trip covered by integration tests). CRC32 is excluded from SUM export because it cannot be re-imported.
- **UI**: Mica window material (Windows 11, automatic fallback on older systems), light/dark theme (follows system by default), instant Chinese/English switching (follows system language), 150–250ms transitions, empty-state guidance.

## 📸 Screenshots

<div align="center">

<img src="docs/screenshot-main.png" alt="Main window" width="840" />

</div>

## 📥 Download

Grab `GoHashTool-*.exe` from the [**Releases**](https://github.com/WanQTs/GoHashTool/releases) page — a portable single file, no installation needed (a smaller UPX-compressed build is also attached).

Requires 64-bit Windows 10/11 and the WebView2 Runtime (preinstalled on Windows 11).

## 🚀 Usage

1. **Hash**: click "Select Files / Select Folder", or drop files/folders anywhere in the window; pick algorithms (SHA-256 by default) and press Start. Live progress bar, current file, elapsed time and speed are shown; cancel anytime.
2. **Verify**: select a file and paste the expected hash — the algorithm is detected automatically and the verdict is shown when done.
3. **Compare**: select two files and press Start.
4. **Batch Verify**: select a manifest (or drop one into the window — the app jumps to the page and starts automatically). Relative paths resolve against the manifest directory by default; a custom base directory can be set.
5. **Export**: once results are in, use "Export CSV" / "Export SUM"; the Batch page can export only the problem rows.

## ⚡ Performance

Test hardware:

- CPU: AMD Ryzen 9 9950X 16-Core · Disk: TOPMORE Dubhe NVMe SSD · RAM: 96 GB
- OS: Windows 11 Pro for Workstations · Go 1.26.6 (stdlib SHA-256/SHA-512 use the SHA-NI/AVX2 assembly paths)

Output of `go test -run='^$' -bench=. -benchtime=1x ./internal/hashcore/`:

```
goos: windows
goarch: amd64
pkg: gohash/internal/hashcore
cpu: AMD Ryzen 9 9950X 16-Core Processor
BenchmarkSHA256LargeFile-32       1   416024500 ns/op    1074 MB_total
BenchmarkSHA256MD5LargeFile-32    1  1324298300 ns/op    1074 MB_total
BenchmarkManySmallFiles-32        1    84642400 ns/op    10.24 MB_total
```

| Benchmark | Target | Measured | Pass |
| --- | --- | --- | --- |
| SHA-256, single 1GB file | ≤ 3 s | **0.42 s** (~2.5 GB/s) | ✅ |
| SHA-256 + MD5, same 1GB scan | ≤ 3.5 s | **1.32 s** | ✅ |
| 10,000 × 1KB small files | ≤ 10 s | **0.08 s** | ✅ |
| Cold start to interactive | < 2 s | **~0.45 s** (smoke script, time to window) | ✅ |
| Memory | RSS delta < 100 MB | Streaming + pooled buffers keep memory flat for any file size; RSS ≈ 33 MB after launch | ✅ |

> Benchmark fixtures are reused within a single process, so later rounds may hit the OS page cache — these are **warm-cache** numbers, not cold-disk first reads.

Design trade-offs (see source comments): streaming reads with adaptive buffers (1MB below 64MB, 16MB above, recycled via `sync.Pool`); a double-buffered pipeline for large files (prefetch overlaps IO with hashing); multiple algorithms in one scan via `io.MultiWriter`; a worker pool of `min(NumCPU, 8)` for small files; 200ms-throttled progress events and result batches capped at 500 rows.

## 🛠️ Build from Scratch

Prerequisites:

- Go 1.25+, Node.js 20.19+ (required by Vite 7)
- Wails CLI v3: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.9`
- Optional: MSYS2 MinGW-w64 GCC (only needed for `go test -race`; the exe build itself disables CGO)
- `wails3 doctor` all green

```bash
# 1. Install frontend dependencies
cd frontend && npm install && cd ..

# 2. Build the single-file exe (production: no console window; icon/manifest/version info via syso)
wails3 build
# Output: bin/gohash.exe

# 3. Optional: compress with UPX
upx --best bin/gohash.exe
```

Development:

```bash
wails3 dev                    # live-reload dev mode
go test ./...                 # unit & integration tests
go test -race ./...           # race detector (needs GCC)
go vet ./... && gofmt -l .    # lint & format
cd frontend && npm run test   # frontend unit tests (vitest)
cd frontend && npm run build  # frontend type-check & build
wails3 generate bindings -ts  # regenerate TS bindings after Go API changes
```

Smoke script:

```powershell
powershell -ExecutionPolicy Bypass -File tools/smoke-test.ps1
# Runs bin/gohash.exe --selftest (exit-code based functional check), then measures
# startup time and RSS, and captures a screenshot.
```

## 🗂️ Project Structure

```
main.go                  Wails v3 entry (window options, Mica backdrop, file-drop forwarding, --selftest)
app.go                   Backend service methods, task management, progress events (200ms throttle)
selftest.go              Headless self-check: known values / SUM round-trip / manifest parsing
Taskfile.yml             Build orchestration entry (wails3 build wraps go-task)
build/config.yml         Project metadata & dev-mode config
build/windows/           Windows assets (icon.ico, info.json, wails.exe.manifest, Taskfile)
internal/hashcore/       Hashing core: streaming engine, double-buffered pipeline, worker pool (pure Go)
internal/checksum/       Manifest parsing, algorithm detection, compare, CSV/SUM export (pure Go)
frontend/bindings/       Generated by wails3 generate bindings -ts (do not edit)
frontend/src/            Vue 3 + TypeScript + Naive UI + vue-i18n
  views/                 Four pages (hash / verify / compare / batch)
  components/            algo-chips / result-table (virtual scroll) / progress-panel
  stores/                pinia settings (theme, locale, localStorage-persisted)
  locales/               zh-cn.json / en-us.json
  api/                   Binding wrappers & task event sessions (single entry to the backend)
tools/smoke-test.ps1     Smoke test (selftest exit code / startup time / RSS / screenshot)
```

## 🛡️ Reliability

- All backend errors are structured (error code + bilingual messages) and toasted in the UI — no panics, no silent swallowing. Task goroutines are recover-guarded; a panic still emits a `fatal` completion event instead of hanging the UI.
- Occupied / no-permission / deleted files are flagged per row without aborting the batch; unreadable subtrees become visible error rows.
- `context.Context` flows through every task; cancellation takes effect within 1 second, and cancel handles are registered atomically with the task.
- Only the last 4 finished task results are retained for re-export; exports write a temp file then rename (atomic replace).
- The exe manifest declares `longPathAware` (paths beyond 260 chars when the system policy allows).
- Hash comparison is case-insensitive; the manifest parser tolerates `*filename` binary markers, GNU `\` line escapes, filenames with spaces, UTF-8 BOM, mixed CRLF/LF and `#` comments; duplicate entries are deduped by path (first wins).
- Testing: Go unit tests + race detector + SUM round-trip integration test + frontend vitest + exe-level `--selftest` and smoke script.

---

<div align="center">

**If this little tool helps you, a ⭐ star is appreciated!**

[Releases](https://github.com/WanQTs/GoHashTool/releases) · [MIT License](LICENSE) © 2026 WanQTs · Legacy: [v2 branch](https://github.com/WanQTs/GoHashTool/tree/v2) (Wails v2 archive)

</div>
