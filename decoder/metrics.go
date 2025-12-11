// Copyright 2018-present Kuei-chun Chen. All rights reserved.
// metrics.go

package decoder

import (
	"bytes"
	"compress/zlib"
	"io"
	"runtime"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ReadAllMetrics reads all metrics with parallel block processing
func (m *Metrics) ReadAllMetrics(data *[]byte) error {
	var pos uint32
	buffer := *data

	// First pass: extract compressed data blocks (sequential)
	var compressedBlocks [][]byte

	for {
		if pos >= uint32(len(buffer)) {
			break
		}
		bs := buffer[pos:(pos + 4)]
		length := GetUint32(bytes.NewReader(bs))
		bs = buffer[pos:(pos + length)]
		pos += length

		var out = bson.M{}
		if err := bson.Unmarshal(bs, &out); err != nil {
			return err
		} else if out["type"] == int32(0) {
			m.Doc = out["doc"]
		} else if out["type"] == int32(1) {
			// Skip first 4 bytes of binary data (uncompressed size)
			compressedBlocks = append(compressedBlocks, (out["data"].(primitive.Binary)).Data[4:])
		}
	}

	if len(compressedBlocks) == 0 {
		return nil
	}

	// Second pass: decompress and decode blocks in parallel
	numWorkers := runtime.NumCPU()
	if numWorkers > len(compressedBlocks) {
		numWorkers = len(compressedBlocks)
	}

	metricsData := make([]MetricsData, len(compressedBlocks))
	var firstErr error
	var errOnce sync.Once
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for i, compressed := range compressedBlocks {
		wg.Add(1)
		go func(idx int, data []byte) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			// Skip if error already occurred
			if firstErr != nil {
				return
			}

			// Decompress
			r, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			decompressed, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}

			// Decode
			md, err := m.decode(decompressed)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			metricsData[idx] = md
		}(i, compressed)
	}

	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	m.Data = metricsData
	return nil
}
