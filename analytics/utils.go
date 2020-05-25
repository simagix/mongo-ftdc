// Copyright 2020 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GetMetricsFilenames gets metrics or keyhole_stats filesnames
func GetMetricsFilenames(filenames []string) []string {
	var err error
	var fi os.FileInfo
	fnames := []string{}

	for _, filename := range filenames {
		if fi, err = os.Stat(filename); err != nil {
			continue
		}
		switch mode := fi.Mode(); {
		case mode.IsDir():
			files, _ := ioutil.ReadDir(filename)
			for _, file := range files {
				if file.IsDir() == false &&
					(strings.HasPrefix(file.Name(), "metrics.") || strings.HasPrefix(file.Name(), "keyhole_stats.")) {
					fnames = append(fnames, filename+"/"+file.Name())
				}
			}
		case mode.IsRegular():
			basename := filepath.Base(filename)
			if strings.HasPrefix(basename, "metrics.") || strings.HasPrefix(basename, "keyhole_stats.") {
				fnames = append(fnames, filename)
			}
		}
	}
	sort.Slice(fnames, func(i int, j int) bool {
		return fnames[i] < fnames[j]
	})
	return fnames
}

func GetScoreByRange(v float64, low float64, high float64) string {
	var score float64
	if v < low {
		score = 100
	} else if v > high {
		score = 0
	} else {
		score = 100 * (1 - float64(v-low)/(high-low))
	}
	return fmt.Sprintf("%v", score)
}

// GetShortLabel gets shorten label
func GetShortLabel(label string) string {
	if strings.HasPrefix(label, "conns_") {
		label = label[6:]
	} else if strings.HasPrefix(label, "cpu_") {
		label = label[4:]
	} else if strings.HasPrefix(label, "latency_") {
		label = label[8:]
	} else if strings.HasPrefix(label, "mem_") {
		label = label[4:]
	} else if strings.HasPrefix(label, "net_") {
		label = label[4:]
	} else if strings.HasPrefix(label, "ops_") {
		label = label[4:]
	} else if strings.HasPrefix(label, "q_") {
		label = label[2:]
	} else if strings.HasPrefix(label, "scan_") {
		label = label[5:]
	} else if strings.HasPrefix(label, "ticket_") {
		label = label[7:]
	} else if strings.HasPrefix(label, "wt_blkmgr_") {
		label = label[10:]
	} else if strings.HasPrefix(label, "wt_cache_") {
		label = label[9:]
	} else if strings.HasPrefix(label, "wt_") {
		label = label[3:]
	}
	return label
}
