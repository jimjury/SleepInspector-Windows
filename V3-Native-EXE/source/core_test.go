package main

import (
	"os"
	"strings"
	"testing"
)

func TestParserSamples(t *testing.T) {
	tests := []struct {
		file    string
		count   int
		system  int
		display int
		other   int
	}{
		{"requests_en.txt", 4, 2, 1, 1},
		{"requests_zh.txt", 2, 2, 0, 0},
		{"requests_none.txt", 0, 0, 0, 0},
	}
	for _, tt := range tests {
		raw, err := os.ReadFile(tt.file)
		if err != nil {
			t.Fatal(err)
		}
		res := ParseRequests(string(raw))
		if len(res.Items) != tt.count {
			t.Fatalf("%s got %d want %d, %+v", tt.file, len(res.Items), tt.count, res.Items)
		}
		a, b, c := countRequests(res.Items)
		if a != tt.system || b != tt.display || c != tt.other {
			t.Fatalf("%s: counts %d %d %d", tt.file, a, b, c)
		}
	}
}
func TestDetailsAndCase(t *testing.T) {
	r := ParseRequests("SYSTEM:\n[PROCESS] C:\\A\\one.exe\nLine one.\nLine two.\nDISPLAY:\nNone.\n")
	if len(r.Items) != 1 || r.Items[0].Detail != "Line one.\nLine two." {
		t.Fatalf("%+v", r.Items)
	}
}
func TestChineseHeadersAndKinds(t *testing.T) {
	r := ParseRequests("系统：\n[服务] 任务\n说明\n显示：\n[驱动程序] 声卡\n")
	if len(r.Items) != 2 || r.Items[0].Kind != "SERVICE" || r.Items[1].Kind != "DRIVER" {
		t.Fatal(r.Items)
	}
}
func TestReport(t *testing.T) {
	r := ParseRequests("SYSTEM:\nNone.\n")
	report := reportText(r, "A", "B", []string{"已关闭"})
	for _, needle := range []string{"A", "B", "已关闭", "SYSTEM:"} {
		if !strings.Contains(report, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
}

func TestUnrecognizedOutputNotSilentZero(t *testing.T) {
	r := ParseRequests("UNKNOWN BLOCK:\n[PROCESS] C:\\Foo.exe\n")
	if r.Err == "" || len(r.Items) != 0 {
		t.Fatal("unknown output must be flagged")
	}
}
