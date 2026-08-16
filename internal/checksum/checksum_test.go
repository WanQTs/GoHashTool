package checksum

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gohash/internal/hashcore"
)

func TestParseManifestStandard(t *testing.T) {
	content := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  hello.txt\n" +
		"900150983cd24fb0d6963f7d28e17f72 *bin.dat\n"
	entries, err := ParseManifest([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expect 2 entries, got %d", len(entries))
	}
	if entries[0].Path != "hello.txt" || entries[0].Line != 1 {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[1].Path != "bin.dat" || entries[1].Hash != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("entry1 = %+v", entries[1])
	}
}

func TestParseManifestCompat(t *testing.T) {
	t.Run("BOM+CRLF", func(t *testing.T) {
		data := []byte{0xEF, 0xBB, 0xBF}
		data = append(data, []byte("900150983cd24fb0d6963f7d28e17f72  a.txt\r\n")...)
		data = append(data, []byte("900150983cd24fb0d6963f7d28e17f72  b.txt\n")...)
		entries, err := ParseManifest(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].Path != "a.txt" || entries[1].Path != "b.txt" {
			t.Errorf("entries = %+v", entries)
		}
	})

	t.Run("filename with spaces", func(t *testing.T) {
		entries, err := ParseManifest([]byte("900150983cd24fb0d6963f7d28e17f72  my file name.txt\n"))
		if err != nil {
			t.Fatal(err)
		}
		if entries[0].Path != "my file name.txt" {
			t.Errorf("path = %q", entries[0].Path)
		}
	})

	t.Run("escaped line", func(t *testing.T) {
		entries, err := ParseManifest([]byte("\\900150983cd24fb0d6963f7d28e17f72  a\\\\b.txt\n"))
		if err != nil {
			t.Fatal(err)
		}
		if entries[0].Path != `a\b.txt` {
			t.Errorf("path = %q", entries[0].Path)
		}
	})

	t.Run("bad line reports line number", func(t *testing.T) {
		_, err := ParseManifest([]byte("900150983cd24fb0d6963f7d28e17f72  a.txt\nnot-a-hash-line\n"))
		pe, ok := err.(*Error)
		if !ok || pe.Line != 2 {
			t.Errorf("err = %v, want *Error with Line=2", err)
		}
	})

	t.Run("empty manifest", func(t *testing.T) {
		_, err := ParseManifest([]byte("\n\n"))
		if pe, ok := err.(*Error); !ok || pe.Code != "empty_manifest" {
			t.Errorf("err = %v, want empty_manifest", err)
		}
	})
}

func TestDetectAlgorithm(t *testing.T) {
	entry := func(hash string, line int) Entry { return Entry{Hash: hash, Line: line} }
	md5Entry := entry("900150983cd24fb0d6963f7d28e17f72", 1)
	sha1Entry := entry("a9993e364706816aba3e25717850c26c9cd0d89d", 2)
	sha256Entry := entry("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", 3)

	t.Run("extension consistent with length", func(t *testing.T) {
		// 扩展名与哈希长度一致时按扩展名识别（大小写不敏感）。
		got, err := DetectAlgorithm("sums.SHA256", []Entry{sha256Entry})
		if err != nil || got != hashcore.SHA256 {
			t.Errorf("got %v, %v", got, err)
		}
		got, err = DetectAlgorithm("checksums.MD5", []Entry{md5Entry})
		if err != nil || got != hashcore.MD5 {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("extension/length mismatch reports line", func(t *testing.T) {
		// 扩展名所示算法与哈希长度不符（多半改错了后缀名）：显式报错而非算完全部不一致。
		_, err := DetectAlgorithm("sums.sha256", []Entry{md5Entry})
		pe, ok := err.(*Error)
		if !ok || pe.Code != "ext_algo_mismatch" || pe.Line != 1 {
			t.Errorf("err = %v, want ext_algo_mismatch at line 1", err)
		}
	})

	t.Run("length inference", func(t *testing.T) {
		got, err := DetectAlgorithm("checksums.txt", []Entry{md5Entry, entry(strings.Repeat("ab", 16), 2)})
		if err != nil || got != hashcore.MD5 {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("mixed length error with line", func(t *testing.T) {
		_, err := DetectAlgorithm("checksums.txt", []Entry{md5Entry, sha1Entry})
		pe, ok := err.(*Error)
		if !ok || pe.Code != "mixed_length" || pe.Line != 2 {
			t.Errorf("err = %v, want mixed_length at line 2", err)
		}
	})
}

func TestEqualHash(t *testing.T) {
	if !EqualHash("BA7816BF", "ba7816bf") {
		t.Error("should be case-insensitive")
	}
	if EqualHash("abc", "abd") {
		t.Error("different hashes must not match")
	}
}

// TestEscapeUnescapeRoundTrip 转义/反转义互逆（含反斜杠、换行与无需转义的普通名）。
func TestEscapeUnescapeRoundTrip(t *testing.T) {
	for _, name := range []string{`a\b.txt`, "a\nb.txt", "plain.txt", `end\`, "mix\\ed\nname"} {
		esc, need := escapeName(name)
		if !need {
			if esc != name {
				t.Errorf("escapeName(%q) = %q, want unchanged", name, esc)
			}
			continue
		}
		if got := unescapeName(esc); got != name {
			t.Errorf("round trip %q -> %q -> %q", name, esc, got)
		}
	}
}

// TestWriteSUMEscapedName 含特殊字符的文件名经 WriteSUM 导出后可被 ParseManifest 还原。
func TestWriteSUMEscapedName(t *testing.T) {
	const h = "900150983cd24fb0d6963f7d28e17f72"
	names := []string{`dir\file.txt`, "line\nbreak.txt", "plain.txt"}
	var items []ExportItem
	for _, n := range names {
		items = append(items, ExportItem{
			Path: n, Size: 1, Status: "ok",
			Hashes: map[string]string{"md5": h},
		})
	}
	var buf bytes.Buffer
	if err := WriteSUM(&buf, items, hashcore.MD5); err != nil {
		t.Fatal(err)
	}
	entries, err := ParseManifest(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("expect %d entries, got %d", len(names), len(entries))
	}
	for i, n := range names {
		if entries[i].Path != n || entries[i].Hash != h {
			t.Errorf("entry %d = %+v, want path %q", i, entries[i], n)
		}
	}
}

// TestWriteCSVVerifyColumns 校验模式 CSV 附加 expected/actual/verdict 三列。
func TestWriteCSVVerifyColumns(t *testing.T) {
	var buf bytes.Buffer
	items := []ExportItem{{
		Path: "a.txt", Size: 3, Status: "ok",
		Hashes:   map[string]string{"md5": "abc"},
		Expected: "abc", Actual: "abc", Verdict: "pass",
	}}
	if err := WriteCSV(&buf, items, []string{"md5"}, true); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(buf.Bytes()[3:])), "\n") // 跳过 BOM
	if len(lines) != 2 {
		t.Fatalf("csv lines = %d, want 2", len(lines))
	}
	for _, col := range []string{"expected", "actual", "verdict"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header missing column %q: %s", col, lines[0])
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), "abc,abc,pass") {
		t.Errorf("row should end with expected/actual/verdict values: %s", lines[1])
	}
}

// TestParseManifestTabSeparator 哈希与文件名之间允许 tab 分隔。
func TestParseManifestTabSeparator(t *testing.T) {
	entries, err := ParseManifest([]byte("900150983cd24fb0d6963f7d28e17f72\tfile.txt\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "file.txt" {
		t.Errorf("entries = %+v", entries)
	}
}

// TestParseManifestBadHashLength 长度不受支持的哈希报 bad_hash_length 并给行号。
func TestParseManifestBadHashLength(t *testing.T) {
	_, err := ParseManifest([]byte("deadbeef  a.txt\n")) // 8 位（CRC32）不受清单支持
	pe, ok := err.(*Error)
	if !ok || pe.Code != "bad_hash_length" || pe.Line != 1 {
		t.Errorf("err = %v, want bad_hash_length at line 1", err)
	}
}

// TestResolveTargets 相对路径解析、重复条目去重（首条胜出）、缺失/目录归类。
func TestResolveTargets(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.txt")
	subDir := filepath.Join(dir, "sub")
	bPath := filepath.Join(subDir, "b.txt")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{aPath, bPath} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries := []Entry{
		{Hash: "h1", Path: "a.txt", Line: 1},
		{Hash: "h2", Path: "./a.txt", Line: 2}, // 与第 1 条同路径，应去重且首条胜出
		{Hash: "h3", Path: bPath, Line: 3},     // 绝对路径直接使用
		{Hash: "h4", Path: "ghost.txt", Line: 4},
		{Hash: "h5", Path: "sub", Line: 5}, // 目录按缺失处理
	}
	toHash, missing, expected := ResolveTargets(entries, dir)

	if len(toHash) != 2 {
		t.Fatalf("toHash = %+v, want 2 items", toHash)
	}
	if toHash[0].Path != aPath || toHash[1].Path != bPath {
		t.Errorf("toHash paths = %s, %s", toHash[0].Path, toHash[1].Path)
	}
	if expected[aPath] != "h1" {
		t.Errorf("expected[aPath] = %q, want h1 (first occurrence wins)", expected[aPath])
	}
	if len(missing) != 2 {
		t.Fatalf("missing = %+v, want 2 items", missing)
	}
	if missing[0].Path != filepath.Join(dir, "ghost.txt") {
		t.Errorf("missing path not resolved: %s", missing[0].Path)
	}
	if missing[1].Path != subDir {
		t.Errorf("directory should be missing, got %s", missing[1].Path)
	}
}

func TestIsManifestName(t *testing.T) {
	cases := map[string]bool{
		"a.sha256": true, "b.MD5": true, "c.txt": true, "d.sums": true,
		"e.exe": false, "f.bin": false, "g": false,
	}
	for name, want := range cases {
		if got := IsManifestName(name); got != want {
			t.Errorf("IsManifestName(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestParseManifestComments "#" 开头的注释行被跳过；合法条目行首必为十六进制哈希，
// 不会与注释行混淆。只剩注释的清单按空清单报错。
func TestParseManifestComments(t *testing.T) {
	content := "# SHA256 sums generated by CI\n" +
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  hello.txt\n" +
		"# 中文注释也行\n" +
		"\n" +
		"900150983cd24fb0d6963f7d28e17f72 *bin.dat\n"
	entries, err := ParseManifest([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expect 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Path != "hello.txt" || entries[0].Line != 2 {
		t.Errorf("entry0 = %+v, want path hello.txt at line 2", entries[0])
	}
	if entries[1].Path != "bin.dat" || entries[1].Line != 5 {
		t.Errorf("entry1 = %+v, want path bin.dat at line 5", entries[1])
	}

	_, err = ParseManifest([]byte("# nothing here\n# really nothing\n"))
	if pe, ok := err.(*Error); !ok || pe.Code != "empty_manifest" {
		t.Errorf("err = %v, want empty_manifest", err)
	}
}

// TestResolveTargetsCaseFoldDedup Windows 下大小写不同的同路径条目去重且首条胜出。
func TestResolveTargetsCaseFoldDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 文件系统大小写不敏感")
	}
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(aPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{Hash: "first", Path: "a.txt", Line: 1},
		{Hash: "second", Path: "A.TXT", Line: 2}, // 同一文件的不同大小写写法，应去重
	}
	toHash, missing, expected := ResolveTargets(entries, dir)
	if len(toHash) != 1 || len(missing) != 0 {
		t.Fatalf("toHash = %+v, missing = %+v, want 1 hashed and 0 missing", toHash, missing)
	}
	if expected[aPath] != "first" {
		t.Errorf("expected[aPath] = %q, want first occurrence wins", expected[aPath])
	}
}
