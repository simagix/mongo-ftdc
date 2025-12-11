// Copyright 2019-present Kuei-chun Chen. All rights reserved.
// diagnostic_test.go

package ftdc

import (
	"os"
	"strings"
	"testing"
)

const DiagnosticDataDirectory = "diagnostic.data"
const DiagnosticDataFilename = DiagnosticDataDirectory + "/metrics.2020-05-19T20-43-17Z-00000"

func TestReadDiagnosticFiles(t *testing.T) {
	var err error
	var files []os.DirEntry
	var filenames []string

	if files, err = os.ReadDir(DiagnosticDataDirectory); err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if strings.Index(f.Name(), "metrics.") != 0 && strings.Index(f.Name(), "keyhole_stats.") != 0 {
			continue
		}
		filename := DiagnosticDataDirectory + "/" + f.Name()
		filenames = append(filenames, filename)
	}
	d := NewDiagnosticData()
	if err = d.readDiagnosticFiles(filenames); err != nil {
		t.Fatal(err)
	}
}
