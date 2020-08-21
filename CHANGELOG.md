## 0.5.0 / 2020-08-21

* [CHANGE] Move exporter metrics to their own endpoint ([#68](https://github.com/prometheus/influxdb_exporter/pull/68))
* [ENHANCEMENT] Ignore the `__name__` label on incoming metrics ([#69](https://github.com/prometheus/influxdb_exporter/pull/69))

This release improves the experience in mixed Prometheus/InfluxDB environments.
By moving the exporter's own metrics to a separate endpoint, we avoid conflicts with metrics from other services using the Prometheus client library.
In these circumstances, a spurious `__name__` label might appear, which we cannot ingest.
The exporter now ignores it.

## 0.4.2 / 2020-06-12

* [CHANGE] Update all dependencies, including Prometheus client ([#66](https://github.com/prometheus/influxdb_exporter/pull/66))

## 0.4.1 / 2020-05-04

* [ENHANCEMENT] Improve performance by reducing allocations ([#64](https://github.com/prometheus/influxdb_exporter/pull/64))

## 0.4.0 / 2020-02-28

* [FEATURE] Add ping endpoint that some clients expect ([#60](https://github.com/prometheus/influxdb_exporter/pull/60))

## 0.3.0 / 2019-10-04

* [CHANGE] Do not run as root in the Docker container by default ([#40](https://github.com/prometheus/influxdb_exporter/pull/40))
* [CHANGE] Update logging library & flags ([#58](https://github.com/prometheus/influxdb_exporter/pull/58))

## 0.2.0 / 2019-02-28

* [CHANGE] Switch to Kingpin flag library ([#14](https://github.com/prometheus/influxdb_exporter/pull/14))
* [FEATURE] Optionally export samples with timestamp ([#36](https://github.com/prometheus/influxdb_exporter/pull/36))

For consistency with other Prometheus projects, the exporter now expects
POSIX-ish flag semantics. Use single dashes for short options (`-h`) and two
dashes for long options (`--help`).

## 0.1.0 / 2017-07-26

Initial release.
