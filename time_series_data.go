// Copyright 2019-present Kuei-chun Chen. All rights reserved.
// time_series_data.go

package ftdc

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const mb = 1024.0 * 1024
const gb = 1024.0 * 1024 * 1024

// TimeSeriesDoc -
type TimeSeriesDoc struct {
	Target     string      `json:"target"`
	DataPoints [][]float64 `json:"datapoints"`
}

// RangeDoc -
type RangeDoc struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// TargetDoc -
type TargetDoc struct {
	Target string `json:"target"`
	RefID  string `json:"refId"`
	Type   string `json:"type"`
}

// QueryRequest -
type QueryRequest struct {
	Timezone string      `json:"timezone"`
	Range    RangeDoc    `json:"range"`
	Targets  []TargetDoc `json:"targets"`
}

var serverStatusChartsLegends = []string{
	"mem_resident", "mem_virtual", "mem_page_faults",
	"conns_active", "conns_available", "conns_current", "conns_created/s",
	"latency_read", "latency_write", "latency_command",
	"net_in", "net_out", "net_requests", "net_physical_in", "net_physical_out",
	"ops_query", "ops_insert", "ops_update", "ops_delete", "ops_getmore", "ops_command",
	"q_active_read", "q_active_write", "q_queued_read", "q_queued_write",
	"scan_keys", "scan_objects", "scan_sort",
}
var wiredTigerChartsLegends = []string{
	"wt_blkmgr_read", "wt_blkmgr_written", "wt_blkmgr_written_checkpoint",
	"wt_cache_max", "wt_cache_used", "wt_cache_dirty",
	"wt_modified_evicted", "wt_unmodified_evicted", "wt_cache_read_in", "wt_cache_written_from",
	"wt_dhandles_active", "ticket_avail_read", "ticket_avail_write",
}

// MongoDB 7.0+ Queues (Admission Control)
var queuesChartsLegends = []string{
	"queues_read_out", "queues_read_available", "queues_read_total",
	"queues_write_out", "queues_write_available", "queues_write_total",
}

// Transactions
var transactionsChartsLegends = []string{
	"txn_active", "txn_inactive", "txn_open",
	"txn_aborted/s", "txn_committed/s", "txn_started/s",
}

// tcmalloc Memory
var tcmallocChartsLegends = []string{
	"tcmalloc_in_use", "tcmalloc_allocated", "tcmalloc_heap", "tcmalloc_physical",
}

// Flow Control
var flowControlChartsLegends = []string{
	"flowctl_rate_limit", "flowctl_acquiring_us", "flowctl_lagged_count",
}
var systemMetricsChartsLegends = []string{
	"cpu_idle", "cpu_iowait", "cpu_nice", "cpu_softirq", "cpu_steal", "cpu_system", "cpu_user",
	"disks_utils", "disks_iops", "io_in_progress", "read_time_ms", "write_time_ms", "io_queued_ms"}
var replSetChartsLegends = []string{"replication_lags"}

// dataPoint returns a pre-allocated 2-element slice [value, timestamp]
func dataPoint(v, t float64) []float64 {
	if v < 0 {
		v = 0
	}
	return []float64{v, t}
}

// initTimeSeriesMap creates a map with pre-allocated slices for each legend
func initTimeSeriesMap(legends []string, capacity int) map[string]*TimeSeriesDoc {
	m := make(map[string]*TimeSeriesDoc, len(legends))
	for _, legend := range legends {
		m[legend] = &TimeSeriesDoc{
			Target:     legend,
			DataPoints: make([][]float64, 0, capacity),
		}
	}
	return m
}

// toValueMap converts pointer map to value map for return
func toValueMap(m map[string]*TimeSeriesDoc) map[string]TimeSeriesDoc {
	result := make(map[string]TimeSeriesDoc, len(m))
	for k, v := range m {
		result[k] = *v
	}
	return result
}

func getReplSetGetStatusTimeSeriesDoc(replSetGetStatusList []ReplSetStatusDoc, legends *[]string) (map[string]TimeSeriesDoc, map[string]TimeSeriesDoc) {
	var timeSeriesData = map[string]TimeSeriesDoc{}
	var replicationLags = map[string]TimeSeriesDoc{}
	var hosts []string
	var ts int64

	for _, legend := range replSetChartsLegends {
		timeSeriesData[legend] = TimeSeriesDoc{legend, [][]float64{}}
	}
	for _, stat := range replSetGetStatusList {
		if len(stat.Members) == 0 { // missing, shouldn't happen
			continue
		}
		ts = 0
		sort.Slice(stat.Members, func(i, j int) bool { return stat.Members[i].Name < stat.Members[j].Name })
		if len(hosts) == 0 || len(hosts) != len(stat.Members) {
			hosts = hosts[:0]
			for n, mb := range stat.Members {
				hostname := fmt.Sprintf("host-%v", n)
				a := strings.Index(mb.Name, ".")
				b := strings.LastIndex(mb.Name, ":")
				var legend string
				if a < 0 || b < 0 {
					legend = mb.Name
				} else {
					legend = mb.Name[0:a] + mb.Name[b:]
				}
				if len(*legends) == 0 {
					log.Println(hostname, legend)
				}
				hosts = append(hosts, hostname)
				timeSeriesData[legend] = TimeSeriesDoc{legend, [][]float64{}}
				node := "repl_" + strconv.Itoa(n)
				timeSeriesData[node] = TimeSeriesDoc{node, [][]float64{}}
			}
			continue
		}

		for _, mb := range stat.Members {
			if mb.State == 1 {
				ts = GetOptime(mb.Optime)
				break
			}
		}

		if ts == 0 {
			continue
		} else {
			t := float64(stat.Date.UnixNano() / 1000 / 1000)
			for i, mb := range stat.Members {
				v := 0.0
				if mb.State == 2 { // SECONDARY
					v = float64(ts - GetOptime(mb.Optime))
				} else if mb.State == 1 { // PRIMARY
					v = 0
				} else if mb.State == 7 { // ARBITER
					continue
				}
				x := replicationLags[hosts[i]]
				x.DataPoints = append(x.DataPoints, dataPoint(v, t))
				replicationLags[hosts[i]] = x
			}
		}
	}

	*legends = hosts
	return timeSeriesData, replicationLags
}

func getSystemMetricsTimeSeriesDoc(systemMetricsList []SystemMetricsDoc) (map[string]TimeSeriesDoc, map[string]DiskStats) {
	n := len(systemMetricsList)
	tsData := initTimeSeriesMap(systemMetricsChartsLegends, n)
	diskStats := make(map[string]DiskStats)
	var pstat SystemMetricsDoc

	for i, stat := range systemMetricsList {
		if i == 0 {
			pstat = stat
			continue
		}

		t := float64(stat.Start.UnixNano() / (1000 * 1000))
		seconds := stat.Start.Sub(pstat.Start).Seconds()
		if seconds < 1 {
			seconds = 1
		}

		for k, disk := range stat.Disks {
			pdisk := pstat.Disks[k]
			u := 100 * float64(disk.IOTimeMS-pdisk.IOTimeMS) / 1000
			iops := float64(disk.Reads+disk.Writes-(pdisk.Reads+pdisk.Writes)) / seconds

			ds := diskStats[k]
			ds.Utilization.DataPoints = append(ds.Utilization.DataPoints, dataPoint(u, t))
			ds.IOPS.DataPoints = append(ds.IOPS.DataPoints, dataPoint(iops, t))
			ds.IOInProgress.DataPoints = append(ds.IOInProgress.DataPoints, dataPoint(float64(disk.IOInProgress), t))
			ds.ReadTimeMS.DataPoints = append(ds.ReadTimeMS.DataPoints, dataPoint(float64(disk.ReadTimeMS-pdisk.ReadTimeMS), t))
			ds.WriteTimeMS.DataPoints = append(ds.WriteTimeMS.DataPoints, dataPoint(float64(disk.WriteTimeMS-pdisk.WriteTimeMS), t))
			ds.IOQueuedMS.DataPoints = append(ds.IOQueuedMS.DataPoints, dataPoint(float64(disk.IOQueuedMS-pdisk.IOQueuedMS), t))
			diskStats[k] = ds
		}

		totalMS := float64(stat.CPU.IOWaitMS + stat.CPU.IdleMS + stat.CPU.NiceMS + stat.CPU.SoftirqMS + stat.CPU.StealMS + stat.CPU.SystemMS + stat.CPU.UserMS)
		ptotalMS := float64(pstat.CPU.IOWaitMS + pstat.CPU.IdleMS + pstat.CPU.NiceMS + pstat.CPU.SoftirqMS + pstat.CPU.StealMS + pstat.CPU.SystemMS + pstat.CPU.UserMS)
		deltaTotalMS := totalMS - ptotalMS
		if deltaTotalMS == 0 {
			deltaTotalMS = 1
		}

		tsData["cpu_idle"].DataPoints = append(tsData["cpu_idle"].DataPoints, dataPoint(100*float64(stat.CPU.IdleMS-pstat.CPU.IdleMS)/deltaTotalMS, t))
		tsData["cpu_iowait"].DataPoints = append(tsData["cpu_iowait"].DataPoints, dataPoint(100*float64(stat.CPU.IOWaitMS-pstat.CPU.IOWaitMS)/deltaTotalMS, t))
		tsData["cpu_system"].DataPoints = append(tsData["cpu_system"].DataPoints, dataPoint(100*float64(stat.CPU.SystemMS-pstat.CPU.SystemMS)/deltaTotalMS, t))
		tsData["cpu_user"].DataPoints = append(tsData["cpu_user"].DataPoints, dataPoint(100*float64(stat.CPU.UserMS-pstat.CPU.UserMS)/deltaTotalMS, t))
		tsData["cpu_nice"].DataPoints = append(tsData["cpu_nice"].DataPoints, dataPoint(100*float64(stat.CPU.NiceMS-pstat.CPU.NiceMS)/deltaTotalMS, t))
		tsData["cpu_steal"].DataPoints = append(tsData["cpu_steal"].DataPoints, dataPoint(100*float64(stat.CPU.StealMS-pstat.CPU.StealMS)/deltaTotalMS, t))
		tsData["cpu_softirq"].DataPoints = append(tsData["cpu_softirq"].DataPoints, dataPoint(100*float64(stat.CPU.SoftirqMS-pstat.CPU.SoftirqMS)/deltaTotalMS, t))

		pstat = stat
	}
	return toValueMap(tsData), diskStats
}

// getAllServerStatusTimeSeriesDoc processes all server status metrics in a single pass
func getAllServerStatusTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	n := len(serverStatusList)
	if n == 0 {
		return map[string]TimeSeriesDoc{}
	}

	// Combine all legends for single-pass processing
	allLegends := make([]string, 0, len(serverStatusChartsLegends)+len(wiredTigerChartsLegends)+
		len(queuesChartsLegends)+len(transactionsChartsLegends)+len(tcmallocChartsLegends)+len(flowControlChartsLegends))
	allLegends = append(allLegends, serverStatusChartsLegends...)
	allLegends = append(allLegends, wiredTigerChartsLegends...)
	allLegends = append(allLegends, queuesChartsLegends...)
	allLegends = append(allLegends, transactionsChartsLegends...)
	allLegends = append(allLegends, tcmallocChartsLegends...)
	allLegends = append(allLegends, flowControlChartsLegends...)

	ts := initTimeSeriesMap(allLegends, n)
	var pstat ServerStatusDoc

	for i, stat := range serverStatusList {
		if stat.Uptime <= pstat.Uptime {
			pstat = stat
			continue
		}

		t := float64(stat.LocalTime.UnixNano() / (1000 * 1000))

		// === Server Status metrics (gauges - no delta needed) ===
		ts["mem_resident"].DataPoints = append(ts["mem_resident"].DataPoints, dataPoint(float64(stat.Mem.Resident)/1024, t))
		ts["mem_virtual"].DataPoints = append(ts["mem_virtual"].DataPoints, dataPoint(float64(stat.Mem.Virtual)/1024, t))
		ts["conns_active"].DataPoints = append(ts["conns_active"].DataPoints, dataPoint(float64(stat.Connections.Active), t))
		ts["conns_available"].DataPoints = append(ts["conns_available"].DataPoints, dataPoint(float64(stat.Connections.Available), t))
		ts["conns_current"].DataPoints = append(ts["conns_current"].DataPoints, dataPoint(float64(stat.Connections.Current), t))
		ts["q_active_read"].DataPoints = append(ts["q_active_read"].DataPoints, dataPoint(float64(stat.GlobalLock.ActiveClients.Readers), t))
		ts["q_active_write"].DataPoints = append(ts["q_active_write"].DataPoints, dataPoint(float64(stat.GlobalLock.ActiveClients.Writers), t))
		ts["q_queued_read"].DataPoints = append(ts["q_queued_read"].DataPoints, dataPoint(float64(stat.GlobalLock.CurrentQueue.Readers), t))
		ts["q_queued_write"].DataPoints = append(ts["q_queued_write"].DataPoints, dataPoint(float64(stat.GlobalLock.CurrentQueue.Writers), t))

		// Latencies (computed)
		var r, w, c float64
		if stat.OpLatencies.Reads.Ops > 0 {
			r = float64(stat.OpLatencies.Reads.Latency) / float64(stat.OpLatencies.Reads.Ops) / 1000
		}
		if stat.OpLatencies.Writes.Ops > 0 {
			w = float64(stat.OpLatencies.Writes.Latency) / float64(stat.OpLatencies.Writes.Ops) / 1000
		}
		if stat.OpLatencies.Commands.Ops > 0 {
			c = float64(stat.OpLatencies.Commands.Latency) / float64(stat.OpLatencies.Commands.Ops) / 1000
		}
		ts["latency_read"].DataPoints = append(ts["latency_read"].DataPoints, dataPoint(r, t))
		ts["latency_write"].DataPoints = append(ts["latency_write"].DataPoints, dataPoint(w, t))
		ts["latency_command"].DataPoints = append(ts["latency_command"].DataPoints, dataPoint(c, t))

		// === WiredTiger metrics (gauges) ===
		ts["wt_cache_max"].DataPoints = append(ts["wt_cache_max"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.MaxBytesConfigured)/gb, t))
		ts["wt_cache_used"].DataPoints = append(ts["wt_cache_used"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.CurrentlyInCache)/gb, t))
		ts["wt_cache_dirty"].DataPoints = append(ts["wt_cache_dirty"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.TrackedDirtyBytes)/gb, t))
		ts["wt_dhandles_active"].DataPoints = append(ts["wt_dhandles_active"].DataPoints, dataPoint(float64(stat.WiredTiger.DataHandle.Active), t))
		ts["ticket_avail_read"].DataPoints = append(ts["ticket_avail_read"].DataPoints, dataPoint(float64(stat.WiredTiger.ConcurrentTransactions.Read.Available), t))
		ts["ticket_avail_write"].DataPoints = append(ts["ticket_avail_write"].DataPoints, dataPoint(float64(stat.WiredTiger.ConcurrentTransactions.Write.Available), t))

		// === Queues (MongoDB 7.0+) ===
		ts["queues_read_out"].DataPoints = append(ts["queues_read_out"].DataPoints, dataPoint(float64(stat.Queues.Execution.Read.Out), t))
		ts["queues_read_available"].DataPoints = append(ts["queues_read_available"].DataPoints, dataPoint(float64(stat.Queues.Execution.Read.Available), t))
		ts["queues_read_total"].DataPoints = append(ts["queues_read_total"].DataPoints, dataPoint(float64(stat.Queues.Execution.Read.TotalTickets), t))
		ts["queues_write_out"].DataPoints = append(ts["queues_write_out"].DataPoints, dataPoint(float64(stat.Queues.Execution.Write.Out), t))
		ts["queues_write_available"].DataPoints = append(ts["queues_write_available"].DataPoints, dataPoint(float64(stat.Queues.Execution.Write.Available), t))
		ts["queues_write_total"].DataPoints = append(ts["queues_write_total"].DataPoints, dataPoint(float64(stat.Queues.Execution.Write.TotalTickets), t))

		// === Transactions (gauges) ===
		ts["txn_active"].DataPoints = append(ts["txn_active"].DataPoints, dataPoint(float64(stat.Transactions.CurrentActive), t))
		ts["txn_inactive"].DataPoints = append(ts["txn_inactive"].DataPoints, dataPoint(float64(stat.Transactions.CurrentInactive), t))
		ts["txn_open"].DataPoints = append(ts["txn_open"].DataPoints, dataPoint(float64(stat.Transactions.CurrentOpen), t))

		// === tcmalloc (gauges) ===
		ts["tcmalloc_in_use"].DataPoints = append(ts["tcmalloc_in_use"].DataPoints, dataPoint(float64(stat.Tcmalloc.Generic.BytesInUseByApp)/gb, t))
		ts["tcmalloc_allocated"].DataPoints = append(ts["tcmalloc_allocated"].DataPoints, dataPoint(float64(stat.Tcmalloc.Generic.CurrentAllocatedBytes)/gb, t))
		ts["tcmalloc_heap"].DataPoints = append(ts["tcmalloc_heap"].DataPoints, dataPoint(float64(stat.Tcmalloc.Generic.HeapSize)/gb, t))
		ts["tcmalloc_physical"].DataPoints = append(ts["tcmalloc_physical"].DataPoints, dataPoint(float64(stat.Tcmalloc.Generic.PhysicalMemoryUsed)/gb, t))

		// === Flow Control (gauge) ===
		ts["flowctl_rate_limit"].DataPoints = append(ts["flowctl_rate_limit"].DataPoints, dataPoint(float64(stat.FlowControl.TargetRateLimit), t))

		// === Delta-based metrics (need previous stat) ===
		if i > 0 {
			seconds := math.Round(stat.LocalTime.Sub(pstat.LocalTime).Seconds())
			if seconds < 1 {
				seconds = 1
			}

			// Server Status deltas
			ts["mem_page_faults"].DataPoints = append(ts["mem_page_faults"].DataPoints, dataPoint(float64(stat.ExtraInfo.PageFaults-pstat.ExtraInfo.PageFaults)/seconds, t))
			ts["conns_created/s"].DataPoints = append(ts["conns_created/s"].DataPoints, dataPoint(float64(stat.Connections.TotalCreated-pstat.Connections.TotalCreated)/seconds, t))
			ts["net_in"].DataPoints = append(ts["net_in"].DataPoints, dataPoint(float64(stat.Network.BytesIn-pstat.Network.BytesIn)/mb/seconds, t))
			ts["net_out"].DataPoints = append(ts["net_out"].DataPoints, dataPoint(float64(stat.Network.BytesOut-pstat.Network.BytesOut)/mb/seconds, t))
			ts["net_requests"].DataPoints = append(ts["net_requests"].DataPoints, dataPoint(float64(stat.Network.NumRequests-pstat.Network.NumRequests)/seconds, t))
			ts["net_physical_in"].DataPoints = append(ts["net_physical_in"].DataPoints, dataPoint(float64(stat.Network.PhysicalBytesIn-pstat.Network.PhysicalBytesIn)/mb/seconds, t))
			ts["net_physical_out"].DataPoints = append(ts["net_physical_out"].DataPoints, dataPoint(float64(stat.Network.PhysicalBytesOut-pstat.Network.PhysicalBytesOut)/mb/seconds, t))
			ts["ops_query"].DataPoints = append(ts["ops_query"].DataPoints, dataPoint(float64(stat.OpCounters.Query-pstat.OpCounters.Query)/seconds, t))
			ts["ops_insert"].DataPoints = append(ts["ops_insert"].DataPoints, dataPoint(float64(stat.OpCounters.Insert-pstat.OpCounters.Insert)/seconds, t))
			ts["ops_update"].DataPoints = append(ts["ops_update"].DataPoints, dataPoint(float64(stat.OpCounters.Update-pstat.OpCounters.Update)/seconds, t))
			ts["ops_delete"].DataPoints = append(ts["ops_delete"].DataPoints, dataPoint(float64(stat.OpCounters.Delete-pstat.OpCounters.Delete)/seconds, t))
			ts["ops_getmore"].DataPoints = append(ts["ops_getmore"].DataPoints, dataPoint(float64(stat.OpCounters.Getmore-pstat.OpCounters.Getmore)/seconds, t))
			ts["ops_command"].DataPoints = append(ts["ops_command"].DataPoints, dataPoint(float64(stat.OpCounters.Command-pstat.OpCounters.Command)/seconds, t))
			ts["scan_keys"].DataPoints = append(ts["scan_keys"].DataPoints, dataPoint(float64(stat.Metrics.QueryExecutor.Scanned-pstat.Metrics.QueryExecutor.Scanned)/seconds, t))
			ts["scan_objects"].DataPoints = append(ts["scan_objects"].DataPoints, dataPoint(float64(stat.Metrics.QueryExecutor.ScannedObjects-pstat.Metrics.QueryExecutor.ScannedObjects)/seconds, t))
			ts["scan_sort"].DataPoints = append(ts["scan_sort"].DataPoints, dataPoint(float64(stat.Metrics.Operation.ScanAndOrder-pstat.Metrics.Operation.ScanAndOrder)/seconds, t))

			// WiredTiger deltas
			ts["wt_blkmgr_read"].DataPoints = append(ts["wt_blkmgr_read"].DataPoints, dataPoint(float64(stat.WiredTiger.BlockManager.BytesRead-pstat.WiredTiger.BlockManager.BytesRead)/mb/seconds, t))
			ts["wt_blkmgr_written"].DataPoints = append(ts["wt_blkmgr_written"].DataPoints, dataPoint(float64(stat.WiredTiger.BlockManager.BytesWritten-pstat.WiredTiger.BlockManager.BytesWritten)/mb/seconds, t))
			ts["wt_blkmgr_written_checkpoint"].DataPoints = append(ts["wt_blkmgr_written_checkpoint"].DataPoints, dataPoint(float64(stat.WiredTiger.BlockManager.BytesWrittenCheckPoint-pstat.WiredTiger.BlockManager.BytesWrittenCheckPoint)/mb/seconds, t))
			ts["wt_modified_evicted"].DataPoints = append(ts["wt_modified_evicted"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.ModifiedPagesEvicted-pstat.WiredTiger.Cache.ModifiedPagesEvicted)/seconds, t))
			ts["wt_unmodified_evicted"].DataPoints = append(ts["wt_unmodified_evicted"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.UnmodifiedPagesEvicted-pstat.WiredTiger.Cache.UnmodifiedPagesEvicted)/seconds, t))
			ts["wt_cache_read_in"].DataPoints = append(ts["wt_cache_read_in"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.BytesReadIntoCache-pstat.WiredTiger.Cache.BytesReadIntoCache)/mb/seconds, t))
			ts["wt_cache_written_from"].DataPoints = append(ts["wt_cache_written_from"].DataPoints, dataPoint(float64(stat.WiredTiger.Cache.BytesWrittenFromCache-pstat.WiredTiger.Cache.BytesWrittenFromCache)/mb/seconds, t))

			// Transactions deltas
			ts["txn_aborted/s"].DataPoints = append(ts["txn_aborted/s"].DataPoints, dataPoint(float64(stat.Transactions.TotalAborted-pstat.Transactions.TotalAborted)/seconds, t))
			ts["txn_committed/s"].DataPoints = append(ts["txn_committed/s"].DataPoints, dataPoint(float64(stat.Transactions.TotalCommitted-pstat.Transactions.TotalCommitted)/seconds, t))
			ts["txn_started/s"].DataPoints = append(ts["txn_started/s"].DataPoints, dataPoint(float64(stat.Transactions.TotalStarted-pstat.Transactions.TotalStarted)/seconds, t))

			// Flow Control deltas
			ts["flowctl_acquiring_us"].DataPoints = append(ts["flowctl_acquiring_us"].DataPoints, dataPoint(float64(stat.FlowControl.TimeAcquiringMicros-pstat.FlowControl.TimeAcquiringMicros)/seconds, t))
			ts["flowctl_lagged_count"].DataPoints = append(ts["flowctl_lagged_count"].DataPoints, dataPoint(float64(stat.FlowControl.IsLaggedCount-pstat.FlowControl.IsLaggedCount)/seconds, t))
		}

		pstat = stat
	}

	return toValueMap(ts)
}

// Legacy functions for backward compatibility - now just wrappers
func getServerStatusTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(serverStatusChartsLegends))
	for _, legend := range serverStatusChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

func getWiredTigerTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(wiredTigerChartsLegends))
	for _, legend := range wiredTigerChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

func getQueuesTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(queuesChartsLegends))
	for _, legend := range queuesChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

func getTransactionsTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(transactionsChartsLegends))
	for _, legend := range transactionsChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

func getTcmallocTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(tcmallocChartsLegends))
	for _, legend := range tcmallocChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

func getFlowControlTimeSeriesDoc(serverStatusList []ServerStatusDoc) map[string]TimeSeriesDoc {
	all := getAllServerStatusTimeSeriesDoc(serverStatusList)
	result := make(map[string]TimeSeriesDoc, len(flowControlChartsLegends))
	for _, legend := range flowControlChartsLegends {
		if v, ok := all[legend]; ok {
			result[legend] = v
		}
	}
	return result
}

// Deprecated: use dataPoint instead
func getDataPoint(v float64, t float64) []float64 {
	return dataPoint(v, t)
}
