package roomwork

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A room's cost per turn is a copy of its folder and a merge of what the turn changed. These time both on folders of three sizes,
// the ones docs/ROOM-WORK.md quotes:
//
//	go test ./internal/roomwork -run '^$' -bench Turn -benchtime 3x
var folders = []struct {
	name  string
	files int
	kb    int
}{{"200files", 200, 10}, {"5000files", 5000, 20}, {"30000files", 30000, 20}}

func fill(b *testing.B, folder string, files, kb int) {
	b.Helper()
	text := make([]byte, kb<<10)
	for i := range files {
		path := filepath.Join(folder, fmt.Sprintf("d%d", i%50), fmt.Sprintf("s%d", i%7), fmt.Sprintf("f%d.txt", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		text[0], text[1] = byte(i), byte(i>>8)
		if err := os.WriteFile(path, text, 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTurnBegin(b *testing.B) {
	for _, size := range folders {
		b.Run(size.name, func(b *testing.B) {
			base := b.TempDir()
			in := Room{Folder: filepath.Join(base, "workspace"), Meta: filepath.Join(base, "merge")}
			fill(b, in.Folder, size.files, size.kb)
			hub := NewHub(Limits{Bytes: 8 << 30, Files: 1 << 20})
			b.ResetTimer()
			for range b.N {
				dest := filepath.Join(base, "copy")
				turn, err := hub.Begin(context.Background(), in, dest, "Ada")
				if err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				turn.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkTurnMerge is the merge of a turn that changed ten files, with the scan of its copy that finds them.
func BenchmarkTurnMerge(b *testing.B) {
	for _, size := range folders {
		b.Run(size.name, func(b *testing.B) {
			base := b.TempDir()
			in := Room{Folder: filepath.Join(base, "workspace"), Meta: filepath.Join(base, "merge")}
			fill(b, in.Folder, size.files, size.kb)
			hub := NewHub(Limits{Bytes: 8 << 30, Files: 1 << 20})
			var total time.Duration
			for round := range b.N {
				turn, err := hub.Begin(context.Background(), in, filepath.Join(base, "copy"), "Ada")
				if err != nil {
					b.Fatal(err)
				}
				for i := range 10 {
					path := filepath.Join(turn.dest, "d0", "s0", fmt.Sprintf("f%d.txt", i*350))
					if err := os.WriteFile(path, []byte(fmt.Sprintf("round %d file %d", round, i)), 0o644); err != nil {
						b.Fatal(err)
					}
				}
				began := time.Now()
				if _, err := turn.Merge(context.Background()); err != nil {
					b.Fatal(err)
				}
				total += time.Since(began)
				turn.Close()
			}
			b.ReportMetric(float64(total.Milliseconds())/float64(b.N), "merge-ms")
		})
	}
}
