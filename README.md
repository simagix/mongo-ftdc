# MongoDB FTDC Metrics and Charts

A dockerized tool to view MongoDB FTDC (Full-Time Diagnostic Data Capture) metrics with Grafana dashboards.

This is the only publicly available tool to visually analyze FTDC data, similar to MongoDB's internal "t2" tool used by support engineers.

## Quick Start with Docker

### 1. Build Docker Images

```bash
./build.sh docker
```

This builds two images:
- `simagix/ftdc` - FTDC data parser and API server
- `simagix/grafana-ftdc` - Grafana with pre-configured dashboards

### 2. Prepare FTDC Data

Create the data directory and copy your FTDC metrics files:

```bash
mkdir -p ./diagnostic.data/
cp /path/to/your/metrics.* ./diagnostic.data/
```

FTDC files are typically located at:
- Linux: `/var/lib/mongo/diagnostic.data/`
- macOS: `/usr/local/var/mongodb/diagnostic.data/`
- Atlas: Download from Atlas UI → Clusters → ... → Download Diagnostics

### 3. Start the Services

```bash
docker-compose up
```

### 4. View Metrics in Grafana

Click the URL printed in the terminal output:
```
Grafana: http://localhost:3030/d/simagix-grafana/mongodb-mongo-ftdc?from=...&to=...&kiosk=1
```

No login required - anonymous access is enabled.

## Features

- **Assessment Panel** - Automatic health scoring with color-coded metrics
- **Query Metrics** - Query Targeting ratio, Document metrics, Write Conflicts
- **WiredTiger** - Cache usage, evictions, block manager stats
- **Server Status** - Connections, latency, ops counters, memory
- **System Metrics** - CPU usage, disk IOPS and utilization
- **MongoDB 7.0+** - Transactions, Admission Control, Flow Control

## Loading Different FTDC Data

### Option 1: Hot Reload (No Restart)

Replace files and trigger a reload:

```bash
rm ./diagnostic.data/*
cp /path/to/new/metrics.* ./diagnostic.data/
curl -XPOST http://localhost:5408/grafana/dir -d '{"dir": "/diagnostic.data"}'
```

### Option 2: Restart Containers

```bash
docker-compose down
# Replace files in diagnostic.data/
docker-compose up
```

## Shutdown

```bash
docker-compose down
```

## Building from Source

Requirements: Go 1.23+

```bash
./build.sh
./dist/ftdc_json /path/to/diagnostic.data/
```

## Ports

| Service | Port | Description |
|---------|------|-------------|
| Grafana | 3030 | Web UI |
| FTDC API | 5408 | Data backend |

## Support This Project

If you find this tool useful, consider [sponsoring](https://github.com/sponsors/simagix) to support continued development.

## Disclaimer

This software is not supported by MongoDB, Inc. under any of their commercial support subscriptions or otherwise. Any usage is at your own risk. Bug reports, feature requests and questions can be posted in the Issues section on GitHub.
