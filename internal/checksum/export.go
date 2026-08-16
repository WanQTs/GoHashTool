package checksum

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"gohash/internal/hashcore"
)

// ExportItem 导出用结果行（与上层表格行对应）。
type ExportItem struct {
	Path       string            // 文件完整路径
	Size       int64             // 字节数
	Hashes     map[string]string // algo -> hash
	DurationMs int64             // 耗时（毫秒）
	Status     string            // ok / occupied / no_permission / not_found / error / canceled
	Expected   string            // 校验期望值（仅批量校验）
	Actual     string            // 校验实际值（仅批量校验）
	Verdict    string            // pass / fail / missing（仅批量校验）
}

// WriteSUM 写出标准 SUM 格式：`<hash>  <路径>`（两空格 = 文本模式），
// 文件名含反斜杠/换行时按 GNU 约定加行首 "\" 转义。
// 路径按原样写出（通常为绝对路径）；重新导入时绝对路径直接使用、
// 相对路径相对基准目录解析，因此导出的 SUM 可被本工具批量校验重新导入（闭环）。
func WriteSUM(w io.Writer, items []ExportItem, algo hashcore.Algorithm) error {
	bw := bufio.NewWriter(w)
	for _, it := range items {
		h := it.Hashes[string(algo)]
		if h == "" || it.Status != string(hashcore.StatusOK) {
			continue
		}
		name := it.Path
		prefix := ""
		if escaped, need := escapeName(name); need {
			prefix, name = "\\", escaped
		}
		if _, err := fmt.Fprintf(bw, "%s%s  %s\n", prefix, h, name); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// WriteCSV 写出 CSV，带 UTF-8 BOM（Excel 直接打开不乱码）。
// verify=true 时附加 期望值/实际值/结论 三列（批量校验结果导出）。
func WriteCSV(w io.Writer, items []ExportItem, algos []string, verify bool) error {
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	header := []string{"name", "path", "size_bytes"}
	header = append(header, algos...)
	header = append(header, "duration_ms", "status")
	if verify {
		header = append(header, "expected", "actual", "verdict")
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, it := range items {
		row := []string{fileName(it.Path), it.Path, strconv.FormatInt(it.Size, 10)}
		for _, a := range algos {
			row = append(row, it.Hashes[a])
		}
		row = append(row, strconv.FormatInt(it.DurationMs, 10), it.Status)
		if verify {
			row = append(row, it.Expected, it.Actual, it.Verdict)
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// fileName 取路径末段作为文件名（同时兼容正反斜杠）。
func fileName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
