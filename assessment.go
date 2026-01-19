// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// assessment.go

package ftdc

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ScoreFormula holds metric info
type ScoreFormula struct {
	formula string
	label   string
	low     int
	high    int
}

// FormulaMap holds low and high watermarks
var FormulaMap = map[string]ScoreFormula{
	"conns_created/s":       {label: "conns_created/s", formula: "conns_created/s", low: 0, high: 5},
	"conns_current":         {label: "conns_current %%", formula: "1MB*(p95 of conns_current)/RAM", low: 5, high: 20},
	"cpu_idle":              {label: "cpu_idle %%", formula: "p5 of cpu_idle", low: 50, high: 80},
	"cpu_iowait":            {label: "cpu_iowait %%", formula: "p95 of cpu_iowait", low: 5, high: 15},
	"cpu_system":            {label: "cpu_system %%", formula: "p95 of cpu_system", low: 5, high: 15},
	"cpu_user":              {label: "cpu_user %%", formula: "p95 of cpu_user", low: 50, high: 70},
	"disku_":                {label: "disku_&lt;dev&gt; %%", formula: "p95 of disku_&lt;dev&gt;", low: 50, high: 90},
	"iops_":                 {label: "iops_&lt;dev&gt;", formula: "(p95 of iops_&lt;dev&gt;)/(avg of iops_<dev>)", low: 2, high: 4},
	"latency_command":       {label: "latency_command (ms)", formula: "p95 of latency_command", low: 20, high: 100},
	"latency_read":          {label: "latency_read (ms)", formula: "p95 of latency_read", low: 20, high: 100},
	"latency_write":         {label: "latency_write (ms)", formula: "p95 of latency_write", low: 20, high: 100},
	"mem_page_faults":       {label: "mem_page_faults", formula: "p95 of mem_page_faults", low: 10, high: 20},
	"mem_resident":          {label: "mem_resident %%", formula: "mem_resident/RAM", low: 70, high: 90},
	"ops_":                  {label: "ops_&lt;op&gt;", formula: "ops_&lt;op&gt;", low: 0, high: 64000},
	"queued_read":           {label: "queued_read", formula: "p95 of queued_read", low: 1, high: 5},
	"queued_write":          {label: "queued_write", formula: "p95 of queued_write", low: 1, high: 5},
	"scan_keys":             {label: "scan_keys", formula: "scan_keys", low: 0, high: (1024 * 1024)},
	"scan_objects":          {label: "scan_objects", formula: "avg of [](scan_objects/scan_keys)", low: 2, high: 5},
	"scan_sort":             {label: "scan_sort", formula: "scan_sort", low: 0, high: 1000},
	"wt_cache_used":         {label: "wt_cache_used %%", formula: "(p95 of wt_cache_used)/wt_cache_max", low: 80, high: 95},
	"wt_cache_dirty":        {label: "wt_cache_dirty %%", formula: "(p95 of wt_cache_dirty)/wt_cache_max", low: 5, high: 20},
	"wt_dhandles_active":    {label: "wt_dhandles_active", formula: "(p95 of wt_dhandles_active)", low: 16000, high: 20000},
	"wt_modified_evicted":   {label: "wt_modified_evicted  %%", formula: "(p95 of wt_modified_evicted)/(pages of wt_cache_max)", low: 5, high: 10},
	"wt_unmodified_evicted": {label: "wt_unmodified_evicted  %%", formula: "(p95 of wt_unmodified_evicted)/(pages of wt_cache_max)", low: 5, high: 10},

	// Query Targeting - ratio of keys/docs examined to docs returned (ideal is 1:1)
	"query_targeting_keys":    {label: "query_targeting_keys", formula: "p95 of (keys_examined/docs_returned)", low: 10, high: 100},
	"query_targeting_objects": {label: "query_targeting_objects", formula: "p95 of (docs_examined/docs_returned)", low: 10, high: 100},
	"write_conflicts/s":       {label: "write_conflicts/s", formula: "p95 of write_conflicts/s", low: 10, high: 100},

	// Replication - lag in seconds (per host)
	"repl_lag_": {label: "repl_lag_&lt;host&gt; (s)", formula: "p95 of replication lag", low: 5, high: 30},

	// MongoDB 7.0+ Admission Control - % of tickets in use
	"queues_read_out":  {label: "queues_read_out %%", formula: "(p95 of queues_read_out)/queues_read_total", low: 50, high: 90},
	"queues_write_out": {label: "queues_write_out %%", formula: "(p95 of queues_write_out)/queues_write_total", low: 50, high: 90},

	// Transactions - inactive txns hold resources, high abort rate indicates contention
	"txn_inactive":  {label: "txn_inactive", formula: "p95 of txn_inactive", low: 5, high: 20},
	"txn_aborted/s": {label: "txn_aborted/s", formula: "p95 of txn_aborted/s", low: 10, high: 50},

	// tcmalloc Memory - fragmentation ratio
	"tcmalloc_fragmentation": {label: "tcmalloc_frag %%", formula: "(heap-in_use)/heap", low: 20, high: 50},

	// Flow Control - lagged members and time acquiring tickets
	"flowctl_lagged_count": {label: "flowctl_lagged_count", formula: "p95 of flowctl_lagged_count", low: 1, high: 3},
	"flowctl_acquiring_us": {label: "flowctl_acquiring (ms)", formula: "p95 of flowctl_acquiring_us/1000", low: 100, high: 1000},
}

// Assessment stores timeserie data
type Assessment struct {
	blocks        int
	maxCachePages int
	stats         FTDCStats
	verbose       bool
}

type metricStats struct {
	label  string
	median float64
	p5     float64
	p95    float64
	score  int
}

// NewAssessment returns assessment object
func NewAssessment(stats FTDCStats) *Assessment {
	assessment := Assessment{blocks: 1, stats: stats}
	cores := stats.ServerInfo.HostInfo.System.NumCores
	m := FormulaMap["queued_read"]
	m.low = cores
	m.high = 5 * cores
	FormulaMap["queued_read"] = m
	m = FormulaMap["queued_write"]
	m.low = cores
	m.high = 5 * cores
	FormulaMap["queued_write"] = m
	assessment.maxCachePages = int(.05 * float64(stats.MaxWTCache) * (1024 * 1024 * 1024) / (4 * 1024)) // 5% of WiredTiger cache
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

	if to.Sub(from) <= 72*time.Hour {
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
		metrics = append(metrics, queuesChartsLegends...)
		metrics = append(metrics, transactionsChartsLegends...)
		metrics = append(metrics, flowControlChartsLegends...)
		metrics = append(metrics, replSetChartsLegends...)
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
		// tcmalloc fragmentation assessment
		if heap, ok := as.stats.TimeSeriesData["tcmalloc_heap"]; ok && len(heap.DataPoints) > 0 {
			inUse := as.stats.TimeSeriesData["tcmalloc_in_use"]
			if len(inUse.DataPoints) > 0 {
				m := as.getTcmallocFragmentation(from, to)
				if m.score < 101 || as.verbose {
					marr = append(marr, m)
				}
			}
		}
		for k, v := range as.stats.DiskStats {
			p5, median, p95 := as.getStatsByData(v.IOPS, from, to)
			if p95 == 0 {
				continue
			}
			m := as.getStatsArrayByValues("iops_"+k, p5, median, p95)
			if m.score < 101 || as.verbose {
				marr = append(marr, m)
			}
			p5, median, p95 = as.getStatsByData(v.Utilization, from, to)
			m = as.getStatsArrayByValues("disku_"+k, p5, median, p95)
			if m.score < 101 || as.verbose {
				marr = append(marr, m)
			}
		}
		// Replication lags assessment (stored per-host in separate map)
		for k, v := range as.stats.ReplicationLags {
			p5, median, p95 := as.getStatsByData(v, from, to)
			if p95 == 0 {
				continue
			}
			m := as.getStatsArrayByValues("repl_lag_"+k, p5, median, p95)
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
			arr = append(arr, []interface{}{v.label, v.score, v.p5, v.median, v.p95}...)
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
		rowList = append(rowList, []interface{}{"Assessment is available when date range is less than 3 days"})
	}
	return map[string]interface{}{"columns": headerList, "type": "table", "rows": rowList}
}

func (as *Assessment) getStatsArray(metric string, from time.Time, to time.Time) metricStats {
	p5, median, p95 := as.getStatsByData(as.stats.TimeSeriesData[metric], from, to)
	return as.getStatsArrayByValues(metric, p5, median, p95)
}

func (as *Assessment) getStatsArrayByValues(metric string, p5 float64, median float64, p95 float64) metricStats {
	var score = 101
	label := metric
	if strings.HasSuffix(label, "modified_evicted") {
		label = strings.ReplaceAll(label, "modified_evicted", "mod_evicted")
	}
	if as.stats.MaxWTCache > 0 && (metric == "wt_cache_used" || metric == "wt_cache_dirty") {
		u := 100 * p95 / float64(as.stats.MaxWTCache)
		if metric == "wt_cache_used" { // 80% to 100%
			score = GetScoreByRange(u, 80, 95)
		} else if metric == "wt_cache_dirty" { // 5% to 20%
			score = GetScoreByRange(u, 5, 20)
		}
		return metricStats{label: label + " %", score: score, p5: math.Round(100 * p5 / float64(as.stats.MaxWTCache)),
			median: math.Round(100 * median / float64(as.stats.MaxWTCache)), p95: math.Round(100 * p95 / float64(as.stats.MaxWTCache))}
	} else if as.stats.ServerInfo.HostInfo.System.MemSizeMB > 0 && metric == "mem_resident" {
		total := float64(as.stats.ServerInfo.HostInfo.System.MemSizeMB) / 1024
		u := 100 * p95 / total
		score = GetScoreByRange(u, 70, 90)
		return metricStats{label: label + " %", score: score, p5: math.Round(100 * p5 / total),
			median: math.Round(100 * median / total), p95: math.Round(100 * p95 / total)}
	}
	score = as.getScore(metric, p5, median, p95)
	if strings.HasPrefix(metric, "cpu_") || strings.HasPrefix(metric, "disku_") {
		return metricStats{label: label + " %", score: score, p5: math.Round(p5),
			median: math.Round(median), p95: math.Round(p95)}
	}
	return metricStats{label: label, score: score, p5: math.Round(p5),
		median: math.Round(median), p95: math.Round(p95)}
}

func (as *Assessment) getTcmallocFragmentation(from time.Time, to time.Time) metricStats {
	heap := as.stats.TimeSeriesData["tcmalloc_heap"]
	inUse := as.stats.TimeSeriesData["tcmalloc_in_use"]
	heapStats := FilterTimeSeriesData(heap, from, to)
	inUseStats := FilterTimeSeriesData(inUse, from, to)
	if len(heapStats.DataPoints) == 0 || len(inUseStats.DataPoints) == 0 {
		return metricStats{label: "tcmalloc_frag %", score: 101}
	}
	// Calculate fragmentation ratio for each data point
	ratios := []float64{}
	minLen := len(heapStats.DataPoints)
	if len(inUseStats.DataPoints) < minLen {
		minLen = len(inUseStats.DataPoints)
	}
	for i := 0; i < minLen; i++ {
		h := heapStats.DataPoints[i][0]
		u := inUseStats.DataPoints[i][0]
		if h > 0 {
			frag := 100 * (h - u) / h
			ratios = append(ratios, frag)
		}
	}
	if len(ratios) == 0 {
		return metricStats{label: "tcmalloc_frag %", score: 101}
	}
	sort.Float64s(ratios)
	end := len(ratios) - 1
	samples := float64(len(ratios) + 1)
	p5Idx := int(samples * 0.05)
	if p5Idx > end {
		p5Idx = end
	}
	medianIdx := int(samples * 0.5)
	if medianIdx > end {
		medianIdx = end
	}
	p95Idx := int(samples * 0.95)
	if p95Idx > end {
		p95Idx = end
	}
	p5, median, p95 := ratios[p5Idx], ratios[medianIdx], ratios[p95Idx]
	score := GetScoreByRange(p95, 20, 50) // 20% fragmentation is okay, 50% is bad
	return metricStats{label: "tcmalloc_frag %", score: score, p5: math.Round(p5), median: math.Round(median), p95: math.Round(p95)}
}

func (as *Assessment) getStatsByData(data TimeSeriesDoc, from time.Time, to time.Time) (float64, float64, float64) {
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
	end := len(arr) - 1
	samples := float64(len(arr) + 1)
	p5 := int(samples * 0.05)
	if p5 > end {
		p5 = end
	}
	median := int(samples * .5)
	if median > end {
		median = end
	}
	p95 := int(samples * .95)
	if p95 > len(arr)-1 {
		p95 = len(arr) - 1
	}
	if p95 > end {
		p95 = end
	}
	return arr[p5], arr[median], arr[p95]
}

func (as *Assessment) getScore(metric string, p5 float64, median float64, p95 float64) int {
	score := 101
	met := metric
	if strings.HasPrefix(met, "disku_") {
		met = "disku_"
	} else if strings.HasPrefix(met, "iops_") {
		met = "iops_"
	} else if strings.HasPrefix(met, "ops_") {
		met = "ops_"
	} else if strings.HasPrefix(met, "repl_lag_") {
		met = "repl_lag_"
	}
	if FormulaMap[met].label == "" {
		return score
	}
	lwm := float64(FormulaMap[met].low)
	hwm := float64(FormulaMap[met].high)
	if metric == "conns_created/s" { // 300 conns created per minute, 5/second
		score = GetScoreByRange(median, lwm, hwm)
	} else if metric == "conns_current" { // 5% to 20%
		pct := 100 * p95 / float64(as.stats.ServerInfo.HostInfo.System.MemSizeMB)
		score = GetScoreByRange(pct, lwm, hwm)
	} else if metric == "cpu_idle" {
		if math.IsNaN(p5) {
			return score
		}
		score = 100 - GetScoreByRange(p5, lwm, hwm)
	} else if metric == "cpu_iowait" || metric == "cpu_system" { // 5% - 15% io_wait
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "cpu_user" { // under 50% is good
		score = GetScoreByRange(p95, lwm, hwm)
	} else if strings.HasPrefix(metric, "disku_") { // under 50% is good
		score = GetScoreByRange(p95, lwm, hwm)
	} else if strings.HasPrefix(metric, "iops_") { // median:p95 ratio
		if p95 < 100 {
			return score
		}
		u := p95 / median
		score = GetScoreByRange(u, lwm, hwm)
	} else if strings.HasPrefix(metric, "latency_") { // 20ms is good
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "mem_page_faults" { // 10 page faults
		score = GetScoreByRange(p95, lwm, hwm)
	} else if strings.HasPrefix(metric, "ops_") { // 2ms an op
		score = GetScoreByRange(p95, lwm, hwm)
	} else if strings.HasPrefix(metric, "q_queued_") {
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "scan_keys" { // 1 mil/sec key scanned
		score = GetScoreByRange(p95, 0, 1024.0*1024)
	} else if metric == "scan_objects" {
		if p95 < 1000 {
			return 100
		}
		keys := as.stats.TimeSeriesData["scan_keys"]
		objs := as.stats.TimeSeriesData["scan_objects"]
		// Calculate average ratio of scan_objects/scan_keys
		sum := 0.0
		count := 0
		for i := range keys.DataPoints {
			if keys.DataPoints[i][0] > 0 { // avoid division by zero
				sum += objs.DataPoints[i][0] / keys.DataPoints[i][0]
				count++
			}
		}
		avg := 0.0
		if count > 0 {
			avg = sum / float64(count)
		}
		score = GetScoreByRange(avg, lwm, hwm)
	} else if metric == "scan_sort" { // 1 k sorted in mem
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "wt_dhandles_active" {
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "wt_modified_evicted" || metric == "wt_unmodified_evicted" {
		score = GetScoreByRange(p95/float64(as.maxCachePages), lwm, hwm)
	} else if metric == "query_targeting_keys" || metric == "query_targeting_objects" {
		// Query targeting ratio: keys/docs examined vs docs returned
		// Ideal is 1:1, >10 is concerning, >100 is bad
		if p95 > 0 {
			score = GetScoreByRange(p95, lwm, hwm)
		}
	} else if metric == "write_conflicts/s" {
		// Write conflicts indicate transaction contention
		score = GetScoreByRange(p95, lwm, hwm)
	} else if strings.HasPrefix(metric, "repl_lag_") {
		// Replication lag in seconds: <5s is good, >30s is critical
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "queues_read_out" || metric == "queues_write_out" {
		// MongoDB 7.0+ Admission Control: % of tickets in use
		// Need to calculate as percentage of total tickets
		totalMetric := strings.Replace(metric, "_out", "_total", 1)
		if total, ok := as.stats.TimeSeriesData[totalMetric]; ok && len(total.DataPoints) > 0 {
			// Use first total value as reference (usually constant)
			totalTickets := total.DataPoints[0][0]
			if totalTickets > 0 {
				pct := 100 * p95 / totalTickets
				score = GetScoreByRange(pct, lwm, hwm)
			}
		}
	} else if metric == "txn_inactive" {
		// Inactive transactions hold resources - should be minimal
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "txn_aborted/s" {
		// High abort rate indicates contention
		score = GetScoreByRange(p95, lwm, hwm)
	} else if metric == "flowctl_lagged_count" {
		// Flow control lagged members: >0 means some members are lagging
		if p95 > 0 {
			score = GetScoreByRange(p95, lwm, hwm)
		}
	} else if metric == "flowctl_acquiring_us" {
		// Time spent acquiring flow control tickets (microseconds)
		// Convert to ms for scoring: 100ms is concerning, 1000ms is bad
		ms := p95 / 1000
		score = GetScoreByRange(ms, lwm, hwm)
	}
	return score
}
