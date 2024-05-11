# MongoDB FTDC Metrics and Charts

Introducing new dockerized tool designed to visualize MongoDB FTDC (Field Type Data Capture) metrics. This dashboard provides comprehensive insights into the performance and health of your MongoDB deployment.

## New Features

- FTDC Metrics: Gain visibility into various MongoDB metrics, including WiredTiger and storage engine statistics.
- Customizable Charts: Customize charts to monitor specific metrics based on your requirements.
- New Metrics: We've added Ops Counters Repl, Write Conflicts, Collection Scan, Cursors, TTL Delete, Flow Control and Document Operations metrics to provide deeper insights into your MongoDB deployment.
- Enhanced Graphs: Replication lag and disk IOPS graphs have been enhanced for improved monitoring and analysis.

## Build

Use `build.sh` to build *simagix/ftdc* and *simagix/grafana-ftdc* Docker images.

```bash
./build.sh docker
```

## Startup

Create a `diagnostic.data` directory if it doesn't exist yet:

```bash
mkdir -p ./diagnostic.data/
```

Copy FTDC files to under directory *diagnostic.data*:

```bash
cp $SOMEWHERE/metrics.* ./diagnostic.data/
```

Bring up FTDC viewer:

```bash
docker-compose up -d
```

## View FTDC Metrics

- View results URL `http://localhost:3030/` using a browser.
- Choose **MongoDB FTDC Analytics** from dashboard.
- Change correct *From* and *To* date/time in the *Custom Range* panel.

## Read Other FTDC Data

To read different FTDC files without restarting all Docker containers, remove all files from directory *diagnostic.data* and copy FTDC files to under the same directory.  Execute the command below to force FTDC data reload:

```bash
curl -XPOST http://localhost:5408/grafana/dir -d '{"dir": "/diagnostic.data"}'
```

A JSON document is returned with information of the new endpoint, begin and end timestamps:

> {"endpoint":"/d/simagix-grafana/mongodb-mongo-ftdc?orgId=1\u0026from=1550767345000\u0026to=1550804249000","ok":1}

Alternatively, you can simply run the command `docker-compose down` to bring down the entire cluster, edit the *docker-compose.yaml* file to point the *diagnostic.data* directory to a new location, and run `docker-compose up` to bring up the cluster again.

## Shutdown

```bash
docker-compose down
```

## Disclaimer

This software is not supported by MongoDB, Inc. under any of their commercial support subscriptions or otherwise. Any usage is at your own risk. Bug reports, feature requests and questions can be posted in the Issues section on GitHub.
