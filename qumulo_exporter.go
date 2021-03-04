package main

//equire github.com/go-resty/resty/v2 v2.3.0

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/go-resty/resty/v2"
)

const (
	namespace  = "qumulo"
	tokenHours = 4 //hours
)

// AuthSuccess Token auth for API
type AuthSuccess struct {
	Token string `json:"bearer_token"`
}

type metricInfo struct {
	Desc *prometheus.Desc
	Type prometheus.ValueType
}

// Exporter collects Qumulo stats from the given URI and exports them using
// the prometheus metrics package.
type Exporter struct {
	URI   string
	Token string
	//Timer     *time.Timer
	mutex     sync.RWMutex
	fetchInfo func() (io.ReadCloser, error)
	fetchStat func() (io.ReadCloser, error)

	up                             prometheus.Gauge
	totalScrapes, csvParseFailures prometheus.Counter
	serverMetrics                  map[int]metricInfo
	excludedServerStates           map[string]struct{}
	logger                         log.Logger
}

var (
	qumuloUp = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Was the last scrape of Qumulo successful.", nil, nil)
)

// Exporter collects HAProxy stats from the given URI and exports them using
// the prometheus metrics package.

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, sslVerify bool, username, password string, timeout time.Duration, logger log.Logger) (*Exporter, error) {
	//u, err := url.Parse(uri)
	token, err := getToken(uri, username, password, sslVerify)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating an exporter", "err", err)
		os.Exit(1)
	}
	var fetchInfo func() (io.ReadCloser, error)
	var fetchStat func() (io.ReadCloser, error)
	//timer := time.NewTimer(time.Hour * time.Duration(tokenHours))
	return &Exporter{
		URI:   uri,
		Token: token,

		fetchInfo: fetchInfo,
		fetchStat: fetchStat,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the last scrape of qumulo successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_scrapes_total",
			Help:      "Current total qumulo scrapes.",
		}),
		logger: logger,
	}, nil
}

//func getIOPS(token string) {
//	uri := "analytics/iops"
//}

//func getActivity(token string) {
//	uri := "analytics/activity/current"
//}

func getToken(apiuri, username, password string, sslVerify bool) (string, error) {
	client := resty.New()
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: sslVerify})
	reqBody := map[string]string{
		"username": username,
		"password": password,
	}
	// POST JSON string
	// No need to set content type, if you have client level setting
	var auth AuthSuccess
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetResult(&auth). // or SetResult(AuthSuccess{}).
		Post(apiuri + "/session/login")

	if err != nil {
		return "", errors.New("Unable to authenticate")
	}
	if !(resp.StatusCode() >= 200 && resp.StatusCode() < 300) {
		return "", errors.New("Unable to authenticate - Received: " + fmt.Sprint(resp.StatusCode()))
	}
	return auth.Token, nil
}

// Describe describes all the metrics ever exported by the HAProxy exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	//for _, m := range frontendMetrics {
	//	ch <- m.Desc
	//}
	//for _, m := range backendMetrics {
	//	ch <- m.Desc
	//}
	//for _, m := range e.serverMetrics {
	//	ch <- m.Desc
	//}
	ch <- qumuloUp
	ch <- e.totalScrapes.Desc()
}

// Collect fetches the stats from configured HAProxy location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	up := e.scrape(ch)

	ch <- prometheus.MustNewConstMetric(qumuloUp, prometheus.GaugeValue, up)
	ch <- e.totalScrapes
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) (up float64) {
	e.totalScrapes.Inc()
	// introduce for loop
	//body, err := e.fetchStat()

	//if err != nil {
	//	level.Error(e.logger).Log("msg", "Can't scrape HAProxy", "err", err)
	//	return 0
	//}
	return 1
}

// Create a Resty Client
func main() {
	var (
		listenAddress   = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9101").String()
		metricsPath     = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		qumAPIURI       = kingpin.Flag("qumuloapi.scrape-uri", "URI on which to scrape Qumulo API.").Default("http://localhost:8000/v1").String()
		qumAPISSLVerify = kingpin.Flag("qumuloapi.ssl-verify", "Flag that enables SSL certificate verification for the scrape URI").Default("true").Bool()
		qumAPIUsername  = kingpin.Flag("qumuloapi.username", "Username to authenticate with API.").Required().String()
		qumAPIPassword  = kingpin.Flag("qumuloapi.password", "Password to authenticate with API.").Required().String()
		//haProxyServerMetricFields  = kingpin.Flag("haproxy.server-metric-fields", "Comma-separated list of exported server metrics. See http://cbonte.github.io/haproxy-dconv/configuration-1.5.html#9.1").Default(serverMetrics.String()).String()
		//ExcludeStates = kingpin.Flag("haproxy.server-exclude-states", "Comma-separated list of exported server states to exclude. See https://cbonte.github.io/haproxy-dconv/1.8/management.html#9.1, field 17 statuus").Default(excludedServerStates).String()
		Timeout = kingpin.Flag("qumuloapi.timeout", "Timeout for trying to get stats from Qumulo API.").Default("5s").Duration()
	)
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("qumulo_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("msg", "Attempting initial auth to "+*qumAPIURI)
	exporter, err := NewExporter(*qumAPIURI, *qumAPISSLVerify, *qumAPIUsername, *qumAPIPassword, *Timeout, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating an exporter", "err", err)
		os.Exit(1)
	}
	prometheus.MustRegister(exporter)
	//prometheus.MustRegister(version.NewCollector(exporter))

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Qumulo Exporter</title></head>
             <body>
             <h1>Qumulo Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}

}
