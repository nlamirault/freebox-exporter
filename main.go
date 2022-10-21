package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/trazfr/freebox-exporter/fbx"
)

const (
	EXPORTER = "freebox-exporter"
)

var (
	webConfig  = webflag.AddFlags(kingpin.CommandLine, ":9091")
	metricPath = kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_METRICS_PATH").Default("/metrics").String()
	credentials = kingpin.Flag(
		"credentials",
		"Token file for the Freebox API.",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_CREDENTIALS").String()
	hostDetails = kingpin.Flag(
		"hostDetails",
		"get details about the hosts connected to wifi and ethernet. This increases the number of metrics.",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_HOST_DETAILS").Default("false").Bool()
	discovery = kingpin.Flag(
		"httpDiscovery",
		"use http://mafreebox.freebox.fr/api_version to discover the Freebox at the first run (by default: use mDNS).",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_DISCOVERY").Default("false").Bool()
	apiVersion = kingpin.Flag(
		"apiVersion",
		"Force the API version (by default use the latest one).",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_API_VERSION").Default("0").Int()
	debug = kingpin.Flag(
		"debug",
		"enable the debug mode.",
	).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_DEBUG").Default("false").Bool()
)

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print(EXPORTER))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", EXPORTER), "version", version.Info())
	level.Debug(logger).Log("msg", "Build context", version.BuildContext())

	discoveryMode := fbx.FreeboxDiscoveryMDNS
	if *discovery {
		discoveryMode = fbx.FreeboxDiscoveryHTTP
	}

	collector, err := NewCollector(logger, *credentials, discoveryMode, *apiVersion, *hostDetails, *debug)
	if err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("can't create Prometheus collector: %s", err))
	}
	defer collector.Close()

	prometheus.MustRegister(collector)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Freebox Exporter</title></head>
             <body>
             <h1>Freebox Exporter</h1>
             <p><a href='` + *metricPath + `'>Metrics</a></p>
			 <h2>Build</h2>
             <pre>` + version.Info() + ` ` + version.BuildContext() + `</pre>
             </body>
             </html>`))
	})
	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})
	http.Handle(*metricPath,
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer,
			promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{
					// ErrorLog: &promHTTPLogger{
					// 	logger: logger,
					// },
				},
			),
		),
	)

	server := &http.Server{}
	if err := web.ListenAndServe(server, webConfig, logger); err != nil {
		log.Fatal(err)
	}
}
