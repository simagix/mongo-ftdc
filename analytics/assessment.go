// Copyright 2020 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Assessment stores timeserie data
type Assessment struct {
	blocks     int
	serverInfo ServerInfoDoc
	stats      FTDCStats
	wtCacheMax float64
	verbose    bool
}

type metricStats struct {
	label  string
	median float64
	p1     float64
	p95    float64
	score  int
}

// NewAssessment returns assessment object
func NewAssessment(serverInfo ServerInfoDoc, stats FTDCStats) *Assessment {
	assessment := Assessment{blocks: 3, serverInfo: serverInfo, stats: stats}
	if len(stats.TimeSeriesData["wt_cache_max"].DataPoints) > 0 {
		assessment.wtCacheMax = stats.TimeSeriesData["wt_cache_max"].DataPoints[0][0]
	}
	return &assessment
}

// SetVerbose sets verbose level
func (as *Assessment) SetVerbose(verbose bool) {
	as.verbose = verbose
}

// GetAssessment gets assessment summary
func (as *Assessment) GetAssessment(from time.Time, to time.Time) map[string]interface{} {
	var headerList []map[string]string
	var rowList [][]interface{}

	if to.Sub(from) <= 24*time.Hour {
		for i := 0; i < as.blocks; i++ {
			headerList = append(headerList, map[string]string{"text": "Metric", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Score", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "p5", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Median", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "p95", "type": "Number"})
		}
		marr := []metricStats{}
		metrics := serverStatusChartsLegends
		metrics = append(metrics, wiredTigerChartsLegends...)
		for _, sm := range systemMetricsChartsLegends {
			if strings.HasPrefix(sm, "cpu_") {
				metrics = append(metrics, sm)
			}
		}
		for _, v := range metrics {
			m := as.getStatsArray(v, from, to)
			if m.score < 101 || as.verbose {
				marr = append(marr, m)
			}
		}
		for k, v := range as.stats.DiskStats {
			p1, median, p95 := as.getStatsByData(v.IOPS, from, to)
			if p95 == 0 {
				continue
			}
			m := as.getStatsArrayByValues("iops_"+k, p1, median, p95)
			if m.score < 101 || as.verbose {
				marr = append(marr, m)
			}
			p1, median, p95 = as.getStatsByData(v.Utilization, from, to)
			m = as.getStatsArrayByValues("disku_"+k, p1, median, p95)
			if m.score < 101 || as.verbose {
				marr = append(marr, m)
			}
		}
		sort.Slice(marr, func(i int, j int) bool {
			// return marr[i].label < marr[j].label
			if marr[i].score < marr[j].score {
				return true
			} else if marr[i].score == marr[j].score {
				return marr[i].label < marr[j].label
			}
			return false
		})
		arr := []interface{}{}
		for _, v := range marr {
			arr = append(arr, []interface{}{v.label, v.score, v.p1, v.median, v.p95}...)
			if len(arr) == 5*as.blocks {
				rowList = append(rowList, arr)
				arr = []interface{}{}
			}
		}
		if len(arr) > 0 {
			rowList = append(rowList, arr)
		}
	} else {
		headerList = append(headerList, map[string]string{"text": "Reason", "type": "string"})
		rowList = append(rowList, []interface{}{"Assessment is available when date range is less than a day"})
	}
	return map[string]interface{}{"columns": headerList, "type": "table", "rows": rowList}
}

func (as *Assessment) getStatsArray(metric string, from time.Time, to time.Time) metricStats {
	p1, median, p95 := as.getStatsByData(as.stats.TimeSeriesData[metric], from, to)
	return as.getStatsArrayByValues(metric, p1, median, p95)
}

func (as *Assessment) getStatsArrayByValues(metric string, p1 float64, median float64, p95 float64) metricStats {
	var score = 101
	label := metric
	if strings.HasSuffix(label, "modified_evicted") {
		label = strings.ReplaceAll(label, "modified_evicted", "mod_evict")
	}
	if as.wtCacheMax > 0 && (metric == "wt_cache_used" || metric == "wt_cache_dirty") {
		u := 100 * p95 / as.wtCacheMax
		if metric == "wt_cache_used" { // 80% to 100%
			score = GetScoreByRange(u, 80, 95)
		} else if metric == "wt_cache_dirty" { //5% to 20%
			score = GetScoreByRange(u, 5, 20)
		}
		return metricStats{label: label + " %", score: score, p1: math.Round(100 * p1 / as.wtCacheMax),
			median: math.Round(100 * median / as.wtCacheMax), p95: math.Round(100 * p95 / as.wtCacheMax)}
	} else if as.serverInfo.HostInfo.System.MemSizeMB > 0 && metric == "mem_resident" {
		total := float64(as.serverInfo.HostInfo.System.MemSizeMB) / 1024
		u := 100 * p95 / total
		score = GetScoreByRange(u, 70, 90)
		return metricStats{label: label + " %", score: score, p1: math.Round(100 * p1 / total),
			median: math.Round(100 * median / total), p95: math.Round(100 * p95 / total)}
	}
	score = as.getScore(metric, p1, median, p95)
	if strings.HasPrefix(metric, "cpu_") || strings.HasPrefix(metric, "disku_") {
		return metricStats{label: label + " %", score: score, p1: math.Round(p1),
			median: math.Round(median), p95: math.Round(p95)}
	}
	return metricStats{label: label, score: score, p1: math.Round(p1),
		median: math.Round(median), p95: math.Round(p95)}
}

func (as *Assessment) getStatsByData(data TimeSeriesDoc, from time.Time, to time.Time) (float64, float64, float64) {
	return as.getMedianStatsByData(data, from, to)
}

func (as *Assessment) getMedianStatsByData(data TimeSeriesDoc, from time.Time, to time.Time) (float64, float64, float64) {
	stats := FilterTimeSeriesData(data, from, to)
	if len(stats.DataPoints) == 0 {
		return 0, 0, 0
	}
	arr := []float64{}
	for _, dp := range stats.DataPoints {
		arr = append(arr, dp[0])
	}
	sort.Slice(arr, func(i int, j int) bool {
		return arr[i] < arr[j]
	})
	samples := float64(len(arr) + 1)
	p5 := int(samples * 0.05)
	median := int(samples * .5)
	p95 := int(samples * .95)
	if p95 > len(arr)-1 {
		p95 = len(arr) - 1
	}
	return arr[p5], arr[median], arr[p95]
}

func (as *Assessment) getScore(metric string, p1 float64, median float64, p95 float64) int {
	score := 101
	if median < 1 {
		return score
	}
	if metric == "conns_current" { // 5% to 20%
		pct := 100 * p95 / float64(as.serverInfo.HostInfo.System.MemSizeMB)
		score = GetScoreByRange(pct, 5, 20)
	} else if metric == "conns_created/s" { // 120 conns created per minute, 2/second
		score = GetScoreByRange(median, 0, 2)
	} else if metric == "cpu_idle" {
		score = 100 - GetScoreByRange(p1, 50, 80)
	} else if metric == "cpu_iowait" || metric == "cpu_system" { // 5% - 15% io_wait
		score = GetScoreByRange(p95, 5, 15)
	} else if metric == "cpu_user" { // under 50% is good
		score = GetScoreByRange(p95, 50, 70)
	} else if strings.HasPrefix(metric, "disku_") { // under 70% is good
		score = GetScoreByRange(p95, 50, 90)
	} else if strings.HasPrefix(metric, "iops_") { // median:p95 ratio
		if p95 < 100 {
			return score
		}
		u := p95 / median
		score = GetScoreByRange(u, 2, 4)
	} else if metric == "latency_command" || metric == "latency_read" || metric == "latency_write" { // 20ms is good
		score = GetScoreByRange(p95, 20, 100)
	} else if metric == "mem_page_faults" { // 10 page faults
		score = GetScoreByRange(p95, 10, 20)
	} else if strings.HasPrefix(metric, "ops_") { // 2ms an op
		total := 500.0 * 128
		score = GetScoreByRange(p95, 0, total)
	} else if metric == "q_queued_read" || metric == "q_queued_write" {
		cores := float64(as.serverInfo.HostInfo.System.NumCores)
		score = GetScoreByRange(p95, cores, 5*cores)
	} else if metric == "scan_objects" {
		if p95 < 1000 {
			return 100
		}
		keys := as.stats.TimeSeriesData["scan_keys"]
		objs := as.stats.TimeSeriesData["scan_objects"]
		max := 0.0
		for i := range keys.DataPoints {
			scale := objs.DataPoints[i][0] / keys.DataPoints[i][0]
			if scale > max {
				max = scale
			}
		}
		score = GetScoreByRange(max, 2, 5)
	} else if metric == "scan_keys" { // 1 mil/sec key scanned
		score = GetScoreByRange(p95, 0, 1024.0*1024)
	} else if metric == "scan_sort" { // 1 k sorted in mem
		score = GetScoreByRange(p95, 0, 1000)
	} else if metric == "ticket_avail_read" || metric == "ticket_avail_write" {
		score = int(100 * p1 / 128)
	} else if metric == "wt_modified_evicted" || metric == "wt_unmodified_evicted" {
		pages := .05 * as.wtCacheMax * (1024 * 1024 * 1024) / (4 * 1024) // 5% of WiredTiger cache
		score = GetScoreByRange(p95, pages, 2*pages)
	}
	return score
}
