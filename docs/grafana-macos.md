# Grafana Installation on macOS

This guide covers installing Grafana natively on macOS for use with mongo-ftdc.

## Prerequisites

- macOS with Homebrew installed
- mongo-ftdc built (`./build.sh`)

## Installation Steps

### 1. Install Grafana

```bash
brew install grafana
```

### 2. Install JSON Datasource Plugin

```bash
grafana cli --homepath /opt/homebrew/opt/grafana/share/grafana \
  --pluginsDir /opt/homebrew/var/lib/grafana/plugins \
  plugins install simpod-json-datasource
```

### 3. Create Required Directories

```bash
mkdir -p /opt/homebrew/var/lib/grafana/plugins
mkdir -p /opt/homebrew/var/lib/grafana/dashboards
mkdir -p /opt/homebrew/etc/grafana/provisioning/datasources
mkdir -p /opt/homebrew/etc/grafana/provisioning/dashboards
```

### 4. Configure Datasource

Create `/opt/homebrew/etc/grafana/provisioning/datasources/ftdc.yaml`:

```yaml
apiVersion: 1

datasources:
- name: ftdc
  type: simpod-json-datasource
  access: proxy
  orgId: 1
  url: http://localhost:5408/grafana
  isDefault: true
  version: 1
  editable: true
```

### 5. Configure Dashboard Provisioning

Create `/opt/homebrew/etc/grafana/provisioning/dashboards/ftdc.yaml`:

```yaml
apiVersion: 1

providers:
- name: 'ftdc'
  orgId: 1
  folder: ''
  type: file
  options:
    path: /opt/homebrew/var/lib/grafana/dashboards
```

### 6. Copy Dashboard

```bash
cp grafana/dashboards/analytics.json /opt/homebrew/var/lib/grafana/dashboards/
```

### 7. Update Grafana Configuration

Append the following to `/opt/homebrew/etc/grafana/grafana.ini`:

```ini
# Custom settings for FTDC

[paths]
data = /opt/homebrew/var/lib/grafana
plugins = /opt/homebrew/var/lib/grafana/plugins
provisioning = /opt/homebrew/etc/grafana/provisioning

[auth.anonymous]
enabled = true
org_role = Admin
```

## Running

### Start Grafana

```bash
brew services start grafana
```

### Start FTDC Server

In a separate terminal:

```bash
./dist/mftdc -server /path/to/diagnostic.data/
```

The URL printed will automatically show:
- Port **3000** when running natively
- Port **3030** when running in Docker (detected by hostname)

### Access Dashboard

Open: http://localhost:3000

The dashboard will be available at:
```
http://localhost:3000/d/simagix-grafana/mongodb-mongo-ftdc
```

## Common Commands

| Action | Command |
|--------|---------|
| Start Grafana | `brew services start grafana` |
| Stop Grafana | `brew services stop grafana` |
| Restart Grafana | `brew services restart grafana` |
| Check status | `brew services info grafana` |
| View logs | `tail -f /opt/homebrew/var/log/grafana/grafana.log` |

## Update Dashboard

After modifying `analytics.json`, copy it to the dashboards directory:

```bash
cp grafana/dashboards/analytics.json /opt/homebrew/var/lib/grafana/dashboards/
```

Grafana will automatically pick up changes (may take a few seconds).

## File Locations

| Component | Path |
|-----------|------|
| Grafana binary | `/opt/homebrew/bin/grafana` |
| Config file | `/opt/homebrew/etc/grafana/grafana.ini` |
| Plugins | `/opt/homebrew/var/lib/grafana/plugins/` |
| Dashboards | `/opt/homebrew/var/lib/grafana/dashboards/` |
| Provisioning | `/opt/homebrew/etc/grafana/provisioning/` |
| Logs | `/opt/homebrew/var/log/grafana/` |

## Troubleshooting

### Plugin not loading

Ensure the plugin path is correctly set in `grafana.ini`:
```ini
[paths]
plugins = /opt/homebrew/var/lib/grafana/plugins
```

Then restart Grafana:
```bash
brew services restart grafana
```

### Datasource connection error

1. Verify FTDC server is running on port 5408
2. Check datasource URL in Grafana UI: Settings → Data Sources → ftdc
3. Test connection in Grafana UI

### Dashboard not appearing

1. Check provisioning config exists at `/opt/homebrew/etc/grafana/provisioning/dashboards/ftdc.yaml`
2. Verify dashboard JSON is in `/opt/homebrew/var/lib/grafana/dashboards/`
3. Check Grafana logs for errors

## Notes

- Paths shown are for Apple Silicon Macs (`/opt/homebrew/`)
- Intel Macs use `/usr/local/` instead
- Check your prefix with: `brew --prefix grafana`

