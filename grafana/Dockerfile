FROM grafana/grafana
LABEL Ken Chen <ken.chen@simagix.com>
ADD grafana/dashboards/analytics.json /var/lib/grafana/dashboards/
ADD grafana/dashboards.yaml /etc/grafana/provisioning/dashboards/ftdc.yaml
ADD grafana/datasources.yaml /etc/grafana/provisioning/datasources/ftdc.yaml
