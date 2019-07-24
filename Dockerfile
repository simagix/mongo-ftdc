FROM grafana/grafana
MAINTAINER Ken Chen <ken.chen@simagix.com>
ADD conf/analytics_mini.json /var/lib/grafana/dashboards/
ADD conf/ftdc-dashboards.yaml /etc/grafana/provisioning/dashboards/ftdc.yaml
ADD conf/ftdc-datasources.yaml /etc/grafana/provisioning/datasources/ftdc.yaml
