# MongoDB FTDC Metrics and Charts
A dockerized tool to view MongoDB FTDC metrics.

## Build
Use `build.sh` to build *simagix/ftdc* and *simagix/grafana-ftdc* Docker images.

```
./build.sh
```

## Startup
Create a `diagnostic.data` directory if it doesn't exist yet:

```
mkdir -p ./diagnostic.data/
```

Copy FTDC files to under directory *diagnostic.data*:

```
cp $SOMEWHERE/metrics.* ./diagnostic.data/
```

Bring up FTDC viewer:

```
$ docker-compose up
```

## View FTDC Metrics

- View results URL `http://localhost:3030/` using a browser.
- Choose **MongoDB FTDC Analytics** from dashboard.
- Change correct *From* and *To* date/time in the *Custom Range* panel.

## Read Other FTDC Data
To read different FTDC files without restarting all Docker containers, remove all files from directory *diagnostic.data* and copy FTDC files to under the same directory.  Execute the command below to force FTDC data reload:

```
curl -XPOST http://localhost:5438/grafana/dir -d '{"dir": "/diagnostic.data"}'
```

A JSON document is returned with information of the new endpoint, begin and end timestamps:

> {"endpoint":"/d/simagix-grafana/mongodb-mongo-ftdc?orgId=1\u0026from=1550767345000\u0026to=1550804249000","ok":1}

Alternatively, you can simply run the command `docker-compose down` to bring down the entire cluster, edit the *docker-compose.yaml* file to point the *diagnostic.data* directory to a new location, and run `docker-compose up` to bring up the cluster again.

## Shutdown

```
docker-compose down
```
