package wal

import (
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/config"
)

func baseWalConfig(dir string) *config.WalConfig {
	return &config.WalConfig{
		FlushingBatchTimeout: 10 * time.Millisecond,
		FlushingBatchCount:   1,
		FlushingBatchVolume:  1024,
		MaxSegmentSize:       1024,
		SegmentStoragePath:   dir,
		MaskName:             "wal_",
	}
}
