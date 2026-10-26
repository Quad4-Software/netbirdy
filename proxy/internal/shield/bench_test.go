package shield

import (
	"fmt"
	"testing"
)

func BenchmarkClassifyUA(b *testing.B) {
	cfg := DefaultConfig()
	sh, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("repeat", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sh.classifyUA("sqlmap/1.8")
		}
	})
	b.Run("flood", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sh.classifyUA(fmt.Sprintf("ua-%d", i))
		}
	})
}
