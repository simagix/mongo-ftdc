// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// attribs.go

package ftdc

import (
	"math"
	"strings"
	"time"

	"github.com/simagix/mongo-ftdc/decoder"
)

const diskMetricsPrefix = "systemMetrics/disks/"

// Attribs stores attribs map
type Attribs struct {
	attribsMap *map[string][]uint64
	diskKeys   []diskKeyInfo // cached disk keys for efficient iteration
}

// diskKeyInfo stores pre-parsed disk key information
type diskKeyInfo struct {
	fullKey  string
	diskName string
	statName string
}

// NewAttribs returns Attribs structure
func NewAttribs(attribsMap *map[string][]uint64) *Attribs {
	attr := &Attribs{attribsMap: attribsMap}
	// Pre-filter and parse disk keys once
	attr.diskKeys = make([]diskKeyInfo, 0)
	for key := range *attribsMap {
		if strings.HasPrefix(key, diskMetricsPrefix) {
			// Parse: "systemMetrics/disks/sda/read_time_ms"
			suffix := key[len(diskMetricsPrefix):] // "sda/read_time_ms"
			slashIdx := strings.Index(suffix, decoder.PathSeparator)
			if slashIdx > 0 {
				attr.diskKeys = append(attr.diskKeys, diskKeyInfo{
					fullKey:  key,
					diskName: suffix[:slashIdx],
					statName: suffix[slashIdx+1:],
				})
			}
		}
	}
	return attr
}

// GetServerStatusDataPoints returns server status
func (attr *Attribs) GetServerStatusDataPoints(i int) ServerStatusDoc {
	ss := ServerStatusDoc{}
	ss.LocalTime = time.Unix(0, int64(time.Millisecond)*int64(attr.get("serverStatus/localTime", i)))
	ss.Mem.Resident = attr.get("serverStatus/mem/resident", i)
	ss.Mem.Virtual = attr.get("serverStatus/mem/virtual", i)
	ss.Network.BytesIn = attr.get("serverStatus/network/bytesIn", i)
	ss.Network.BytesOut = attr.get("serverStatus/network/bytesOut", i)
	ss.Network.NumRequests = attr.get("serverStatus/network/numRequests", i)
	ss.Network.PhysicalBytesIn = attr.get("serverStatus/network/physicalBytesIn", i)
	ss.Network.PhysicalBytesOut = attr.get("serverStatus/network/physicalBytesOut", i)
	ss.Connections.Current = attr.get("serverStatus/connections/current", i)
	ss.Connections.TotalCreated = attr.get("serverStatus/connections/totalCreated", i)
	ss.Connections.Available = attr.get("serverStatus/connections/available", i)
	ss.Connections.Active = attr.get("serverStatus/connections/active", i)
	ss.ExtraInfo.PageFaults = attr.get("serverStatus/extra_info/page_faults", i)
	ss.GlobalLock.ActiveClients.Readers = attr.get("serverStatus/globalLock/activeClients/readers", i)
	ss.GlobalLock.ActiveClients.Writers = attr.get("serverStatus/globalLock/activeClients/writers", i)
	ss.GlobalLock.CurrentQueue.Readers = attr.get("serverStatus/globalLock/currentQueue/readers", i)
	ss.GlobalLock.CurrentQueue.Writers = attr.get("serverStatus/globalLock/currentQueue/writers", i)
	ss.Metrics.QueryExecutor.Scanned = attr.get("serverStatus/metrics/queryExecutor/scanned", i)
	ss.Metrics.QueryExecutor.ScannedObjects = attr.get("serverStatus/metrics/queryExecutor/scannedObjects", i)
	ss.Metrics.Operation.ScanAndOrder = attr.get("serverStatus/metrics/operation/scanAndOrder", i)
	ss.OpLatencies.Commands.Latency = attr.get("serverStatus/opLatencies/commands/latency", i)
	ss.OpLatencies.Commands.Ops = attr.get("serverStatus/opLatencies/commands/ops", i)
	ss.OpLatencies.Reads.Latency = attr.get("serverStatus/opLatencies/reads/latency", i)
	ss.OpLatencies.Reads.Ops = attr.get("serverStatus/opLatencies/reads/ops", i)
	ss.OpLatencies.Writes.Latency = attr.get("serverStatus/opLatencies/writes/latency", i)
	ss.OpLatencies.Writes.Ops = attr.get("serverStatus/opLatencies/writes/ops", i)
	ss.OpCounters.Command = attr.get("serverStatus/opcounters/command", i)
	ss.OpCounters.Delete = attr.get("serverStatus/opcounters/delete", i)
	ss.OpCounters.Getmore = attr.get("serverStatus/opcounters/getmore", i)
	ss.OpCounters.Insert = attr.get("serverStatus/opcounters/insert", i)
	ss.OpCounters.Query = attr.get("serverStatus/opcounters/query", i)
	ss.OpCounters.Update = attr.get("serverStatus/opcounters/update", i)
	ss.Uptime = attr.get("serverStatus/uptime", i)

	ss.WiredTiger.BlockManager.BytesRead = attr.get("serverStatus/wiredTiger/block-manager/bytes read", i)
	ss.WiredTiger.BlockManager.BytesWritten = attr.get("serverStatus/wiredTiger/block-manager/bytes written", i)
	ss.WiredTiger.BlockManager.BytesWrittenCheckPoint = attr.get("serverStatus/wiredTiger/block-manager/bytes written for checkpoint", i)
	ss.WiredTiger.Cache.CurrentlyInCache = attr.get("serverStatus/wiredTiger/cache/bytes currently in the cache", i)
	ss.WiredTiger.Cache.MaxBytesConfigured = attr.get("serverStatus/wiredTiger/cache/maximum bytes configured", i)
	ss.WiredTiger.Cache.ModifiedPagesEvicted = attr.get("serverStatus/wiredTiger/cache/modified pages evicted", i)
	ss.WiredTiger.Cache.BytesReadIntoCache = attr.get("serverStatus/wiredTiger/cache/bytes read into cache", i)
	ss.WiredTiger.Cache.BytesWrittenFromCache = attr.get("serverStatus/wiredTiger/cache/bytes written from cache", i)
	ss.WiredTiger.Cache.TrackedDirtyBytes = attr.get("serverStatus/wiredTiger/cache/tracked dirty bytes in the cache", i)
	ss.WiredTiger.Cache.UnmodifiedPagesEvicted = attr.get("serverStatus/wiredTiger/cache/unmodified pages evicted", i)
	ss.WiredTiger.DataHandle.Active = attr.get("serverStatus/wiredTiger/data-handle/connection data handles currently active", i)
	ss.WiredTiger.ConcurrentTransactions.Read.Available = attr.get("serverStatus/wiredTiger/concurrentTransactions/read/available", i)
	ss.WiredTiger.ConcurrentTransactions.Write.Available = attr.get("serverStatus/wiredTiger/concurrentTransactions/write/available", i)

	// MongoDB 7.0+ Queues (Admission Control)
	ss.Queues.Execution.Read.Out = attr.get("serverStatus/queues/execution/read/out", i)
	ss.Queues.Execution.Read.Available = attr.get("serverStatus/queues/execution/read/available", i)
	ss.Queues.Execution.Read.TotalTickets = attr.get("serverStatus/queues/execution/read/totalTickets", i)
	ss.Queues.Execution.Write.Out = attr.get("serverStatus/queues/execution/write/out", i)
	ss.Queues.Execution.Write.Available = attr.get("serverStatus/queues/execution/write/available", i)
	ss.Queues.Execution.Write.TotalTickets = attr.get("serverStatus/queues/execution/write/totalTickets", i)

	// Transactions
	ss.Transactions.CurrentActive = attr.get("serverStatus/transactions/currentActive", i)
	ss.Transactions.CurrentInactive = attr.get("serverStatus/transactions/currentInactive", i)
	ss.Transactions.CurrentOpen = attr.get("serverStatus/transactions/currentOpen", i)
	ss.Transactions.TotalAborted = attr.get("serverStatus/transactions/totalAborted", i)
	ss.Transactions.TotalCommitted = attr.get("serverStatus/transactions/totalCommitted", i)
	ss.Transactions.TotalStarted = attr.get("serverStatus/transactions/totalStarted", i)

	// tcmalloc Memory
	ss.Tcmalloc.Generic.BytesInUseByApp = attr.get("serverStatus/tcmalloc/generic/bytes_in_use_by_app", i)
	ss.Tcmalloc.Generic.CurrentAllocatedBytes = attr.get("serverStatus/tcmalloc/generic/current_allocated_bytes", i)
	ss.Tcmalloc.Generic.HeapSize = attr.get("serverStatus/tcmalloc/generic/heap_size", i)
	ss.Tcmalloc.Generic.PhysicalMemoryUsed = attr.get("serverStatus/tcmalloc/generic/physical_memory_used", i)

	// Flow Control (Replication)
	ss.FlowControl.TargetRateLimit = attr.get("serverStatus/flowControl/targetRateLimit", i)
	ss.FlowControl.TimeAcquiringMicros = attr.get("serverStatus/flowControl/timeAcquiringMicros", i)
	ss.FlowControl.IsLaggedCount = attr.get("serverStatus/flowControl/isLaggedCount", i)

	return ss
}

// GetSystemMetricsDataPoints returns system metrics
func (attr *Attribs) GetSystemMetricsDataPoints(i int) SystemMetricsDoc {
	sm := SystemMetricsDoc{Disks: make(map[string]DiskMetrics)}
	sm.Start = time.Unix(0, int64(time.Millisecond)*int64(attr.get("serverStatus/localTime", i)))
	sm.CPU.IdleMS = attr.get("systemMetrics/cpu/idle_ms", i)
	sm.CPU.UserMS = attr.get("systemMetrics/cpu/user_ms", i)
	sm.CPU.IOWaitMS = attr.get("systemMetrics/cpu/iowait_ms", i)
	sm.CPU.NiceMS = attr.get("systemMetrics/cpu/nice_ms", i)
	sm.CPU.SoftirqMS = attr.get("systemMetrics/cpu/softirq_ms", i)
	sm.CPU.StealMS = attr.get("systemMetrics/cpu/steal_ms", i)
	sm.CPU.SystemMS = attr.get("systemMetrics/cpu/system_ms", i)

	// Use pre-cached disk keys instead of iterating entire map
	for _, dk := range attr.diskKeys {
		m := sm.Disks[dk.diskName] // zero value if not exists
		switch dk.statName {
		case "read_time_ms":
			m.ReadTimeMS = attr.get(dk.fullKey, i)
		case "write_time_ms":
			m.WriteTimeMS = attr.get(dk.fullKey, i)
		case "io_queued_ms":
			m.IOQueuedMS = attr.get(dk.fullKey, i)
		case "io_time_ms":
			m.IOTimeMS = attr.get(dk.fullKey, i)
		case "reads":
			m.Reads = attr.get(dk.fullKey, i)
		case "writes":
			m.Writes = attr.get(dk.fullKey, i)
		case "io_in_progress":
			m.IOInProgress = attr.get(dk.fullKey, i)
		default:
			continue // skip unknown stats, don't update map
		}
		sm.Disks[dk.diskName] = m
	}
	return sm
}

func (attr *Attribs) get(key string, i int) uint64 {
	arr := (*attr.attribsMap)[key]
	if i < len(arr) && !math.IsNaN(float64(arr[i])) {
		return arr[i]
	}
	return 0
}
