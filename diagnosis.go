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

// TimeRange represents a time period when an issue occurred
type TimeRange struct {
	Start time.Time
	End   time.Time
	Peak  float64
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
				symptoms = append(symptoms, fmt.Sprintf("Objects scanned per key: max=%.0f:1 (score: %d)", m.p95, m.score))
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
            padding: 3rem;
            color: var(--accent-green);
            font-size: 1.2rem;
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
            <h2>📊 Summary</h2>
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
		GeneratedAt  string
	}{
		FromTime:     d.from.Format("2006-01-02 15:04:05"),
		ToTime:       d.to.Format("2006-01-02 15:04:05"),
		Duration:     d.duration.Round(time.Second).String(),
		Hostname:     d.stats.ServerInfo.HostInfo.System.Hostname,
		MongoVersion: d.stats.ServerInfo.BuildInfo.Version,
		Results:      d.results,
		GeneratedAt:  time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	t, err := template.New("report").Parse(tmpl)
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

