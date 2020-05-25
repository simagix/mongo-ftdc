// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"strings"
	"time"

	"github.com/simagix/mongo-ftdc/ftdc"
)

func getServerStatusDataPoints(attribsMap map[string][]int64, i uint32) ServerStatusDoc {
	ss := ServerStatusDoc{}
	ss.LocalTime = time.Unix(0, int64(time.Millisecond)*attribsMap["serverStatus/localTime"][i])
	ss.Mem.Resident = attribsMap["serverStatus/mem/resident"][i]
	ss.Mem.Virtual = attribsMap["serverStatus/mem/virtual"][i]
	ss.Network.BytesIn = attribsMap["serverStatus/network/bytesIn"][i]
	ss.Network.BytesOut = attribsMap["serverStatus/network/bytesOut"][i]
	ss.Network.NumRequests = attribsMap["serverStatus/network/numRequests"][i]
	ss.Network.PhysicalBytesIn = attribsMap["serverStatus/network/physicalBytesIn"][i]
	ss.Network.PhysicalBytesOut = attribsMap["serverStatus/network/physicalBytesOut"][i]
	ss.Connections.Current = attribsMap["serverStatus/connections/current"][i]
	ss.Connections.TotalCreated = attribsMap["serverStatus/connections/totalCreated"][i]
	ss.Connections.Available = attribsMap["serverStatus/connections/available"][i]
	if len(attribsMap["serverStatus/connections/active"]) > 0 { // after 4.0
		ss.Connections.Active = attribsMap["serverStatus/connections/active"][i]
	}
	ss.ExtraInfo.PageFaults = attribsMap["serverStatus/extra_info/page_faults"][i]
	ss.GlobalLock.ActiveClients.Readers = attribsMap["serverStatus/globalLock/activeClients/readers"][i]
	ss.GlobalLock.ActiveClients.Writers = attribsMap["serverStatus/globalLock/activeClients/writers"][i]
	ss.GlobalLock.CurrentQueue.Readers = attribsMap["serverStatus/globalLock/currentQueue/readers"][i]
	ss.GlobalLock.CurrentQueue.Writers = attribsMap["serverStatus/globalLock/currentQueue/writers"][i]
	ss.Metrics.QueryExecutor.Scanned = attribsMap["serverStatus/metrics/queryExecutor/scanned"][i]
	ss.Metrics.QueryExecutor.ScannedObjects = attribsMap["serverStatus/metrics/queryExecutor/scannedObjects"][i]
	ss.Metrics.Operation.ScanAndOrder = attribsMap["serverStatus/metrics/operation/scanAndOrder"][i]
	if len(attribsMap["serverStatus/opLatencies/commands/latency"]) > 0 { // 3.2 didn't have opLatencies
		ss.OpLatencies.Commands.Latency = attribsMap["serverStatus/opLatencies/commands/latency"][i]
		ss.OpLatencies.Commands.Ops = attribsMap["serverStatus/opLatencies/commands/ops"][i]
		ss.OpLatencies.Reads.Latency = attribsMap["serverStatus/opLatencies/reads/latency"][i]
		ss.OpLatencies.Reads.Ops = attribsMap["serverStatus/opLatencies/reads/ops"][i]
		ss.OpLatencies.Writes.Latency = attribsMap["serverStatus/opLatencies/writes/latency"][i]
		ss.OpLatencies.Writes.Ops = attribsMap["serverStatus/opLatencies/writes/ops"][i]
	}
	ss.OpCounters.Command = attribsMap["serverStatus/opcounters/command"][i]
	ss.OpCounters.Delete = attribsMap["serverStatus/opcounters/delete"][i]
	ss.OpCounters.Getmore = attribsMap["serverStatus/opcounters/getmore"][i]
	ss.OpCounters.Insert = attribsMap["serverStatus/opcounters/insert"][i]
	ss.OpCounters.Query = attribsMap["serverStatus/opcounters/query"][i]
	ss.OpCounters.Update = attribsMap["serverStatus/opcounters/update"][i]
	ss.Uptime = attribsMap["serverStatus/uptime"][i]

	ss.WiredTiger.BlockManager.BytesRead = attribsMap["serverStatus/wiredTiger/block-manager/bytes read"][i]
	ss.WiredTiger.BlockManager.BytesWritten = attribsMap["serverStatus/wiredTiger/block-manager/bytes written"][i]
	ss.WiredTiger.BlockManager.BytesWrittenCheckPoint = attribsMap["serverStatus/wiredTiger/block-manager/bytes written for checkpoint"][i]
	ss.WiredTiger.Cache.CurrentlyInCache = attribsMap["serverStatus/wiredTiger/cache/bytes currently in the cache"][i]
	ss.WiredTiger.Cache.MaxBytesConfigured = attribsMap["serverStatus/wiredTiger/cache/maximum bytes configured"][i]
	ss.WiredTiger.Cache.ModifiedPagesEvicted = attribsMap["serverStatus/wiredTiger/cache/modified pages evicted"][i]
	ss.WiredTiger.Cache.BytesReadIntoCache = attribsMap["serverStatus/wiredTiger/cache/bytes read into cache"][i]
	ss.WiredTiger.Cache.BytesWrittenFromCache = attribsMap["serverStatus/wiredTiger/cache/bytes written from cache"][i]
	ss.WiredTiger.Cache.TrackedDirtyBytes = attribsMap["serverStatus/wiredTiger/cache/tracked dirty bytes in the cache"][i]
	ss.WiredTiger.Cache.UnmodifiedPagesEvicted = attribsMap["serverStatus/wiredTiger/cache/unmodified pages evicted"][i]
	ss.WiredTiger.ConcurrentTransactions.Read.Available = attribsMap["serverStatus/wiredTiger/concurrentTransactions/read/available"][i]
	ss.WiredTiger.ConcurrentTransactions.Write.Available = attribsMap["serverStatus/wiredTiger/concurrentTransactions/write/available"][i]
	return ss
}

func getSystemMetricsDataPoints(attribsMap map[string][]int64, i uint32) SystemMetricsDoc {
	sm := SystemMetricsDoc{Disks: map[string]DiskMetrics{}}
	sm.Start = time.Unix(0, int64(time.Millisecond)*attribsMap["serverStatus/localTime"][i])
	if attribsMap["systemMetrics/cpu/idle_ms"] != nil { // system metrics only available from Linux
		sm.CPU.IdleMS = attribsMap["systemMetrics/cpu/idle_ms"][i]
		sm.CPU.UserMS = attribsMap["systemMetrics/cpu/user_ms"][i]
		sm.CPU.IOWaitMS = attribsMap["systemMetrics/cpu/iowait_ms"][i]
		sm.CPU.NiceMS = attribsMap["systemMetrics/cpu/nice_ms"][i]
		sm.CPU.SoftirqMS = attribsMap["systemMetrics/cpu/softirq_ms"][i]
		sm.CPU.StealMS = attribsMap["systemMetrics/cpu/steal_ms"][i]
		sm.CPU.SystemMS = attribsMap["systemMetrics/cpu/system_ms"][i]
	}
	for key := range attribsMap {
		if strings.Index(key, "systemMetrics/disks/") != 0 {
			continue
		}
		tokens := strings.Split(key, ftdc.PathSeparator)
		disk := tokens[2]
		stats := tokens[3]
		if _, ok := sm.Disks[disk]; !ok {
			sm.Disks[disk] = DiskMetrics{}
		}
		m := sm.Disks[disk]
		switch stats {
		case "read_time_ms":
			m.ReadTimeMS = attribsMap[key][i]
		case "write_time_ms":
			m.WriteTimeMS = attribsMap[key][i]
		case "io_queued_ms":
			m.IOQueuedMS = attribsMap[key][i]
		case "io_time_ms":
			m.IOTimeMS = attribsMap[key][i]
		case "reads":
			m.Reads = attribsMap[key][i]
		case "writes":
			m.Writes = attribsMap[key][i]
		case "io_in_progress":
			m.IOInProgress = attribsMap[key][i]
		}
		sm.Disks[disk] = m
	}
	return sm
}
