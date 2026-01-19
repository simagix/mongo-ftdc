// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// diagnosis.go

package ftdc

import (
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"
	"time"
)

// DiagnosisRule defines a problem pattern
type DiagnosisRule struct {
	Name        string
	Description string
	Severity    string // "critical", "warning", "info"
	Conditions  func(d *Diagnosis) bool
	Symptoms    func(d *Diagnosis) []string
	Suggestion  string
}

// DiagnosisResult holds a detected problem
type DiagnosisResult struct {
	Rule       DiagnosisRule
	Symptoms   []string
	Score      int // 0-100, lower is worse
	DetectedAt time.Time
}

// ActivitySummary holds key metrics that are always displayed
type ActivitySummary struct {
	// Operations per second
	OpsQuery   float64
	OpsInsert  float64
	OpsUpdate  float64
	OpsDelete  float64
	OpsCommand float64
	OpsTotal   float64

	// Latencies in ms (p95)
	LatencyRead    float64
	LatencyWrite   float64
	LatencyCommand float64

	// Scan activity
	ScanKeys     float64
	ScanObjects  float64
	DocsReturned float64

	// Resource utilization (percentages)
	CPUUser     float64
	CPUSystem   float64
	CPUIdle     float64
	MemResident float64 // percentage of total RAM
	CacheUsed   float64 // percentage of WT cache
	DiskUtil    float64 // max disk utilization

	// Network
	NetRequestsPerSec float64
	NetInMBps         float64
	NetOutMBps        float64

	// Connections
	ConnsActive  float64
	ConnsCurrent float64
}

// TimeRange represents a time period when an issue occurred
type TimeRange struct {
	Start time.Time
	End   time.Time
	Peak  float64
}

// AnomalyEvent represents a detected anomaly with timestamp for timeline
type AnomalyEvent struct {
	Timestamp time.Time
	EndTime   time.Time
	Duration  time.Duration
	Metric    string
	Peak      float64
	Threshold string
	Severity  string // "critical", "warning", "info"
}

// Diagnosis analyzes FTDC data for problems
type Diagnosis struct {
	stats       FTDCStats
	from        time.Time
	to          time.Time
	duration    time.Duration
	metrics     map[string]metricStats
	diskMetrics map[string]map[string]metricStats // disk -> metric -> stats
	replMetrics map[string]metricStats            // host -> lag stats
	results     []DiagnosisResult
	summary     ActivitySummary
	anomalies   []AnomalyEvent
}

// NewDiagnosis creates a new diagnosis engine
func NewDiagnosis(stats FTDCStats, from, to time.Time) *Diagnosis {
	d := &Diagnosis{
		stats:       stats,
		from:        from,
		to:          to,
		duration:    to.Sub(from),
		metrics:     make(map[string]metricStats),
		diskMetrics: make(map[string]map[string]metricStats),
		replMetrics: make(map[string]metricStats),
	}
	d.computeMetrics()
	d.computeActivitySummary()
	d.collectAnomalies()
	return d
}

// computeMetrics calculates p5/median/p95 for all metrics
func (d *Diagnosis) computeMetrics() {
	as := NewAssessment(d.stats)

	// Standard metrics
	allMetrics := serverStatusChartsLegends
	allMetrics = append(allMetrics, wiredTigerChartsLegends...)
	allMetrics = append(allMetrics, queuesChartsLegends...)
	allMetrics = append(allMetrics, transactionsChartsLegends...)
	allMetrics = append(allMetrics, flowControlChartsLegends...)
	allMetrics = append(allMetrics, replSetChartsLegends...)
	for _, sm := range systemMetricsChartsLegends {
		if strings.HasPrefix(sm, "cpu_") {
			allMetrics = append(allMetrics, sm)
		}
	}

	for _, metric := range allMetrics {
		m := as.getStatsArray(metric, d.from, d.to)
		d.metrics[metric] = m
	}

	// Disk metrics
	for disk, stats := range d.stats.DiskStats {
		d.diskMetrics[disk] = make(map[string]metricStats)
		p5, median, p95 := as.getStatsByData(stats.IOPS, d.from, d.to)
		d.diskMetrics[disk]["iops"] = as.getStatsArrayByValues("iops_"+disk, p5, median, p95)
		p5, median, p95 = as.getStatsByData(stats.Utilization, d.from, d.to)
		d.diskMetrics[disk]["util"] = as.getStatsArrayByValues("disku_"+disk, p5, median, p95)
	}

	// Replication lag metrics
	for host, lagData := range d.stats.ReplicationLags {
		p5, median, p95 := as.getStatsByData(lagData, d.from, d.to)
		d.replMetrics[host] = as.getStatsArrayByValues("repl_lag_"+host, p5, median, p95)
	}

	// tcmalloc fragmentation
	if heap, ok := d.stats.TimeSeriesData["tcmalloc_heap"]; ok && len(heap.DataPoints) > 0 {
		m := as.getTcmallocFragmentation(d.from, d.to)
		d.metrics["tcmalloc_frag"] = m
	}
}

// computeActivitySummary calculates the activity summary metrics
func (d *Diagnosis) computeActivitySummary() {
	// Operations (use median for typical activity)
	d.summary.OpsQuery = d.getMetric("ops_query").median
	d.summary.OpsInsert = d.getMetric("ops_insert").median
	d.summary.OpsUpdate = d.getMetric("ops_update").median
	d.summary.OpsDelete = d.getMetric("ops_delete").median
	d.summary.OpsCommand = d.getMetric("ops_command").median
	d.summary.OpsTotal = d.summary.OpsQuery + d.summary.OpsInsert + d.summary.OpsUpdate + d.summary.OpsDelete + d.summary.OpsCommand

	// Latencies (use p95 for worst-case)
	d.summary.LatencyRead = d.getMetric("latency_read").p95
	d.summary.LatencyWrite = d.getMetric("latency_write").p95
	d.summary.LatencyCommand = d.getMetric("latency_command").p95

	// Scan activity (use p95 to catch spikes)
	d.summary.ScanKeys = d.getMetric("scan_keys").p95
	d.summary.ScanObjects = d.getMetric("scan_objects").p95
	d.summary.DocsReturned = d.getMetric("doc_returned/s").median

	// CPU (use median for typical, show user+system)
	d.summary.CPUUser = d.getMetric("cpu_user").median
	d.summary.CPUSystem = d.getMetric("cpu_system").median
	d.summary.CPUIdle = d.getMetric("cpu_idle").median

	// Memory - getMetric returns ALREADY as percentage (Assessment converts it)
	// But only if MemSizeMB > 0, otherwise it returns raw GB
	if d.stats.ServerInfo.HostInfo.System.MemSizeMB > 0 {
		d.summary.MemResident = d.getMetric("mem_resident").median // already a percentage
	} else {
		d.summary.MemResident = -1 // N/A
	}

	// WiredTiger cache - getMetric returns ALREADY as percentage (Assessment converts it)
	// But only if MaxWTCache > 0, otherwise it returns raw GB
	if d.stats.MaxWTCache > 0 {
		d.summary.CacheUsed = d.getMetric("wt_cache_used").median // already a percentage
	} else {
		d.summary.CacheUsed = -1 // N/A
	}

	// Disk utilization - find max across all disks
	maxDiskUtil := 0.0
	for _, diskMetrics := range d.diskMetrics {
		if util, ok := diskMetrics["util"]; ok {
			if util.p95 > maxDiskUtil {
				maxDiskUtil = util.p95
			}
		}
	}
	d.summary.DiskUtil = maxDiskUtil

	// Network
	d.summary.NetRequestsPerSec = d.getMetric("net_requests").median
	d.summary.NetInMBps = d.getMetric("net_in").median
	d.summary.NetOutMBps = d.getMetric("net_out").median

	// Connections
	d.summary.ConnsActive = d.getMetric("conns_active").median
	d.summary.ConnsCurrent = d.getMetric("conns_current").median
}

// AnomalyThreshold defines threshold for anomaly detection
type AnomalyThreshold struct {
	Metric    string
	Threshold float64
	Above     bool // true = anomaly when > threshold, false = anomaly when < threshold
	Severity  string
	Label     string
}

// collectAnomalies finds all anomaly events across metrics
func (d *Diagnosis) collectAnomalies() {
	thresholds := []AnomalyThreshold{
		// Latency anomalies (above threshold is bad)
		{"latency_read", 20, true, "warning", "> 20ms"},
		{"latency_write", 20, true, "warning", "> 20ms"},
		{"latency_command", 20, true, "warning", "> 20ms"},

		// Scan anomalies (high scans indicate slow queries)
		{"scan_keys", 100000, true, "warning", "> 100K/s"},
		{"scan_objects", 100000, true, "warning", "> 100K/s"},

		// CPU anomalies
		{"cpu_idle", 30, false, "warning", "< 30%"},  // low idle = high CPU
		{"cpu_iowait", 15, true, "warning", "> 15%"}, // high iowait = disk bottleneck

		// Memory/Cache anomalies (handled specially for percentages)
		{"mem_page_faults", 20, true, "warning", "> 20/s"},

		// Queue anomalies
		{"q_queued_read", 10, true, "warning", "> 10"},
		{"q_queued_write", 10, true, "warning", "> 10"},

		// Replication
		{"write_conflicts/s", 10, true, "warning", "> 10/s"},
	}

	for _, t := range thresholds {
		if data, ok := d.stats.TimeSeriesData[t.Metric]; ok {
			var ranges []TimeRange
			if t.Above {
				ranges = d.findExceedances(data, t.Threshold)
			} else {
				ranges = d.findBelowThreshold(data, t.Threshold)
			}

			for _, r := range ranges {
				if r.End.Sub(r.Start) >= 10*time.Second { // Only significant anomalies (>10s)
					d.anomalies = append(d.anomalies, AnomalyEvent{
						Timestamp: r.Start,
						EndTime:   r.End,
						Duration:  r.End.Sub(r.Start),
						Metric:    t.Metric,
						Peak:      r.Peak,
						Threshold: t.Label,
						Severity:  t.Severity,
					})
				}
			}
		}
	}

	// Disk utilization anomalies
	for disk, stats := range d.stats.DiskStats {
		ranges := d.findExceedances(stats.Utilization, 70) // > 70% disk util
		for _, r := range ranges {
			if r.End.Sub(r.Start) >= 10*time.Second {
				d.anomalies = append(d.anomalies, AnomalyEvent{
					Timestamp: r.Start,
					EndTime:   r.End,
					Duration:  r.End.Sub(r.Start),
					Metric:    "disk_util_" + disk,
					Peak:      r.Peak,
					Threshold: "> 70%",
					Severity:  "warning",
				})
			}
		}
	}

	// Replication lag anomalies
	for host, lagData := range d.stats.ReplicationLags {
		ranges := d.findExceedances(lagData, 5) // > 5 seconds lag
		for _, r := range ranges {
			if r.End.Sub(r.Start) >= 10*time.Second {
				d.anomalies = append(d.anomalies, AnomalyEvent{
					Timestamp: r.Start,
					EndTime:   r.End,
					Duration:  r.End.Sub(r.Start),
					Metric:    "repl_lag_" + host,
					Peak:      r.Peak,
					Threshold: "> 5s",
					Severity:  "critical",
				})
			}
		}
	}

	// Sort by timestamp
	sort.Slice(d.anomalies, func(i, j int) bool {
		return d.anomalies[i].Timestamp.Before(d.anomalies[j].Timestamp)
	})
}

// findBelowThreshold finds time ranges when metric was below threshold
func (d *Diagnosis) findBelowThreshold(data TimeSeriesDoc, threshold float64) []TimeRange {
	ranges := []TimeRange{}
	if len(data.DataPoints) == 0 {
		return ranges
	}

	var currentRange *TimeRange
	for _, dp := range data.DataPoints {
		if len(dp) < 2 {
			continue
		}
		value := dp[0]
		ts := time.Unix(0, int64(dp[1])*int64(time.Millisecond))

		if value < threshold {
			if currentRange == nil {
				currentRange = &TimeRange{Start: ts, Peak: value}
			}
			if value < currentRange.Peak { // For "below" threshold, lower is worse
				currentRange.Peak = value
			}
			currentRange.End = ts
		} else if currentRange != nil {
			ranges = append(ranges, *currentRange)
			currentRange = nil
		}
	}
	if currentRange != nil {
		ranges = append(ranges, *currentRange)
	}
	return ranges
}

// findExceedances finds time ranges when a metric exceeded a threshold
func (d *Diagnosis) findExceedances(data TimeSeriesDoc, threshold float64) []TimeRange {
	ranges := []TimeRange{}
	if len(data.DataPoints) == 0 {
		return ranges
	}

	var currentRange *TimeRange
	for _, dp := range data.DataPoints {
		if len(dp) < 2 {
			continue
		}
		value := dp[0]
		ts := time.Unix(0, int64(dp[1])*int64(time.Millisecond))

		if value > threshold {
			if currentRange == nil {
				currentRange = &TimeRange{Start: ts, Peak: value}
			}
			if value > currentRange.Peak {
				currentRange.Peak = value
			}
			currentRange.End = ts
		} else if currentRange != nil {
			ranges = append(ranges, *currentRange)
			currentRange = nil
		}
	}
	if currentRange != nil {
		ranges = append(ranges, *currentRange)
	}
	return ranges
}

// summarizeExceedances returns a human-readable summary of when issues occurred
func (d *Diagnosis) summarizeExceedances(ranges []TimeRange) string {
	if len(ranges) == 0 {
		return ""
	}

	// Calculate total duration of issues
	var totalDuration time.Duration
	for _, r := range ranges {
		totalDuration += r.End.Sub(r.Start)
	}

	if len(ranges) == 1 {
		r := ranges[0]
		duration := r.End.Sub(r.Start)
		if duration < time.Minute {
			return fmt.Sprintf("at %s (peak: %.1f)", r.Start.Format("15:04:05"), r.Peak)
		}
		return fmt.Sprintf("%s to %s (%.0fm, peak: %.1f)",
			r.Start.Format("Jan 02 15:04"), r.End.Format("15:04"),
			duration.Minutes(), r.Peak)
	}

	// Multiple ranges - summarize
	return fmt.Sprintf("%d occurrences over %.0fm (peak: %.1f)",
		len(ranges), totalDuration.Minutes(), d.maxPeak(ranges))
}

func (d *Diagnosis) maxPeak(ranges []TimeRange) float64 {
	max := 0.0
	for _, r := range ranges {
		if r.Peak > max {
			max = r.Peak
		}
	}
	return max
}

// getReplLagExceedances finds when replication lag exceeded threshold for a host
func (d *Diagnosis) getReplLagExceedances(host string, threshold float64) []TimeRange {
	if lagData, ok := d.stats.ReplicationLags[host]; ok {
		return d.findExceedances(lagData, threshold)
	}
	return nil
}

// getMetricExceedances finds when a standard metric exceeded threshold
func (d *Diagnosis) getMetricExceedances(metric string, threshold float64) []TimeRange {
	if data, ok := d.stats.TimeSeriesData[metric]; ok {
		return d.findExceedances(data, threshold)
	}
	return nil
}

// getMetric returns metric stats, safe for missing metrics
func (d *Diagnosis) getMetric(name string) metricStats {
	if m, ok := d.metrics[name]; ok {
		return m
	}
	return metricStats{score: 101}
}

// hasProblematicScore returns true if score < threshold
func (d *Diagnosis) hasProblematicScore(name string, threshold int) bool {
	m := d.getMetric(name)
	return m.score < threshold && m.score < 101
}

// DiagnosisRules defines all problem patterns
var DiagnosisRules = []DiagnosisRule{
	{
		Name:        "Connection Pool Misconfiguration",
		Description: "High connection count consuming memory - each connection uses ~1MB RAM",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			connScore := d.getMetric("conns_current").score
			// Flag if connections alone are problematic (score < 50)
			// OR if both connections and memory are concerning
			memScore := d.getMetric("mem_resident").score
			return connScore < 50 || (connScore < 70 && memScore < 50)
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			conn := d.getMetric("conns_current")
			mem := d.getMetric("mem_resident")
			connCreated := d.getMetric("conns_created/s")
			if conn.score < 70 {
				// Add timing info for connection spikes
				exceedances := d.getMetricExceedances("conns_current", conn.median*1.5) // 50% above median
				timing := d.summarizeExceedances(exceedances)
				if timing != "" {
					symptoms = append(symptoms, fmt.Sprintf("High connections: p95=%.0f (score: %d) — %s",
						conn.p95, conn.score, timing))
				} else {
					symptoms = append(symptoms, fmt.Sprintf("High connections: p95=%.0f (score: %d)", conn.p95, conn.score))
				}
			}
			if connCreated.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("Frequent connection churn: %.1f new conns/s (score: %d)", connCreated.median, connCreated.score))
			}
			if mem.score < 70 {
				symptoms = append(symptoms, fmt.Sprintf("Memory pressure: p95=%.0f%% (score: %d)", mem.p95, mem.score))
			}
			latency := d.getMetric("latency_read")
			if latency.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("Elevated read latency: p95=%.0fms (score: %d)", latency.p95, latency.score))
			}
			return symptoms
		},
		Suggestion: "Reduce maxPoolSize in application connection strings. Consider using connection pooling middleware. Each connection uses ~1MB RAM.",
	},
	{
		Name:        "Replication Lag Issues",
		Description: "Secondary nodes falling behind primary, potentially causing stale reads",
		Severity:    "critical",
		Conditions: func(d *Diagnosis) bool {
			for _, lag := range d.replMetrics {
				if lag.score < 50 {
					return true
				}
			}
			return false
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			for host, lag := range d.replMetrics {
				if lag.score < 101 && lag.p95 > 0 {
					// Find when lag exceeded 5 seconds (threshold for concern)
					exceedances := d.getReplLagExceedances(host, 5.0)
					timing := d.summarizeExceedances(exceedances)
					if timing != "" {
						symptoms = append(symptoms, fmt.Sprintf("Replication lag on %s: p95=%.1fs (score: %d) — %s",
							host, lag.p95, lag.score, timing))
					} else {
						symptoms = append(symptoms, fmt.Sprintf("Replication lag on %s: p95=%.1fs (score: %d)",
							host, lag.p95, lag.score))
					}
				}
			}
			// Check related metrics
			if m := d.getMetric("ops_insert"); m.p95 > 1000 {
				symptoms = append(symptoms, fmt.Sprintf("High insert rate: %.0f ops/s", m.p95))
			}
			if m := d.getMetric("ops_update"); m.p95 > 1000 {
				symptoms = append(symptoms, fmt.Sprintf("High update rate: %.0f ops/s", m.p95))
			}
			return symptoms
		},
		Suggestion: "Check network latency between nodes. Consider upgrading secondary hardware. Review oplog size. For Atlas, consider scaling up the cluster.",
	},
	{
		Name:        "Working Set Exceeds RAM",
		Description: "Active data larger than available memory causing excessive disk I/O",
		Severity:    "critical",
		Conditions: func(d *Diagnosis) bool {
			cacheScore := d.getMetric("wt_cache_used").score
			evictMod := d.getMetric("wt_modified_evicted").score
			evictUnmod := d.getMetric("wt_unmodified_evicted").score
			pageFaults := d.getMetric("mem_page_faults").score
			return cacheScore < 30 || (evictMod < 50 && evictUnmod < 50) || pageFaults < 50
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("wt_cache_used"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("WiredTiger cache saturated: p95=%.0f%% (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("wt_modified_evicted"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High dirty page evictions: p95=%.0f (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("wt_unmodified_evicted"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High clean page evictions: p95=%.0f (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("mem_page_faults"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High page faults: p95=%.0f (score: %d)", m.p95, m.score))
			}
			// Disk I/O correlation
			for disk, metrics := range d.diskMetrics {
				if util := metrics["util"]; util.score < 50 {
					symptoms = append(symptoms, fmt.Sprintf("High disk utilization on %s: p95=%.0f%% (score: %d)", disk, util.p95, util.score))
				}
			}
			return symptoms
		},
		Suggestion: "Add more RAM or scale up instance. Create indexes to reduce working set. Archive old data. Consider sharding for horizontal scaling.",
	},
	{
		Name:        "Write Contention",
		Description: "High transaction conflicts and aborts indicate hot documents or poor schema design",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			conflicts := d.getMetric("write_conflicts/s")
			aborted := d.getMetric("txn_aborted/s")
			return conflicts.score < 50 || aborted.score < 50
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("write_conflicts/s"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Write conflicts: p95=%.0f/s (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("txn_aborted/s"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Transaction aborts: p95=%.0f/s (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("txn_inactive"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Inactive transactions: p95=%.0f (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("queued_write"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("Write queue depth: p95=%.0f (score: %d)", m.p95, m.score))
			}
			return symptoms
		},
		Suggestion: "Review document update patterns to avoid hot documents. Use optimistic concurrency control. Consider redesigning schema to distribute writes. Reduce transaction scope.",
	},
	{
		Name:        "Missing Indexes",
		Description: "High query targeting ratios indicate collection scans or inefficient index usage",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			keysRatio := d.getMetric("query_targeting_keys")
			objsRatio := d.getMetric("query_targeting_objects")
			scanKeys := d.getMetric("scan_keys")
			return keysRatio.score < 50 || objsRatio.score < 50 || scanKeys.score < 50
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("query_targeting_keys"); m.score < 101 && m.p95 > 1 {
				symptoms = append(symptoms, fmt.Sprintf("Keys examined per doc returned: p95=%.0f:1 (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("query_targeting_objects"); m.score < 101 && m.p95 > 1 {
				symptoms = append(symptoms, fmt.Sprintf("Docs examined per doc returned: p95=%.0f:1 (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("scan_keys"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("Keys scanned: p95=%.0f/s (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("scan_objects"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("Objects scanned per key: avg=%.0f:1 (score: %d)", m.p95, m.score))
			}
			return symptoms
		},
		Suggestion: "Run explain() on slow queries. Create compound indexes matching query patterns. Use covered queries where possible. Consider using $hint for query plan control.",
	},
	{
		Name:        "Memory Fragmentation",
		Description: "tcmalloc memory fragmentation causing inefficient memory usage",
		Severity:    "info",
		Conditions: func(d *Diagnosis) bool {
			frag := d.getMetric("tcmalloc_frag")
			return frag.score < 50
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("tcmalloc_frag"); m.score < 101 {
				symptoms = append(symptoms, fmt.Sprintf("Memory fragmentation: p95=%.0f%% (score: %d)", m.p95, m.score))
			}
			return symptoms
		},
		Suggestion: "Consider restarting mongod during maintenance window. This is often caused by varied allocation sizes. Monitor for memory growth over time.",
	},
	{
		Name:        "CPU Saturation",
		Description: "High CPU usage may indicate inefficient queries or under-provisioned hardware",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			idle := d.getMetric("cpu_idle")
			user := d.getMetric("cpu_user")
			system := d.getMetric("cpu_system")
			return idle.score < 30 || user.score < 30 || system.score < 30
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("cpu_idle"); m.score < 50 && m.p5 < 100 {
				symptoms = append(symptoms, fmt.Sprintf("Low CPU idle: p5=%.0f%% (score: %d)", m.p5, m.score))
			}
			if m := d.getMetric("cpu_user"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High CPU user: p95=%.0f%% (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("cpu_system"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High CPU system: p95=%.0f%% (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("cpu_iowait"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("High CPU I/O wait: p95=%.0f%% (score: %d)", m.p95, m.score))
			}
			return symptoms
		},
		Suggestion: "Profile slow queries. Add appropriate indexes. Consider upgrading CPU or scaling horizontally. Check for runaway operations with currentOp().",
	},
	{
		Name:        "Disk I/O Bottleneck",
		Description: "Storage subsystem struggling to keep up with I/O demands",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			for _, metrics := range d.diskMetrics {
				if util := metrics["util"]; util.score < 30 {
					return true
				}
			}
			return false
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			for disk, metrics := range d.diskMetrics {
				if util := metrics["util"]; util.score < 50 {
					symptoms = append(symptoms, fmt.Sprintf("Disk %s utilization: p95=%.0f%% (score: %d)", disk, util.p95, util.score))
				}
				if iops := metrics["iops"]; iops.score < 50 {
					symptoms = append(symptoms, fmt.Sprintf("Disk %s IOPS spikes: p95=%.0f (score: %d)", disk, iops.p95, iops.score))
				}
			}
			if m := d.getMetric("cpu_iowait"); m.score < 50 {
				symptoms = append(symptoms, fmt.Sprintf("CPU I/O wait: p95=%.0f%%", m.p95))
			}
			return symptoms
		},
		Suggestion: "Upgrade to faster storage (NVMe SSD). Increase IOPS provisioning. Reduce working set size. Ensure readahead is optimized for MongoDB workload.",
	},
	{
		Name:        "Flow Control Activated",
		Description: "MongoDB 7.0+ flow control is throttling writes due to lagging members",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			lagged := d.getMetric("flowctl_lagged_count")
			acquiring := d.getMetric("flowctl_acquiring_us")
			return (lagged.score < 101 && lagged.p95 > 0) || (acquiring.score < 50)
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("flowctl_lagged_count"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Lagged members: p95=%.0f (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("flowctl_acquiring_us"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Flow control wait: p95=%.0fms (score: %d)", m.p95/1000, m.score))
			}
			return symptoms
		},
		Suggestion: "Address replication lag on secondary nodes. Check network connectivity. Consider upgrading secondary hardware to match primary.",
	},
	{
		Name:        "Admission Control Queuing",
		Description: "MongoDB 7.0+ admission control is queuing operations",
		Severity:    "warning",
		Conditions: func(d *Diagnosis) bool {
			readOut := d.getMetric("queues_read_out")
			writeOut := d.getMetric("queues_write_out")
			return readOut.score < 50 || writeOut.score < 50
		},
		Symptoms: func(d *Diagnosis) []string {
			symptoms := []string{}
			if m := d.getMetric("queues_read_out"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Read tickets in use: p95=%.0f (score: %d)", m.p95, m.score))
			}
			if m := d.getMetric("queues_write_out"); m.score < 101 && m.p95 > 0 {
				symptoms = append(symptoms, fmt.Sprintf("Write tickets in use: p95=%.0f (score: %d)", m.p95, m.score))
			}
			return symptoms
		},
		Suggestion: "High ticket usage indicates system under load. Scale up resources or optimize queries. Review concurrent operation patterns.",
	},
}

// Run executes all diagnosis rules
func (d *Diagnosis) Run() []DiagnosisResult {
	d.results = []DiagnosisResult{}
	for _, rule := range DiagnosisRules {
		if rule.Conditions(d) {
			symptoms := rule.Symptoms(d)
			if len(symptoms) > 0 {
				// Calculate overall score (average of symptom-related metrics)
				score := d.calculateRuleScore(rule)
				d.results = append(d.results, DiagnosisResult{
					Rule:       rule,
					Symptoms:   symptoms,
					Score:      score,
					DetectedAt: time.Now(),
				})
			}
		}
	}
	// Sort by severity then score
	sort.Slice(d.results, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
		if sevOrder[d.results[i].Rule.Severity] != sevOrder[d.results[j].Rule.Severity] {
			return sevOrder[d.results[i].Rule.Severity] < sevOrder[d.results[j].Rule.Severity]
		}
		return d.results[i].Score < d.results[j].Score
	})
	return d.results
}

func (d *Diagnosis) calculateRuleScore(rule DiagnosisRule) int {
	// Return worst score among related metrics
	worst := 100
	for name, m := range d.metrics {
		if m.score < worst && m.score < 101 {
			// Only count if metric name suggests relation to rule
			if strings.Contains(strings.ToLower(rule.Name), strings.Split(name, "_")[0]) {
				worst = m.score
			}
		}
	}
	if worst == 100 {
		// Fallback: find any problematic metric
		for _, m := range d.metrics {
			if m.score < worst && m.score < 101 {
				worst = m.score
			}
		}
	}
	return worst
}

// PrintReport outputs diagnosis to stdout
func (d *Diagnosis) PrintReport() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                         FTDC DIAGNOSTIC REPORT                               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("📅 Analysis Period: %s to %s\n", d.from.Format("2006-01-02 15:04:05"), d.to.Format("2006-01-02 15:04:05"))
	fmt.Printf("⏱  Duration: %s\n", d.duration.Round(time.Second))
	fmt.Printf("🖥  Host: %s\n", d.stats.ServerInfo.HostInfo.System.Hostname)
	fmt.Printf("📊 MongoDB: v%s\n", d.stats.ServerInfo.BuildInfo.Version)
	fmt.Println()

	// Activity Summary
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("📊 ACTIVITY SUMMARY")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Printf("   ⚡ Operations/sec:  query=%.0f  insert=%.0f  update=%.0f  delete=%.0f  cmd=%.0f  (total=%.0f)\n",
		d.summary.OpsQuery, d.summary.OpsInsert, d.summary.OpsUpdate, d.summary.OpsDelete, d.summary.OpsCommand, d.summary.OpsTotal)
	fmt.Printf("   ⏱  Latency p95 (ms): read=%.1f  write=%.1f  command=%.1f\n",
		d.summary.LatencyRead, d.summary.LatencyWrite, d.summary.LatencyCommand)
	fmt.Printf("   🔍 Scans p95/s:      keys=%.0f  objects=%.0f  docs_returned=%.0f\n",
		d.summary.ScanKeys, d.summary.ScanObjects, d.summary.DocsReturned)
	// Format resource percentages, showing N/A for invalid values
	ramStr := "N/A"
	if d.summary.MemResident >= 0 {
		ramStr = fmt.Sprintf("%.0f%%", d.summary.MemResident)
	}
	cacheStr := "N/A"
	if d.summary.CacheUsed >= 0 {
		cacheStr = fmt.Sprintf("%.0f%%", d.summary.CacheUsed)
	}
	fmt.Printf("   💻 Resources:        CPU=%.0f%%  RAM=%s  Cache=%s  Disk=%.0f%%\n",
		d.summary.CPUUser+d.summary.CPUSystem, ramStr, cacheStr, d.summary.DiskUtil)
	fmt.Printf("   🌐 Network:          requests=%.0f/s  in=%.1f MB/s  out=%.1f MB/s\n",
		d.summary.NetRequestsPerSec, d.summary.NetInMBps, d.summary.NetOutMBps)
	fmt.Printf("   🔗 Connections:      active=%.0f  current=%.0f\n",
		d.summary.ConnsActive, d.summary.ConnsCurrent)
	fmt.Println()

	// Anomaly Timeline (top 10 events)
	if len(d.anomalies) > 0 {
		fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
		fmt.Println("📅 ANOMALY TIMELINE (for slow query log correlation)")
		fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
		fmt.Println()
		maxEvents := 10
		if len(d.anomalies) < maxEvents {
			maxEvents = len(d.anomalies)
		}
		for i := 0; i < maxEvents; i++ {
			a := d.anomalies[i]
			peakStr := d.formatPeakValue(a.Peak, a.Metric)
			fmt.Printf("   %s  %-20s  peak=%-10s  duration=%s\n",
				a.Timestamp.Format("Jan 02 15:04:05"),
				a.Metric,
				peakStr,
				a.Duration.Round(time.Second))
		}
		if len(d.anomalies) > 10 {
			fmt.Printf("   ... and %d more events (see HTML report for full timeline)\n", len(d.anomalies)-10)
		}
		fmt.Println()
	}

	if len(d.results) == 0 {
		fmt.Println("✅ No significant issues detected!")
		fmt.Println()
		return
	}

	fmt.Printf("⚠️  Found %d potential issue(s):\n", len(d.results))
	fmt.Println()

	for i, result := range d.results {
		severityIcon := map[string]string{"critical": "🔴", "warning": "🟡", "info": "🔵"}[result.Rule.Severity]
		fmt.Printf("─────────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("%s [%d] %s (%s)\n", severityIcon, i+1, result.Rule.Name, strings.ToUpper(result.Rule.Severity))
		fmt.Printf("─────────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("   %s\n\n", result.Rule.Description)
		fmt.Println("   Symptoms:")
		for _, s := range result.Symptoms {
			fmt.Printf("   • %s\n", s)
		}
		fmt.Println()
		fmt.Printf("   💡 Suggestion: %s\n", result.Rule.Suggestion)
		fmt.Println()
	}
}

// GenerateHTML creates an HTML report
func (d *Diagnosis) GenerateHTML(filename string) error {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FTDC Diagnostic Report</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #c9d1d9;
            --text-secondary: #8b949e;
            --accent-green: #238636;
            --accent-red: #da3633;
            --accent-yellow: #d29922;
            --accent-blue: #58a6ff;
            --border-color: #30363d;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.6;
            padding: 2rem;
        }
        .container { max-width: 1000px; margin: 0 auto; }
        header {
            text-align: center;
            padding: 2rem;
            background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
            border-radius: 12px;
            border: 1px solid var(--border-color);
            margin-bottom: 2rem;
        }
        h1 {
            font-size: 1.8rem;
            font-weight: 600;
            margin-bottom: 0.5rem;
            background: linear-gradient(90deg, var(--accent-blue), #a371f7);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .meta {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-top: 1.5rem;
            text-align: left;
        }
        .meta-item {
            background: var(--bg-primary);
            padding: 0.75rem 1rem;
            border-radius: 6px;
            border: 1px solid var(--border-color);
        }
        .meta-label { color: var(--text-secondary); font-size: 0.75rem; text-transform: uppercase; }
        .meta-value { font-size: 0.9rem; margin-top: 0.25rem; }
        .summary {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 1.5rem;
            margin-bottom: 2rem;
            border: 1px solid var(--border-color);
        }
        .summary h2 { font-size: 1rem; margin-bottom: 1rem; }
        .no-issues {
            text-align: center;
            padding: 2rem;
            color: var(--accent-green);
            font-size: 1.1rem;
        }
        .activity-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 1rem;
            margin-top: 1rem;
        }
        .activity-card {
            background: var(--bg-tertiary);
            border-radius: 8px;
            padding: 1rem;
            border: 1px solid var(--border-color);
        }
        .activity-card h4 {
            font-size: 0.75rem;
            color: var(--text-secondary);
            text-transform: uppercase;
            margin-bottom: 0.75rem;
            letter-spacing: 0.5px;
        }
        .activity-row {
            display: flex;
            justify-content: space-between;
            padding: 0.25rem 0;
            font-size: 0.85rem;
        }
        .activity-label { color: var(--text-secondary); }
        .activity-value { font-weight: 500; }
        .activity-value.highlight { color: var(--accent-blue); }
        .activity-value.warn { color: var(--accent-yellow); }
        .activity-value.good { color: var(--accent-green); }
        .timeline-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 1rem;
            font-size: 0.85rem;
        }
        .timeline-table th {
            text-align: left;
            padding: 0.75rem;
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            font-weight: 500;
            text-transform: uppercase;
            font-size: 0.7rem;
            letter-spacing: 0.5px;
        }
        .timeline-table td {
            padding: 0.5rem 0.75rem;
            border-bottom: 1px solid var(--border-color);
        }
        .timeline-table tr:hover {
            background: var(--bg-tertiary);
        }
        .timeline-severity {
            display: inline-block;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            margin-right: 0.5rem;
        }
        .timeline-severity.critical { background: var(--accent-red); }
        .timeline-severity.warning { background: var(--accent-yellow); }
        .timeline-severity.info { background: var(--accent-blue); }
        .timeline-metric { font-family: monospace; }
        .timeline-peak { color: var(--accent-yellow); font-weight: 500; }
        .no-anomalies {
            text-align: center;
            padding: 2rem;
            color: var(--text-secondary);
        }
        .issue {
            background: var(--bg-secondary);
            border-radius: 12px;
            margin-bottom: 1.5rem;
            border: 1px solid var(--border-color);
            overflow: hidden;
        }
        .issue-header {
            padding: 1rem 1.5rem;
            display: flex;
            align-items: center;
            gap: 0.75rem;
            border-bottom: 1px solid var(--border-color);
        }
        .severity {
            width: 12px;
            height: 12px;
            border-radius: 50%;
        }
        .severity.critical { background: var(--accent-red); box-shadow: 0 0 10px var(--accent-red); }
        .severity.warning { background: var(--accent-yellow); box-shadow: 0 0 10px var(--accent-yellow); }
        .severity.info { background: var(--accent-blue); box-shadow: 0 0 10px var(--accent-blue); }
        .issue-title { font-weight: 600; flex: 1; }
        .issue-badge {
            font-size: 0.7rem;
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            text-transform: uppercase;
            font-weight: 600;
        }
        .issue-badge.critical { background: rgba(218, 54, 51, 0.2); color: var(--accent-red); }
        .issue-badge.warning { background: rgba(210, 153, 34, 0.2); color: var(--accent-yellow); }
        .issue-badge.info { background: rgba(88, 166, 255, 0.2); color: var(--accent-blue); }
        .issue-body { padding: 1.5rem; }
        .issue-desc { color: var(--text-secondary); margin-bottom: 1rem; }
        .symptoms { margin-bottom: 1.5rem; }
        .symptoms h3 { font-size: 0.85rem; color: var(--text-secondary); margin-bottom: 0.75rem; }
        .symptom {
            background: var(--bg-tertiary);
            padding: 0.5rem 0.75rem;
            border-radius: 4px;
            margin-bottom: 0.5rem;
            font-size: 0.85rem;
            border-left: 3px solid var(--accent-blue);
        }
        .suggestion {
            background: rgba(35, 134, 54, 0.1);
            border: 1px solid var(--accent-green);
            border-radius: 6px;
            padding: 1rem;
        }
        .suggestion h3 { font-size: 0.85rem; color: var(--accent-green); margin-bottom: 0.5rem; }
        .suggestion p { font-size: 0.85rem; }
        footer {
            text-align: center;
            padding: 2rem;
            color: var(--text-secondary);
            font-size: 0.8rem;
        }
        footer a { color: var(--accent-blue); text-decoration: none; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔍 FTDC Diagnostic Report</h1>
            <div class="meta">
                <div class="meta-item">
                    <div class="meta-label">Analysis Period</div>
                    <div class="meta-value">{{.FromTime}} → {{.ToTime}}</div>
                </div>
                <div class="meta-item">
                    <div class="meta-label">Duration</div>
                    <div class="meta-value">{{.Duration}}</div>
                </div>
                <div class="meta-item">
                    <div class="meta-label">Host</div>
                    <div class="meta-value">{{.Hostname}}</div>
                </div>
                <div class="meta-item">
                    <div class="meta-label">MongoDB Version</div>
                    <div class="meta-value">{{.MongoVersion}}</div>
                </div>
            </div>
        </header>

        <div class="summary">
            <h2>📊 Activity Summary</h2>
            <div class="activity-grid">
                <div class="activity-card">
                    <h4>⚡ Operations/sec (median)</h4>
                    <div class="activity-row">
                        <span class="activity-label">Query</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.OpsQuery}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Insert</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.OpsInsert}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Update</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.OpsUpdate}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Delete</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.OpsDelete}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Command</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.OpsCommand}}</span>
                    </div>
                    <div class="activity-row" style="border-top: 1px solid var(--border-color); margin-top: 0.5rem; padding-top: 0.5rem;">
                        <span class="activity-label"><strong>Total</strong></span>
                        <span class="activity-value highlight"><strong>{{printf "%.0f" .Summary.OpsTotal}}</strong></span>
                    </div>
                </div>
                <div class="activity-card">
                    <h4>⏱ Latency p95 (ms)</h4>
                    <div class="activity-row">
                        <span class="activity-label">Read</span>
                        <span class="activity-value {{if gt .Summary.LatencyRead 20.0}}warn{{else}}good{{end}}">{{printf "%.1f" .Summary.LatencyRead}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Write</span>
                        <span class="activity-value {{if gt .Summary.LatencyWrite 20.0}}warn{{else}}good{{end}}">{{printf "%.1f" .Summary.LatencyWrite}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Command</span>
                        <span class="activity-value {{if gt .Summary.LatencyCommand 20.0}}warn{{else}}good{{end}}">{{printf "%.1f" .Summary.LatencyCommand}}</span>
                    </div>
                </div>
                <div class="activity-card">
                    <h4>🔍 Query Efficiency (p95)</h4>
                    <div class="activity-row">
                        <span class="activity-label">Keys Scanned/s</span>
                        <span class="activity-value {{if gt .Summary.ScanKeys 100000.0}}warn{{end}}">{{printf "%.0f" .Summary.ScanKeys}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Objects Scanned/s</span>
                        <span class="activity-value {{if gt .Summary.ScanObjects 100000.0}}warn{{end}}">{{printf "%.0f" .Summary.ScanObjects}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Docs Returned/s</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.DocsReturned}}</span>
                    </div>
                </div>
                <div class="activity-card">
                    <h4>💻 Resources</h4>
                    <div class="activity-row">
                        <span class="activity-label">CPU (user+sys)</span>
                        <span class="activity-value {{if gt (addFloat .Summary.CPUUser .Summary.CPUSystem) 70.0}}warn{{else}}good{{end}}">{{printf "%.0f" (addFloat .Summary.CPUUser .Summary.CPUSystem)}}%</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Memory</span>
                        {{if lt .Summary.MemResident 0.0}}<span class="activity-value">N/A</span>{{else}}<span class="activity-value {{if gt .Summary.MemResident 80.0}}warn{{else}}good{{end}}">{{printf "%.0f" .Summary.MemResident}}%</span>{{end}}
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">WT Cache</span>
                        {{if lt .Summary.CacheUsed 0.0}}<span class="activity-value">N/A</span>{{else}}<span class="activity-value {{if gt .Summary.CacheUsed 80.0}}warn{{else}}good{{end}}">{{printf "%.0f" .Summary.CacheUsed}}%</span>{{end}}
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Disk (max)</span>
                        <span class="activity-value {{if gt .Summary.DiskUtil 70.0}}warn{{else}}good{{end}}">{{printf "%.0f" .Summary.DiskUtil}}%</span>
                    </div>
                </div>
                <div class="activity-card">
                    <h4>🌐 Network</h4>
                    <div class="activity-row">
                        <span class="activity-label">Requests/s</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.NetRequestsPerSec}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">In</span>
                        <span class="activity-value">{{printf "%.1f" .Summary.NetInMBps}} MB/s</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Out</span>
                        <span class="activity-value">{{printf "%.1f" .Summary.NetOutMBps}} MB/s</span>
                    </div>
                </div>
                <div class="activity-card">
                    <h4>🔗 Connections</h4>
                    <div class="activity-row">
                        <span class="activity-label">Active</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.ConnsActive}}</span>
                    </div>
                    <div class="activity-row">
                        <span class="activity-label">Current</span>
                        <span class="activity-value">{{printf "%.0f" .Summary.ConnsCurrent}}</span>
                    </div>
                </div>
            </div>
        </div>

        <div class="summary">
            <h2>🩺 Diagnosis</h2>
            {{if eq (len .Results) 0}}
            <div class="no-issues">✅ No significant issues detected!</div>
            {{else}}
            <p>Found <strong>{{len .Results}}</strong> potential issue(s) requiring attention.</p>
            {{end}}
        </div>

        {{range $i, $result := .Results}}
        <div class="issue">
            <div class="issue-header">
                <div class="severity {{$result.Rule.Severity}}"></div>
                <div class="issue-title">{{$result.Rule.Name}}</div>
                <span class="issue-badge {{$result.Rule.Severity}}">{{$result.Rule.Severity}}</span>
            </div>
            <div class="issue-body">
                <p class="issue-desc">{{$result.Rule.Description}}</p>
                <div class="symptoms">
                    <h3>Symptoms Detected</h3>
                    {{range $result.Symptoms}}
                    <div class="symptom">{{.}}</div>
                    {{end}}
                </div>
                <div class="suggestion">
                    <h3>💡 Recommendation</h3>
                    <p>{{$result.Rule.Suggestion}}</p>
                </div>
            </div>
        </div>
        {{end}}

        {{if gt (len .Anomalies) 0}}
        <div class="summary">
            <h2>📅 Anomaly Timeline</h2>
            <p style="color: var(--text-secondary); margin-bottom: 1rem;">
                Use these timestamps to correlate with slow query logs and application metrics.
            </p>
            <table class="timeline-table">
                <thead>
                    <tr>
                        <th>Time</th>
                        <th>Duration</th>
                        <th>Metric</th>
                        <th>Peak</th>
                        <th>Threshold</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Anomalies}}
                    <tr>
                        <td>
                            <span class="timeline-severity {{.Severity}}"></span>
                            {{.Timestamp.Format "Jan 02 15:04:05"}}
                        </td>
                        <td>{{formatDuration .Duration}}</td>
                        <td class="timeline-metric">{{.Metric}}</td>
                        <td class="timeline-peak">{{formatPeak .Peak .Metric}}</td>
                        <td>{{.Threshold}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        <footer>
            Generated by <a href="https://github.com/simagix/mongo-ftdc">mongo-ftdc</a> • {{.GeneratedAt}}
        </footer>
    </div>
</body>
</html>`

	data := struct {
		FromTime     string
		ToTime       string
		Duration     string
		Hostname     string
		MongoVersion string
		Results      []DiagnosisResult
		Summary      ActivitySummary
		Anomalies    []AnomalyEvent
		GeneratedAt  string
	}{
		FromTime:     d.from.Format("2006-01-02 15:04:05"),
		ToTime:       d.to.Format("2006-01-02 15:04:05"),
		Duration:     d.duration.Round(time.Second).String(),
		Hostname:     d.stats.ServerInfo.HostInfo.System.Hostname,
		MongoVersion: d.stats.ServerInfo.BuildInfo.Version,
		Results:      d.results,
		Summary:      d.summary,
		Anomalies:    d.anomalies,
		GeneratedAt:  time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	funcMap := template.FuncMap{
		"addFloat": func(a, b float64) float64 { return a + b },
		"formatDuration": func(d time.Duration) string {
			if d < time.Minute {
				return fmt.Sprintf("%.0fs", d.Seconds())
			} else if d < time.Hour {
				return fmt.Sprintf("%.0fm %.0fs", d.Minutes(), d.Seconds()-float64(int(d.Minutes()))*60)
			}
			return fmt.Sprintf("%.0fh %.0fm", d.Hours(), d.Minutes()-float64(int(d.Hours()))*60)
		},
		"formatPeak": func(peak float64, metric string) string {
			if strings.Contains(metric, "latency") {
				return fmt.Sprintf("%.1fms", peak)
			} else if strings.Contains(metric, "cpu") || strings.Contains(metric, "disk_util") {
				return fmt.Sprintf("%.0f%%", peak)
			} else if strings.Contains(metric, "repl_lag") {
				return fmt.Sprintf("%.1fs", peak)
			} else if peak >= 1000000 {
				return fmt.Sprintf("%.1fM", peak/1000000)
			} else if peak >= 1000 {
				return fmt.Sprintf("%.1fK", peak/1000)
			}
			return fmt.Sprintf("%.0f", peak)
		},
	}

	t, err := template.New("report").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, data)
}

// GetResults returns the diagnosis results
func (d *Diagnosis) GetResults() []DiagnosisResult {
	return d.results
}

// formatPeakValue formats peak value based on metric type
func (d *Diagnosis) formatPeakValue(peak float64, metric string) string {
	if strings.Contains(metric, "latency") {
		return fmt.Sprintf("%.1fms", peak)
	} else if strings.Contains(metric, "cpu") || strings.Contains(metric, "disk_util") {
		return fmt.Sprintf("%.0f%%", peak)
	} else if strings.Contains(metric, "repl_lag") {
		return fmt.Sprintf("%.1fs", peak)
	} else if peak >= 1000000 {
		return fmt.Sprintf("%.1fM", peak/1000000)
	} else if peak >= 1000 {
		return fmt.Sprintf("%.1fK", peak/1000)
	}
	return fmt.Sprintf("%.0f", peak)
}
