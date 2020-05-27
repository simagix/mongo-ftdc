// Copyright 2020 Kuei-chun Chen. All rights reserved.

package analytics

import (
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

// GetScoreByRange gets score
func GetScoreByRange(v float64, low float64, high float64) int {
	var score int
	if v < low {
		score = 100
	} else if v > high {
		score = 0
	} else {
		score = int(100 * (1 - float64(v-low)/(high-low)))
	}
	return score
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

func getHTML(metric string) string {
	html := `
	<!DOCTYPE html>
	<html lang="en">
	<head>
	  <title>Metric Scores</title>
		<meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate" />
		<meta http-equiv="Pragma" content="no-cache" />
		<meta http-equiv="Expires" content="0" />
	  <script src="https://www.gstatic.com/charts/loader.js"></script>
	  <style>
			body {
				font-family: Arial, Helvetica, sans-serif;
	      font-size; 11px;
			}
	    table
	    {
	      font-family: Consolas, monaco, monospace;
	      font-size; 10px;
	    	border-collapse:collapse;
	    	#width:100%;
	    	#max-width:800px;
	    	min-width:600px;
	    	#text-align:center;
	    }
	    caption
	    {
	    	caption-side:top;
	    	font-weight:bold;
	    	font-style:italic;
	    	margin:4px;
	    }
	    table,th, td
	    {
	    	border: 1px solid gray;
	    }
	    th, td
	    {
	    	height: 24px;
	    	padding:4px;
	    	vertical-align:middle;
	    }
	    th
	    {
	    	#background-image:url(table-shaded.png);
	      background-color: #333;
	      color: #FFF;
	    }
	    .rowtitle
	    {
	    	font-weight:bold;
	    }
			a {
			  text-decoration: none;
			  color: #000;
			  display: block;

			  -webkit-transition: font-size 0.3s ease, background-color 0.3s ease;
			  -moz-transition: font-size 0.3s ease, background-color 0.3s ease;
			  -o-transition: font-size 0.3s ease, background-color 0.3s ease;
			  -ms-transition: font-size 0.3s ease, background-color 0.3s ease;
			  transition: font-size 0.3s ease, background-color 0.3s ease;
			}
			a:hover {
				color: blue;
			}
	    .button {
	      background-color: #333;
	      border: none;
	      border-radius: 8px;
	      color: white;
	      padding: 3px 6px;
	      text-align: center;
	      text-decoration: none;
	      display: inline-block;
	      font-size: 12px;
	    }
	    </style>
	</head>
	<body><h3>Scores:</h3>
	<table>
	<tr><th>Metric</th><th>Formula</th><th>Acceptable</th><th>High</th></tr>
	<tr><td class='rowtitle'>conns_created/s</td><td>conns_created/s</td><td align='right'>0</td><td align='right'>2</td></tr>
	<tr><td class='rowtitle'>conns_current</td><td>1MB*(p95 of conns_current)/RAM</td><td align='right'>5%%</td><td align='right'>20%%</td></tr>

	<tr><td class='rowtitle'>cpu_idle</td><td>p5 of cpu_idle</td><td align='right'>20%%</td><td align='right'>50%%</td></tr>
	<tr><td class='rowtitle'>cpu_iowait</td><td>p95 of cpu_iowait</td><td align='right'>5%%</td><td align='right'>15%%</td></tr>
	<tr><td class='rowtitle'>cpu_user</td><td>p95 of cpu_user</td><td align='right'>50%%</td><td align='right'>70%%</td></tr>
	<tr><td class='rowtitle'>cpu_system</td><td>p95 of cpu_system</td><td align='right'>5%%</td><td align='right'>15%%</td></tr>

	<tr><td class='rowtitle'>disku_&lt;dev&gt;</td><td>p95 of disku_&lt;dev&gt;</td><td align='right'>50%%</td><td align='right'>90%%</td></tr>
	<tr><td class='rowtitle'>iops_&lt;dev&gt;</td><td>(p95 of iops_&lt;dev&gt;)/(avg of iops_&lt;dev&gt;)</td><td align='right'>2</td><td align='right'>4</td></tr>

	<tr><td class='rowtitle'>lantency_command (ms)</td><td>p95 of lantency_command</td><td align='right'>20</td><td align='right'>100</td></tr>
	<tr><td class='rowtitle'>lantency_read (ms)</td><td>p95 of lantency_read</td><td align='right'>20</td><td align='right'>100</td></tr>
	<tr><td class='rowtitle'>lantency_write (ms)</td><td>p95 of lantency_write</td><td align='right'>20</td><td align='right'>100</td></tr>

	<tr><td class='rowtitle'>mem_page_faults</td><td>p95 of mem_page_faults</td><td align='right'>10</td><td align='right'>20</td></tr>
	<tr><td class='rowtitle'>mem_resident</td><td>mem_resident/RAM</td><td align='right'>70%%</td><td align='right'>90%%</td></tr>

	<tr><td class='rowtitle'>ops_&lt;*&gt;</td><td>(ops_&lts*&gt;)</td><td align='right'>0</td><td align='right'>(# wt tickets)*(1s/2ms)</td></tr>

	<tr><td class='rowtitle'>queued_read</td><td>p95 of queued_read</td><td align='right'># cores</td><td align='right'>5*(# cores)</td></tr>
	<tr><td class='rowtitle'>queued_write</td><td>p95 of queued_write</td><td align='right'># cores</td><td align='right'>5*(# cores)</td></tr>

	<tr><td class='rowtitle'>scan_keys</td><td>scan_keys</td><td align='right'>0</td><td align='right'>1 mil</td></tr>
	<tr><td class='rowtitle'>scan_objects</td><td>max of [](scan_objects/scan_keys)</td><td align='right'>2</td><td align='right'>5</td></tr>
	<tr><td class='rowtitle'>scan_sort</td><td>scan_sort</td><td align='right'>0</td><td align='right'>1,000</td></tr>

	<tr><td class='rowtitle'>ticket_avail_read</td><td>(p5 of ticket_avail_read)/128</td><td align='right'>0%%</td><td align='right'>100%%</td></tr>
	<tr><td class='rowtitle'>ticket_avail_write</td><td>(p5 of ticket_avail_write)/128</td><td align='right'>0%%</td><td align='right'>100%%</td></tr>

	<tr><td class='rowtitle'>wt_cache_used</td><td>(p95 of wt_cache_used)/wt_cache_max</td><td align='right'>80%%</td><td align='right'>95%%</td></tr>
	<tr><td class='rowtitle'>wt_cache_dirty</td><td>(p95 of wt_cache_dirty)/wt_cache_max</td><td align='right'>5%%</td><td align='right'>20%%</td></tr>
	</table>
	</body></html>
	`
	return html
}
