// Copyright 2018-present Kuei-chun Chen. All rights reserved.
// decode_test.go

package decoder

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

var filename = "../diagnostic.data/metrics.2025-12-10T21-34-36Z-00000"

func TestDecode(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = os.ReadFile(filename); err != nil {
		t.Fatal(err)
	}
	m := NewMetrics()
	if err = m.ReadAllMetrics(&buffer); err != nil {
		t.Fatal(err)
	}
	if len(m.Data) == 0 {
		t.Fatal("no data decoded")
	}
}

func BenchmarkReadAllMetrics(b *testing.B) {
	buffer, err := os.ReadFile(filename)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := NewMetrics()
		m.ReadAllMetrics(&buffer)
	}
}

func TestTraverseDocElem(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = os.ReadFile(filename); err != nil {
		t.Fatal(err)
	}
	m := NewMetrics()
	m.ReadAllMetrics(&buffer)
	if len(m.Data) == 0 {
		t.Fatal()
	}

	var dp = MetricsData{DataPointsMap: map[string][]uint64{}}
	var docElem = bson.D{}
	var attribsList = []string{}
	bson.Unmarshal(m.Data[0].Block, &docElem) // first document
	traverseDocElem(&attribsList, &dp.DataPointsMap, docElem, "", 1) // sliceCap=1 for test
	// Note: list may have more entries than map due to duplicate BSON keys
	if len(attribsList) == 0 || len(dp.DataPointsMap) > len(attribsList) {
		t.Fatal()
	}
}
