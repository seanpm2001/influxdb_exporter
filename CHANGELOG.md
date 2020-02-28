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
