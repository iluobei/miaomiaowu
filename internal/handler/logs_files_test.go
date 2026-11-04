package handler

import (
	"path/filepath"
	"testing"
)

func TestSafeLogPathRejectsTraversal(t *testing.T) {
	bad := []string{"", ".", "..", "../traffic.db", "a/b.log", `a\b.log`, "/etc/passwd", "mmw..log", " .. "}
	for _, name := range bad {
		if _, err := safeLogPath(name); err == nil {
			t.Errorf("safeLogPath(%q) 应当拒绝", name)
		}
	}
	got, err := safeLogPath("mmw-2026-01-01T00-00-00.000.log")
	if err != nil {
		t.Fatalf("合法文件名被拒绝: %v", err)
	}
	if want := filepath.Join(logsDir, "mmw-2026-01-01T00-00-00.000.log"); got != want {
		t.Errorf("safeLogPath = %q, want %q", got, want)
	}
}

func TestParseLogfmtLine(t *testing.T) {
	row := parseLogfmtLine(`time="2026-01-01 12:00:00" level="INFO " msg="节点 \"A\" 已同步" count=3 name="x y"`)
	if row.Time != "2026-01-01 12:00:00" || row.Level != "INFO" {
		t.Errorf("time/level 解析错误: %+v", row)
	}
	if row.Msg != `节点 "A" 已同步` {
		t.Errorf("msg 解析错误: %q", row.Msg)
	}
	if row.Fields != `count=3 name="x y"` {
		t.Errorf("fields 解析错误: %q", row.Fields)
	}
	// 裸 msg（无空格不带引号）
	row = parseLogfmtLine(`time="2026-01-01 12:00:00" level=ERROR msg=boom`)
	if row.Level != "ERROR" || row.Msg != "boom" || row.Fields != "" {
		t.Errorf("裸值解析错误: %+v", row)
	}
	// 非本格式的行：原样保留
	row = parseLogfmtLine("plain text line")
	if row.Raw != "plain text line" || row.Level != "" {
		t.Errorf("非格式行应保留 Raw: %+v", row)
	}
	// 引号未闭合的半行不能 panic
	_ = parseLogfmtLine(`time="2026-01-01 12:00:00" level="INFO " msg="half`)
}
