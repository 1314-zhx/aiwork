package httpx

import "testing"

func TestJoinHostPath(t *testing.T) {
	got := JoinHostPath("http://127.0.0.1:8000", "upload/a.png")
	if got != "http://127.0.0.1:8000/upload/a.png" {
		t.Fatalf("got %s", got)
	}
	got = JoinHostPath("http://127.0.0.1:8000/", "/upload/a.png")
	if got != "http://127.0.0.1:8000/upload/a.png" {
		t.Fatalf("got %s", got)
	}
}
