// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"math"
	"strings"
	"time"

	"github.com/simagix/mongo-ftdc/ftdc"
)

func getValueAt(arr []int64, i uint32) int64 {
	if int(i) < len(arr) && math.IsNaN(float64(arr[i])) == false {
		return arr[i]
	}
	return -1
}

func getServerStatusDataPoints(attribsMap map[string][]int64, i uint32) ServerStatusDoc {
	ss := ServerStatusDoc{}
	ss.LocalTime = time.Unix(0, int64(time.Millisecond)*getValueAt(attribsMap["serverStatus/localTime"], i))
	ss.Mem.Resident = getValueAt(attribsMap["serverStatus/mem/resident"], i)
	ss.Mem.Virtual = getValueAt(attribsMap["serverStatus/mem/virtual"], i)
	ss.Network.BytesIn = getValueAt(attribsMap["serverStatus/network/bytesIn"], i)
	ss.Network.BytesOut = getValueAt(attribsMap["serverStatus/network/bytesOut"], i)
	ss.Network.NumRequests = getValueAt(attribsMap["serverStatus/network/numRequests"], i)
	ss.Network.PhysicalBytesIn = getValueAt(attribsMap["serverStatus/network/physicalBytesIn"], i)
	ss.Network.PhysicalBytesOut = getValueAt(attribsMap["serverStatus/network/physicalBytesOut"], i)
	ss.Connections.Current = getValueAt(attribsMap["serverStatus/connections/current"], i)
	ss.Connections.TotalCreated = getValueAt(attribsMap["serverStatus/connections/totalCreated"], i)
	ss.Connections.Available = getValueAt(attribsMap["serverStatus/connections/available"], i)
	ss.Connections.Active = getValueAt(attribsMap["serverStatus/connections/active"], i)
	ss.ExtraInfo.PageFaults = getValueAt(attribsMap["serverStatus/extra_info/page_faults"], i)
	ss.GlobalLock.ActiveClients.Readers = getValueAt(attribsMap["serverStatus/globalLock/activeClients/readers"], i)
	ss.GlobalLock.ActiveClients.Writers = getValueAt(attribsMap["serverStatus/globalLock/activeClients/writers"], i)
	ss.GlobalLock.CurrentQueue.Readers = getValueAt(attribsMap["serverStatus/globalLock/currentQueue/readers"], i)
	ss.GlobalLock.CurrentQueue.Writers = getValueAt(attribsMap["serverStatus/globalLock/currentQueue/writers"], i)
	ss.Metrics.QueryExecutor.Scanned = getValueAt(attribsMap["serverStatus/metrics/queryExecutor/scanned"], i)
	ss.Metrics.QueryExecutor.ScannedObjects = getValueAt(attribsMap["serverStatus/metrics/queryExecutor/scannedObjects"], i)
	ss.Metrics.Operation.ScanAndOrder = getValueAt(attribsMap["serverStatus/metrics/operation/scanAndOrder"], i)
	ss.OpLatencies.Commands.Latency = getValueAt(attribsMap["serverStatus/opLatencies/commands/latency"], i)
	ss.OpLatencies.Commands.Ops = getValueAt(attribsMap["serverStatus/opLatencies/commands/ops"], i)
	ss.OpLatencies.Reads.Latency = getValueAt(attribsMap["serverStatus/opLatencies/reads/latency"], i)
	ss.OpLatencies.Reads.Ops = getValueAt(attribsMap["serverStatus/opLatencies/reads/ops"], i)
	ss.OpLatencies.Writes.Latency = getValueAt(attribsMap["serverStatus/opLatencies/writes/latency"], i)
	ss.OpLatencies.Writes.Ops = getValueAt(attribsMap["serverStatus/opLatencies/writes/ops"], i)
	ss.OpCounters.Command = getValueAt(attribsMap["serverStatus/opcounters/command"], i)
	ss.OpCounters.Delete = getValueAt(attribsMap["serverStatus/opcounters/delete"], i)
	ss.OpCounters.Getmore = getValueAt(attribsMap["serverStatus/opcounters/getmore"], i)
	ss.OpCounters.Insert = getValueAt(attribsMap["serverStatus/opcounters/insert"], i)
	ss.OpCounters.Query = getValueAt(attribsMap["serverStatus/opcounters/query"], i)
	ss.OpCounters.Update = getValueAt(attribsMap["serverStatus/opcounters/update"], i)
	ss.Uptime = getValueAt(attribsMap["serverStatus/uptime"], i)

	ss.WiredTiger.BlockManager.BytesRead = getValueAt(attribsMap["serverStatus/wiredTiger/block-manager/bytes read"], i)
	ss.WiredTiger.BlockManager.BytesWritten = getValueAt(attribsMap["serverStatus/wiredTiger/block-manager/bytes written"], i)
	ss.WiredTiger.BlockManager.BytesWrittenCheckPoint = getValueAt(attribsMap["serverStatus/wiredTiger/block-manager/bytes written for checkpoint"], i)
	ss.WiredTiger.Cache.CurrentlyInCache = getValueAt(attribsMap["serverStatus/wiredTiger/cache/bytes currently in the cache"], i)
	ss.WiredTiger.Cache.MaxBytesConfigured = getValueAt(attribsMap["serverStatus/wiredTiger/cache/maximum bytes configured"], i)
	ss.WiredTiger.Cache.ModifiedPagesEvicted = getValueAt(attribsMap["serverStatus/wiredTiger/cache/modified pages evicted"], i)
	ss.WiredTiger.Cache.BytesReadIntoCache = getValueAt(attribsMap["serverStatus/wiredTiger/cache/bytes read into cache"], i)
	ss.WiredTiger.Cache.BytesWrittenFromCache = getValueAt(attribsMap["serverStatus/wiredTiger/cache/bytes written from cache"], i)
	ss.WiredTiger.Cache.TrackedDirtyBytes = getValueAt(attribsMap["serverStatus/wiredTiger/cache/tracked dirty bytes in the cache"], i)
	ss.WiredTiger.Cache.UnmodifiedPagesEvicted = getValueAt(attribsMap["serverStatus/wiredTiger/cache/unmodified pages evicted"], i)
	ss.WiredTiger.DataHandle.Active = getValueAt(attribsMap["serverStatus/wiredTiger/data-handle/connection data handles currently active"], i)
	ss.WiredTiger.ConcurrentTransactions.Read.Available = getValueAt(attribsMap["serverStatus/wiredTiger/concurrentTransactions/read/available"], i)
	ss.WiredTiger.ConcurrentTransactions.Write.Available = getValueAt(attribsMap["serverStatus/wiredTiger/concurrentTransactions/write/available"], i)
	return ss
}

func getSystemMetricsDataPoints(attribsMap map[string][]int64, i uint32) SystemMetricsDoc {
	sm := SystemMetricsDoc{Disks: map[string]DiskMetrics{}}
	sm.Start = time.Unix(0, int64(time.Millisecond)*getValueAt(attribsMap["serverStatus/localTime"], i))
	sm.CPU.IdleMS = getValueAt(attribsMap["systemMetrics/cpu/idle_ms"], i)
	sm.CPU.UserMS = getValueAt(attribsMap["systemMetrics/cpu/user_ms"], i)
	sm.CPU.IOWaitMS = getValueAt(attribsMap["systemMetrics/cpu/iowait_ms"], i)
	sm.CPU.NiceMS = getValueAt(attribsMap["systemMetrics/cpu/nice_ms"], i)
	sm.CPU.SoftirqMS = getValueAt(attribsMap["systemMetrics/cpu/softirq_ms"], i)
	sm.CPU.StealMS = getValueAt(attribsMap["systemMetrics/cpu/steal_ms"], i)
	sm.CPU.SystemMS = getValueAt(attribsMap["systemMetrics/cpu/system_ms"], i)
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
			m.ReadTimeMS = getValueAt(attribsMap[key], i)
		case "write_time_ms":
			m.WriteTimeMS = getValueAt(attribsMap[key], i)
		case "io_queued_ms":
			m.IOQueuedMS = getValueAt(attribsMap[key], i)
		case "io_time_ms":
			m.IOTimeMS = getValueAt(attribsMap[key], i)
		case "reads":
			m.Reads = getValueAt(attribsMap[key], i)
		case "writes":
			m.Writes = getValueAt(attribsMap[key], i)
		case "io_in_progress":
			m.IOInProgress = getValueAt(attribsMap[key], i)
		}
		sm.Disks[disk] = m
	}
	return sm
}
