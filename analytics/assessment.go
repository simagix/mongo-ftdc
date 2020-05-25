// Copyright 2020 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"fmt"
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
	var rowList [][]string

	if to.Sub(from) <= 24*time.Hour {
		for i := 0; i < as.blocks; i++ {
			headerList = append(headerList, map[string]string{"text": "Metric", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Score", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Min", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Avg", "type": "Number"})
			headerList = append(headerList, map[string]string{"text": "Max", "type": "Number"})
		}
		var arr []string
		mmap := map[string][]string{}
		metrics := serverStatusChartsLegends
		metrics = append(metrics, wiredTigerChartsLegends...)
		for _, v := range metrics {
			m := as.getStatsArray(v, from, to)
			if m[1] != "-" || as.verbose {
				mmap[v] = m
			}
		}
		for k, v := range as.stats.DiskStats {
			min, avg, max := as.getStatsByData(v.IOPS, from, to)
			if max == 0 {
				continue
			}
			mmap["iops_"+k] = as.getStatsArrayByValues("iops_"+k, min, avg, max)
			min, avg, max = as.getStatsByData(v.Utilization, from, to)
			mmap["disku+"+k] = as.getStatsArrayByValues("disku_"+k, min, avg, max)
		}

		keys := []string{}
		for k := range mmap {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i int, j int) bool {
			return keys[i] < keys[j]
		})
		arr = []string{}
		for _, v := range keys {
			arr = append(arr, mmap[v]...)
			if len(arr) == 5*as.blocks {
				rowList = append(rowList, arr)
				arr = []string{}
			}
		}
		if len(arr) > 0 {
			rowList = append(rowList, arr)
		}
	} else {
		headerList = append(headerList, map[string]string{"text": "Reason", "type": "string"})
		rowList = append(rowList, []string{fmt.Sprintf("Assessment is available when date range is no greater than a day")})
	}
	return map[string]interface{}{"columns": headerList, "type": "table", "rows": rowList}
}

func (as *Assessment) getStatsArray(metric string, from time.Time, to time.Time) []string {
	min, avg, max := as.getStatsByData(as.stats.TimeSeriesData[metric], from, to)
	return as.getStatsArrayByValues(metric, min, avg, max)
}

func (as *Assessment) getStatsArrayByValues(metric string, min float64, avg float64, max float64) []string {
	var score = "-"
	label := metric
	if strings.HasSuffix(label, "_per_minute") {
		label = strings.ReplaceAll(label, "_per_minute", "/min")
	} else if strings.HasSuffix(label, "modified_evicted") {
		label = strings.ReplaceAll(label, "modified_evicted", "mod_evict")
	}
	if as.wtCacheMax > 0 && (metric == "wt_cache_used" || metric == "wt_cache_dirty") {
		u := 100 * max / as.wtCacheMax
		if metric == "wt_cache_used" { // 80% to 100%
			score = GetScoreByRange(u, 80, 95)
		} else if metric == "wt_cache_dirty" { //5% to 20%
			score = GetScoreByRange(u, 5, 20)
		}
		return []string{label, score, fmt.Sprintf("%.0f%%", math.Round(100*min/as.wtCacheMax)),
			fmt.Sprintf("%.0f%%", math.Round(100*avg/as.wtCacheMax)),
			fmt.Sprintf("%.0f%%", math.Round(100*max/as.wtCacheMax))}
	} else if as.serverInfo.HostInfo.System.MemSizeMB > 0 && metric == "mem_resident" {
		total := float64(as.serverInfo.HostInfo.System.MemSizeMB) / 1024
		u := 100 * max / total
		score = GetScoreByRange(u, 70, 90)
		return []string{label, score, fmt.Sprintf("%.0f%%", math.Round(100*min/total)),
			fmt.Sprintf("%.0f%%", math.Round(100*avg/total)),
			fmt.Sprintf("%.0f%%", math.Round(100*max/total))}
	}
	score = as.getScore(metric, min, avg, max)
	if strings.HasPrefix(metric, "cpu_") || strings.HasPrefix(metric, "disku_") {
		return []string{label, score, fmt.Sprintf("%.0f%%", math.Round(min)),
			fmt.Sprintf("%.0f%%", math.Round(avg)), fmt.Sprintf("%.0f%%", math.Round(max))}
	}
	return []string{label, score, fmt.Sprintf("%f", math.Round(min)),
		fmt.Sprintf("%f", math.Round(avg)), fmt.Sprintf("%f", math.Round(max))}
}

func (as *Assessment) getStatsByData(data TimeSeriesDoc, from time.Time, to time.Time) (float64, float64, float64) {
	stats := FilterTimeSeriesData(data, from, to)
	min := 1000000.0
	avg := 0.0
	max := -1.0
	sum := 0.0
	if len(stats.DataPoints) == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	arr := stats.DataPoints
	if len(stats.DataPoints) > 2 {
		arr = stats.DataPoints[1 : len(stats.DataPoints)-1]
	}
	for _, dp := range arr {
		if dp[0] < min {
			min = dp[0]
		}
		if dp[0] > max {
			max = dp[0]
		}
		sum += dp[0]
	}
	avg = sum / float64(len(arr))
	return min, avg, max
}

func (as *Assessment) getScore(metric string, min float64, avg float64, max float64) string {
	score := "-"
	if metric == "conns_current" { // 5% to 20%
		pct := 100 * max / float64(as.serverInfo.HostInfo.System.MemSizeMB)
		score = GetScoreByRange(pct, 5, 20)
	} else if metric == "conns_created/s" { // 120 conns created per minute, 2/second
		score = GetScoreByRange(avg, 0, 2)
	} else if metric == "cpu_idle" {
		score = fmt.Sprintf("%v", min)
	} else if metric == "cpu_iowait" || metric == "mem_page_faults" { // 15% io_wait or 15 page
		score = GetScoreByRange(max, 5, 15)
	} else if metric == "cpu_user" || metric == "cpu_system" { // under 10% is good
		score = GetScoreByRange(max, 10, 100)
	} else if strings.HasPrefix(metric, "disku_") { // under 70% is good
		score = GetScoreByRange(max, 70, 100)
	} else if strings.HasPrefix(metric, "iops_") { // avg:max ratio
		score = fmt.Sprintf("%v", 100*(1-avg/max))
	} else if metric == "latency_command" || metric == "latency_read" || metric == "latency_write" { // 20ms is good
		score = GetScoreByRange(max, 20, 100)
	} else if strings.HasPrefix(metric, "ops_") { // 2ms an op
		total := 500.0 * 128
		score = GetScoreByRange(max, 0, total)
	} else if metric == "q_queued_read" || metric == "q_queued_write" {
		cores := float64(as.serverInfo.HostInfo.System.NumCores)
		score = GetScoreByRange(max, cores, 4*cores)
	} else if metric == "scan_objects" {
		if max < 1000 {
			return "100"
		}
		keys := as.stats.TimeSeriesData["scan_keys"]
		objs := as.stats.TimeSeriesData["scan_objects"]
		mx := 0.0
		for i := range keys.DataPoints {
			scale := objs.DataPoints[i][0] / keys.DataPoints[i][0]
			if scale > mx {
				mx = scale
			}
		}
		mx--
		if mx < 0 {
			mx = 0
		} else if mx > 1 {
			mx = 1
		}
		score = fmt.Sprintf("%v", 100*(1-mx))
	} else if metric == "scan_keys" { // 1 mil/sec key scanned
		score = GetScoreByRange(max, 0, 1024.0*1024)
	} else if metric == "scan_sort" { // 1 k sorted in mem
		score = GetScoreByRange(max, 0, 1000)
	} else if metric == "ticket_avail_read" || metric == "ticket_avail_write" {
		score = fmt.Sprintf("%v", 100*min/128)
	} else if metric == "wt_modified_evicted" || metric == "wt_unmodified_evicted" {
		pages := .05 * as.wtCacheMax * (1024 * 1024 * 1024) / (4 * 1024) // 5% of WiredTiger cache
		score = GetScoreByRange(max, pages, 2*pages)
	}
	return score
}
