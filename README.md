# MongoDB FTDC Metrics and Charts

A dockerized tool to view MongoDB FTDC (Full-Time Diagnostic Data Capture) metrics with Grafana dashboards.

## Requirements

- Docker and Docker Compose
- Go 1.23+ (only if building from source)

## Quick Start

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

### 3. Start the Services

```bash
docker-compose up -d
```

### 4. View Metrics in Grafana

1. Open http://localhost:3030 in your browser
2. Login with `admin` / `admin`
3. Navigate to **Dashboards** → **MongoDB FTDC Analytics**
4. Adjust the time range (top-right) to match your FTDC data timestamps

## Loading Different FTDC Data

### Option 1: Hot Reload (No Restart)

Replace files and trigger a reload:

```bash
rm ./diagnostic.data/*
cp /path/to/new/metrics.* ./diagnostic.data/
curl -XPOST http://localhost:5408/grafana/dir -d '{"dir": "/diagnostic.data"}'
```

Returns JSON with the new time range:
```json
{"endpoint":"/d/simagix-grafana/mongodb-mongo-ftdc?orgId=1&from=1550767345000&to=1550804249000","ok":1}
```

### Option 2: Restart Containers

```bash
docker-compose down
# Replace files in diagnostic.data/
docker-compose up -d
```

## Shutdown

```bash
docker-compose down
```

## Building from Source

To build the binary locally (without Docker):

```bash
./build.sh
./dist/ftdc_json -version
./dist/ftdc_json /path/to/diagnostic.data/
```

## Ports

| Service | Port | Description |
|---------|------|-------------|
| Grafana | 3030 | Web UI |
| FTDC API | 5408 | Data backend |

## Disclaimer

This software is not supported by MongoDB, Inc. under any of their commercial support subscriptions or otherwise. Any usage is at your own risk. Bug reports, feature requests and questions can be posted in the Issues section on GitHub.
