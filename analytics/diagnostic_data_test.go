// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"io/ioutil"
	"testing"

	"github.com/simagix/mongo-ftdc/ftdc"
)

func TestGetServerStatusDataPoints(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = ioutil.ReadFile(DiagnosticDataFilename); err != nil {
		t.Fatal(err)
	}
	metrics := ftdc.NewMetrics()
	metrics.ReadAllMetrics(buffer)
	v := getServerStatusDataPoints(metrics.Data[0].DataPointsMap, 0)
	t.Log(v)
}

func TestGetSystemMetricsDataPoints(t *testing.T) {
	var err error
	var buffer []byte

	if buffer, err = ioutil.ReadFile(DiagnosticDataFilename); err != nil {
		t.Fatal(err)
	}
	metrics := ftdc.NewMetrics()
	metrics.ReadAllMetrics(buffer)
	v := getSystemMetricsDataPoints(metrics.Data[0].DataPointsMap, 0)
	t.Log(v)
}
