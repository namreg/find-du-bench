package bench

import (
	"testing"
	"time"
)

var dir = "/go/src/github.com/google/cadvisor"

func BenchmarkDiskUsageNew(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		if _, err = DiskUsageNew(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDiskUsageOld(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		if _, err = GetDirDiskUsage(dir, timeout); err != nil {
			b.Fatal(err)
		}
		if _, err = GetDirInodeUsage(dir, timeout); err != nil {
			b.Fatal(err)
		}
	}
}

var timeout = 2 * time.Minute
