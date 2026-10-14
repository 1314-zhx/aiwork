package router

import (
	"errors"
	"testing"
)

func TestReasonMissing(t *testing.T) {
	got := ReasonMissing("文件ID", "文件名", "表名", "要查询字段")
	want := "用户输入不包含文件ID，文件名，表名，要查询字段等必须内容。"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestNotHandlesUnwraps(t *testing.T) {
	err := NewUnknownIntent([]string{"db", "file"}, "DEFAULT", "看看数据")
	if !errors.Is(err, ErrNotHandles) {
		t.Fatal("want ErrNotHandles")
	}
	miss, ok := AsNotHandles(err)
	if !ok || miss.Destination != "" || miss.NextInputs != "看看数据" {
		t.Fatalf("miss=%+v ok=%v", miss, ok)
	}
	if len(miss.Need) == 0 || miss.Hint == "" {
		t.Fatalf("missing hint/need: %+v", miss)
	}
}

func TestNewMissingKeepsPartial(t *testing.T) {
	err := NewMissing("file", []string{"fileId", "fileName"}, []string{"文件ID", "文件名"}, map[string]string{
		"fileName": "预算.xlsx",
		"input":    "分析这个表",
	}, "请补充文件ID或文件名。")
	miss, ok := AsNotHandles(err)
	if !ok || miss.Destination != "file" || miss.Partial["fileName"] != "预算.xlsx" {
		t.Fatalf("miss=%+v", miss)
	}
	if miss.Reason != "用户输入不包含文件ID，文件名等必须内容。" {
		t.Fatalf("reason=%q", miss.Reason)
	}
}

func TestAsNotHandlesBareSentinel(t *testing.T) {
	miss, ok := AsNotHandles(ErrNotHandles)
	if !ok || miss.Reason != ErrNotHandles.Error() {
		t.Fatalf("miss=%+v ok=%v", miss, ok)
	}
}
