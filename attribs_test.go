// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// attribs_test.go

package ftdc

import (
	"os"
	"testing"

	"github.com/simagix/mongo-ftdc/decoder"
)

func TestGetServerStatusDataPoints(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = os.ReadFile(DiagnosticDataFilename); err != nil {
		t.Fatal(err)
	}
	metrics := decoder.NewMetrics()
	metrics.ReadAllMetrics(&buffer)
	attrib := NewAttribs(&metrics.Data[0].DataPointsMap)
	v := attrib.GetServerStatusDataPoints(0)
	t.Log(v)
}

func TestGetSystemMetricsDataPoints(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = os.ReadFile(DiagnosticDataFilename); err != nil {
		t.Fatal(err)
	}
	metrics := decoder.NewMetrics()
	metrics.ReadAllMetrics(&buffer)
	attrib := NewAttribs(&metrics.Data[0].DataPointsMap)
	v := attrib.GetSystemMetricsDataPoints(0)
	t.Log(v)
}
