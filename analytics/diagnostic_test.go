// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

const DiagnosticDataDirectory = "../diagnostic.data"
const DiagnosticDataFilename = DiagnosticDataDirectory + "/metrics.2019-02-21T16-42-24Z-00000"

func TestReadDiagnosticFiles(t *testing.T) {
	var err error
	var files []os.FileInfo
	var filenames []string

	if files, err = ioutil.ReadDir(DiagnosticDataDirectory); err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if strings.Index(f.Name(), "metrics.") != 0 && strings.Index(f.Name(), "keyhole_stats.") != 0 {
			continue
		}
		filename := DiagnosticDataDirectory + "/" + f.Name()
		filenames = append(filenames, filename)
	}
	d := NewDiagnosticData(300)
	if err = d.readDiagnosticFiles(filenames); err != nil {
		t.Fatal(err)
	}
}
