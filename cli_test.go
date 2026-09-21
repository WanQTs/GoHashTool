package main

// CLI 模式的纯逻辑测试：参数解析、GUI/CLI 分流、哈希与校验两种模式的
// 输出格式与退出码。输出全部注入 bytes.Buffer，不触碰真实控制台，不启动 Wails。

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gohash/internal/checksum"
	"gohash/internal/hashcore"
)

const (
	abcContent   = "abc"
	abcMD5       = "900150983cd24fb0d6963f7d28e17f72"
	abcSHA256    = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	helloContent = "hello"
	helloMD5     = "5d41402abc4b2a76b9719d911017c592"
)

func TestParseCLIArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantErr  bool
		hasFlags bool
		check    bool
		algos    []hashcore.Algorithm
		paths    []string
		quiet    bool
		status   bool
		help     bool
	}{
		{name: "plain-file", args: []string{"f.bin"}, paths: []string{"f.bin"}},
		{name: "short-algo", args: []string{"-a", "md5", "f"}, hasFlags: true,
			algos: []hashcore.Algorithm{hashcore.MD5}, paths: []string{"f"}},
		{name: "long-algo-eq", args: []string{"--algorithm=sha256", "f"}, hasFlags: true,
			algos: []hashcore.Algorithm{hashcore.SHA256}, paths: []string{"f"}},
		{name: "short-algo-eq", args: []string{"-a=sha1", "f"}, hasFlags: true,
			algos: []hashcore.Algorithm{hashcore.SHA1}, paths: []string{"f"}},
		{name: "algo-comma", args: []string{"-a", "md5,sha1", "f"}, hasFlags: true,
			algos: []hashcore.Algorithm{hashcore.MD5, hashcore.SHA1}, paths: []string{"f"}},
		{name: "algo-repeat-dedupe", args: []string{"-a", "md5", "-a", "MD5", "-a", "sha256", "f"}, hasFlags: true,
			algos: []hashcore.Algorithm{hashcore.MD5, hashcore.SHA256}, paths: []string{"f"}},
		{name: "flags-interspersed", args: []string{"f1", "-q", "f2"}, hasFlags: true,
			quiet: true, paths: []string{"f1", "f2"}},
		{name: "double-dash", args: []string{"--", "-a"}, paths: []string{"-a"}},
		{name: "check", args: []string{"-c", "m.sha256"}, hasFlags: true, check: true,
			paths: []string{"m.sha256"}},
		{name: "check-multiple-manifests", args: []string{"-c", "a.md5", "b.md5"}, hasFlags: true,
			check: true, paths: []string{"a.md5", "b.md5"}},
		{name: "check-flag-before-manifest", args: []string{"--check", "--status", "m.md5"}, hasFlags: true,
			check: true, status: true, paths: []string{"m.md5"}},
		{name: "help-short", args: []string{"-h"}, hasFlags: true, help: true},
		{name: "help-long", args: []string{"--help"}, hasFlags: true, help: true},
		{name: "err-unknown-long", args: []string{"--bogus"}, wantErr: true, hasFlags: true},
		{name: "err-unknown-short", args: []string{"-z"}, wantErr: true, hasFlags: true},
		{name: "err-joined-short-value", args: []string{"-amd5"}, wantErr: true, hasFlags: true},
		{name: "err-missing-value", args: []string{"-a"}, wantErr: true, hasFlags: true},
		{name: "err-unknown-algo", args: []string{"-a", "rot13", "f"}, wantErr: true, hasFlags: true},
		{name: "err-algo-with-check", args: []string{"-a", "md5", "-c", "m.md5"}, wantErr: true, hasFlags: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, hasFlags, err := parseCLIArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("args %v: expect error, got nil", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("args %v: %v", tc.args, err)
			}
			if hasFlags != tc.hasFlags {
				t.Errorf("hasFlags = %v, want %v", hasFlags, tc.hasFlags)
			}
			if opts.check != tc.check {
				t.Errorf("check = %v, want %v", opts.check, tc.check)
			}
			if len(opts.algos) != len(tc.algos) {
				t.Errorf("algos = %v, want %v", opts.algos, tc.algos)
			} else {
				for i, a := range opts.algos {
					if a != tc.algos[i] {
						t.Errorf("algos[%d] = %v, want %v", i, a, tc.algos[i])
					}
				}
			}
			if len(opts.paths) != len(tc.paths) {
				t.Errorf("paths = %v, want %v", opts.paths, tc.paths)
			} else {
				for i, p := range opts.paths {
					if p != tc.paths[i] {
						t.Errorf("paths[%d] = %q, want %q", i, p, tc.paths[i])
					}
				}
			}
			if opts.quiet != tc.quiet || opts.status != tc.status || opts.help != tc.help {
				t.Errorf("quiet/status/help = %v/%v/%v, want %v/%v/%v",
					opts.quiet, opts.status, opts.help, tc.quiet, tc.status, tc.help)
			}
		})
	}
}

func TestRouteCLI(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "list.sha256")
	txtManifest := filepath.Join(dir, "list.txt")
	plainFile := filepath.Join(dir, "a.bin")
	for _, p := range []string{manifest, txtManifest, plainFile} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name     string
		opts     cliOptions
		hasFlags bool
		wantCLI  bool
	}{
		{name: "no-args", opts: cliOptions{}, wantCLI: false},
		{name: "single-existing-manifest", opts: cliOptions{paths: []string{manifest}}, wantCLI: false},
		{name: "single-existing-txt", opts: cliOptions{paths: []string{txtManifest}}, wantCLI: false},
		{name: "single-plain-file", opts: cliOptions{paths: []string{plainFile}}, wantCLI: true},
		{name: "single-missing-manifest", opts: cliOptions{paths: []string{filepath.Join(dir, "gone.sha256")}}, wantCLI: true},
		{name: "two-manifests", opts: cliOptions{paths: []string{manifest, txtManifest}}, wantCLI: true},
		{name: "flags-force-cli", opts: cliOptions{paths: []string{manifest}}, hasFlags: true, wantCLI: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routeCLI(tc.opts, tc.hasFlags); got != tc.wantCLI {
				t.Errorf("routeCLI = %v, want %v", got, tc.wantCLI)
			}
		})
	}
}

func TestExpandGlobs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a1.bin", "a2.bin", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	got, err := expandGlobs([]string{"a?.bin", "plain.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "a1.bin" || got[1] != "a2.bin" || got[2] != "plain.txt" {
		t.Errorf("expandGlobs = %v", got)
	}

	// 无匹配保留原样，交由计算层报 not_found
	got, err = expandGlobs([]string{"*.zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "*.zzz" {
		t.Errorf("no-match expandGlobs = %v", got)
	}

	if _, err = expandGlobs([]string{"["}); err == nil {
		t.Error("expect ErrBadPattern")
	}
}

func TestRunCLIHash(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("abc.bin", abcContent)
	write("hello.bin", helloContent)
	write("sub/x.bin", abcContent)
	t.Chdir(dir)

	run := func(args ...string) (int, string, string) {
		opts, _, err := parseCLIArgs(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		var out, errOut bytes.Buffer
		code := runCLIHash(opts, &out, &errOut)
		return code, out.String(), errOut.String()
	}

	t.Run("default-algo-sha256", func(t *testing.T) {
		code, out, errOut := run("abc.bin")
		if code != 0 || errOut != "" {
			t.Fatalf("code = %d, stderr = %q", code, errOut)
		}
		want := abcSHA256 + "  abc.bin\n"
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("single-algo-md5sum-format", func(t *testing.T) {
		code, out, _ := run("-a", "md5", "abc.bin")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		want := abcMD5 + "  abc.bin\n"
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("multi-algo-labeled-lines", func(t *testing.T) {
		code, out, _ := run("-a", "md5", "-a", "sha256", "abc.bin")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		want := "MD5    " + abcMD5 + "  abc.bin\n" +
			"SHA256 " + abcSHA256 + "  abc.bin\n"
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("multiple-files-in-order", func(t *testing.T) {
		code, out, _ := run("-a", "md5", "abc.bin", "hello.bin")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		want := abcMD5 + "  abc.bin\n" + helloMD5 + "  hello.bin\n"
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("glob", func(t *testing.T) {
		code, out, _ := run("-a", "md5", "a*.bin")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		want := abcMD5 + "  abc.bin\n"
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

	t.Run("directory-recursion-gnu-escape", func(t *testing.T) {
		code, out, _ := run("-a", "md5", "sub")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		name := filepath.Join("sub", "x.bin")
		var want string
		if filepath.Separator == '\\' {
			// Windows 路径含反斜杠，按 GNU 约定行首 \ 转义（WriteSUM 行为）
			want = "\\" + abcMD5 + "  " + strings.ReplaceAll(name, "\\", "\\\\") + "\n"
		} else {
			want = abcMD5 + "  " + name + "\n"
		}
		if out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
		// 闭环：输出可被清单解析还原
		entries, err := checksum.ParseManifest([]byte(out))
		if err != nil {
			t.Fatalf("re-parse output: %v", err)
		}
		if len(entries) != 1 || entries[0].Hash != abcMD5 || entries[0].Path != name {
			t.Errorf("roundtrip entries = %+v", entries)
		}
	})

	t.Run("missing-file-exit-1", func(t *testing.T) {
		code, out, errOut := run("nope.bin")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if out != "" {
			t.Errorf("out = %q, want empty", out)
		}
		if !strings.Contains(errOut, "gohash: nope.bin: No such file or directory") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("no-input-exit-2", func(t *testing.T) {
		code, _, errOut := run()
		if code != 2 {
			t.Fatalf("code = %d, want 2", code)
		}
		if !strings.Contains(errOut, "no input files") {
			t.Errorf("stderr = %q", errOut)
		}
	})
}

// writeCheckFixture 造两个文件与一份 md5 清单（相对路径，WriteSUM 生成），返回目录。
func writeCheckFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte(abcContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte(helloContent), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err := checksum.WriteSUM(&buf, []checksum.ExportItem{
		{Path: "a.bin", Hashes: map[string]string{"md5": abcMD5}, Status: "ok"},
		{Path: "b.bin", Hashes: map[string]string{"md5": helloMD5}, Status: "ok"},
	}, hashcore.MD5)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "list.md5"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunCLICheck(t *testing.T) {
	// 注意：t.Chdir 必须注册在子测试自身的 t 上（cleanup 后于 TempDir 删除执行），
	// 否则 Windows 下「目录是进程当前目录」会导致 TempDir 清理失败。
	run := func(t *testing.T, dir string, args ...string) (int, string, string) {
		opts, _, err := parseCLIArgs(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		t.Chdir(dir)
		var out, errOut bytes.Buffer
		code := runCLICheck(opts, &out, &errOut)
		return code, out.String(), errOut.String()
	}

	t.Run("all-pass", func(t *testing.T) {
		dir := writeCheckFixture(t)
		code, out, errOut := run(t, dir, "-c", "list.md5")
		if code != 0 {
			t.Fatalf("code = %d, stderr = %q", code, errOut)
		}
		if out != "a.bin: OK\nb.bin: OK\n" {
			t.Errorf("out = %q", out)
		}
		if errOut != "" {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		dir := writeCheckFixture(t)
		if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("tampered"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := run(t, dir, "-c", "list.md5")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if out != "a.bin: OK\nb.bin: FAILED\n" {
			t.Errorf("out = %q", out)
		}
		if !strings.Contains(errOut, "WARNING: 1 computed checksum did NOT match") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("missing-file", func(t *testing.T) {
		dir := writeCheckFixture(t)
		if err := os.Remove(filepath.Join(dir, "a.bin")); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := run(t, dir, "-c", "list.md5")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(out, "a.bin: FAILED open or read\n") || !strings.Contains(out, "b.bin: OK\n") {
			t.Errorf("out = %q", out)
		}
		if !strings.Contains(errOut, "gohash: a.bin: No such file or directory") {
			t.Errorf("stderr = %q", errOut)
		}
		if !strings.Contains(errOut, "WARNING: 1 listed file could not be read") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("quiet-hides-ok", func(t *testing.T) {
		dir := writeCheckFixture(t)
		if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("tampered"), 0o644); err != nil {
			t.Fatal(err)
		}
		// GNU 语序：-c 为布尔开关，清单在选项之后（回归：-c 曾误吞下一参数为清单路径）
		code, out, _ := run(t, dir, "-c", "--quiet", "list.md5")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if out != "b.bin: FAILED\n" {
			t.Errorf("out = %q", out)
		}
	})

	t.Run("status-silent", func(t *testing.T) {
		dir := writeCheckFixture(t)
		if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("tampered"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := run(t, dir, "-c", "list.md5", "--status")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if out != "" || errOut != "" {
			t.Errorf("out = %q, stderr = %q, want both empty", out, errOut)
		}
	})

	t.Run("unreadable-manifest-exit-2", func(t *testing.T) {
		dir := writeCheckFixture(t)
		code, _, errOut := run(t, dir, "-c", "gone.md5")
		if code != 2 {
			t.Fatalf("code = %d, want 2", code)
		}
		if !strings.Contains(errOut, "gohash: gone.md5: No such file or directory") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("bad-manifest-exit-2", func(t *testing.T) {
		dir := writeCheckFixture(t)
		if err := os.WriteFile(filepath.Join(dir, "bad.md5"), []byte("not-a-manifest\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := run(t, dir, "-c", "bad.md5")
		if code != 2 {
			t.Fatalf("code = %d, want 2", code)
		}
		if !strings.Contains(errOut, "bad_line") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("ext-algo-mismatch-exit-2", func(t *testing.T) {
		dir := writeCheckFixture(t)
		// md5 长度的哈希写进 .sha256 清单 → 扩展名与长度交叉校验失败
		if err := os.Rename(filepath.Join(dir, "list.md5"), filepath.Join(dir, "list.sha256")); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := run(t, dir, "-c", "list.sha256")
		if code != 2 {
			t.Fatalf("code = %d, want 2", code)
		}
		if !strings.Contains(errOut, "ext_algo_mismatch") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	t.Run("multiple-manifests", func(t *testing.T) {
		dir := writeCheckFixture(t)
		// 第二份清单同内容（拷一份改个名）
		data, err := os.ReadFile(filepath.Join(dir, "list.md5"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "list2.md5"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, _ := run(t, dir, "-c", "list.md5", "list2.md5")
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
		if out != "a.bin: OK\nb.bin: OK\na.bin: OK\nb.bin: OK\n" {
			t.Errorf("out = %q", out)
		}
	})
}
