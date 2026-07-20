package domain

import "testing"

func TestParseProcfile(t *testing.T) {
	text := `
# comment
web: serve --port $PORT
worker: run-worker
release: rake db:migrate
`
	got, err := ParseProcfile(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "web" || got[0].Expose != "http" || got[0].Quantity != 1 {
		t.Fatalf("web: %+v", got[0])
	}
	if got[1].Name != "worker" || got[1].Expose != "none" {
		t.Fatalf("worker: %+v", got[1])
	}
	if got[2].Name != "release" || got[2].Quantity != 0 {
		t.Fatalf("release: %+v", got[2])
	}
}

func TestParseProcfileEmpty(t *testing.T) {
	if _, err := ParseProcfile("# only comment\n"); err == nil {
		t.Fatal("expected error")
	}
}
