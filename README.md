# MongoDB FTDC Metrics and Charts
A dockerized tool to view MongoDB FTDC metrics.

## Startup
Create a `diagnostic.data` directory if it doesn't exist yet:

```
mkdir -p ./diagnostic.data/
```

Copy FTDC files to under directory `diagnostic.data`:

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

## Shutdown

```
docker-compose down
```
