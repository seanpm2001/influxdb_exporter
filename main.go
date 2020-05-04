// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	"github.com/influxdata/influxdb/models"
	"strings"
)

const (
	MAX_UDP_PAYLOAD = 64 * 1024
)

var (
	listenAddress   = kingpin.Flag("web.listen-address", "Address on which to expose metrics and web interface.").Default(":9122").String()
	metricsPath     = kingpin.Flag("web.telemetry-path", "Path under which to expose Prometheus metrics.").Default("/metrics").String()
	sampleExpiry    = kingpin.Flag("influxdb.sample-expiry", "How long a sample is valid for.").Default("5m").Duration()
	bindAddress     = kingpin.Flag("udp.bind-address", "Address on which to listen for udp packets.").Default(":9122").String()
	exportTimestamp = kingpin.Flag("timestamps", "Export timestamps of points").Default("false").Bool()
	lastPush        = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "influxdb_last_push_timestamp_seconds",
			Help: "Unix timestamp of the last received influxdb metrics push in seconds.",
		},
	)
	udpParseErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "influxdb_udp_parse_errors_total",
			Help: "Current total udp parse errors.",
		},
	)
)

type influxDBSample struct {
	ID        string
	Name      string
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}

func (c *influxDBCollector) serveUdp() {
	buf := make([]byte, MAX_UDP_PAYLOAD)
	for {
		n, _, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			level.Warn(c.logger).Log("msg", "Failed to read UDP message", "err", err)
			continue
		}

		bufCopy := make([]byte, n)
		copy(bufCopy, buf[:n])

		precision := "ns"
		points, err := models.ParsePointsWithPrecision(bufCopy, time.Now().UTC(), precision)
		if err != nil {
			level.Error(c.logger).Log("msg", "Error parsing udp packet", "err", err)
			udpParseErrors.Inc()
			return
		}

		c.parsePointsToSample(points)
	}
}

type influxDBCollector struct {
	samples map[string]*influxDBSample
	mu      sync.Mutex
	ch      chan *influxDBSample
	logger  log.Logger

	// Udp
	conn *net.UDPConn
}

func newInfluxDBCollector(logger log.Logger) *influxDBCollector {
	c := &influxDBCollector{
		ch:      make(chan *influxDBSample),
		samples: map[string]*influxDBSample{},
		logger:  logger,
	}
	go c.processSamples()
	return c
}

func (c *influxDBCollector) influxDBPost(w http.ResponseWriter, r *http.Request) {
	lastPush.Set(float64(time.Now().UnixNano()) / 1e9)
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("error reading body: %s", err), 500)
		return
	}

	precision := "ns"
	if r.FormValue("precision") != "" {
		precision = r.FormValue("precision")
	}
	points, err := models.ParsePointsWithPrecision(buf, time.Now().UTC(), precision)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing request: %s", err), 400)
		return
	}

	c.parsePointsToSample(points)

	// InfluxDB returns a 204 on success.
	http.Error(w, "", http.StatusNoContent)
}

func (c *influxDBCollector) parsePointsToSample(points []models.Point) {
	for _, s := range points {
		fields, err := s.Fields()
		if err != nil {
			level.Error(c.logger).Log("msg", "error getting fields from point", "err", err)
			continue
		}
		for field, v := range fields {
			var value float64
			switch v := v.(type) {
			case float64:
				value = v
			case int64:
				value = float64(v)
			case bool:
				if v {
					value = 1
				} else {
					value = 0
				}
			default:
				continue
			}

			var name string
			if field == "value" {
				name = string(s.Name())
			} else {
				name = string(s.Name()) + "_" + field
			}

			ReplaceInvalidChars(&name)
			sample := &influxDBSample{
				Name:      name,
				Timestamp: s.Time(),
				Value:     value,
				Labels:    map[string]string{},
			}
			for _, v := range s.Tags() {

				key := string(v.Key)
				ReplaceInvalidChars(&key)
				sample.Labels[key] = string(v.Value)
			}

			// Calculate a consistent unique ID for the sample.
			labelnames := make([]string, 0, len(sample.Labels))
			for k := range sample.Labels {
				labelnames = append(labelnames, k)
			}
			sort.Strings(labelnames)
			parts := make([]string, 0, len(sample.Labels)*2+1)
			parts = append(parts, name)
			for _, l := range labelnames {
				parts = append(parts, l, sample.Labels[l])
			}
			sample.ID = strings.Join(parts, ".")

			c.ch <- sample
		}
	}
}

func (c *influxDBCollector) processSamples() {
	ticker := time.NewTicker(time.Minute).C
	for {
		select {
		case s := <-c.ch:
			c.mu.Lock()
			c.samples[s.ID] = s
			c.mu.Unlock()

		case <-ticker:
			// Garbage collect expired value lists.
			ageLimit := time.Now().Add(-*sampleExpiry)
			c.mu.Lock()
			for k, sample := range c.samples {
				if ageLimit.After(sample.Timestamp) {
					delete(c.samples, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// Collect implements prometheus.Collector.
func (c *influxDBCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- lastPush

	c.mu.Lock()
	samples := make([]*influxDBSample, 0, len(c.samples))
	for _, sample := range c.samples {
		samples = append(samples, sample)
	}
	c.mu.Unlock()

	ageLimit := time.Now().Add(-*sampleExpiry)
	for _, sample := range samples {
		if ageLimit.After(sample.Timestamp) {
			continue
		}

		metric := prometheus.MustNewConstMetric(
			prometheus.NewDesc(sample.Name, "InfluxDB Metric", []string{}, sample.Labels),
			prometheus.UntypedValue,
			sample.Value,
		)

		if *exportTimestamp {
			metric = prometheus.NewMetricWithTimestamp(sample.Timestamp, metric)
		}
		ch <- metric
	}
}

// Describe implements prometheus.Collector.
func (c *influxDBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- lastPush.Desc()
}

// analog of invalidChars = regexp.MustCompile("[^a-zA-Z0-9_]")
func ReplaceInvalidChars(in *string) {

	for charIndex, char := range *in {
		charInt := int(char)
		if !((charInt >= 97 && charInt <= 122) || // a-z
			(charInt >= 65 && charInt <= 90) || // A-Z
			(charInt >= 48 && charInt <= 57) || // 0-9
			charInt == 95) { // _

			*in = (*in)[:charIndex] + "_" + (*in)[charIndex+1:]
		}
	}
}

func init() {
	prometheus.MustRegister(version.NewCollector("influxdb_exporter"))
	prometheus.MustRegister(udpParseErrors)
}

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("msg", "Starting influxdb_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "context", version.BuildContext())

	c := newInfluxDBCollector(logger)
	prometheus.MustRegister(c)

	addr, err := net.ResolveUDPAddr("udp", *bindAddress)
	if err != nil {
		fmt.Printf("Failed to resolve UDP address %s: %s", *bindAddress, err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Failed to set up UDP listener at address %s: %s", addr, err)
		os.Exit(1)
	}

	c.conn = conn
	go c.serveUdp()

	http.HandleFunc("/write", c.influxDBPost)

	// Some InfluxDB clients try to create a database.
	http.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"results": []}`)
	})

	// Some InfluxDB clients want to check if the http server is an influx endpoint
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		// InfluxDB returns a 204 on success.
		http.Error(w, "", http.StatusNoContent)
	})

	http.Handle(*metricsPath, promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
    <head><title>InfluxDB Exporter</title></head>
    <body>
    <h1>InfluxDB Exporter</h1>
    <p><a href="` + *metricsPath + `">Metrics</a></p>
    </body>
    </html>`))
	})

	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}
