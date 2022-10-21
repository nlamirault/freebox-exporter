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
	application = "freebox-exporter"
)

var (
	webConfig = webflag.AddFlags(kingpin.CommandLine, ":9091")
	// listen    = kingpin.Flag(
	// 	"address",
	// 	"Address to listen on for web interface and telemetry.",
	// ).OverrideDefaultFromEnvar("FREEBOX_EXPORTER_ADDR").Default(":9091").String()
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

// func usage() {
// 	fmt.Fprintf(flag.CommandLine.Output(),
// 		"Usage: %s [options] <api_token_file>\n"+
// 			"\n"+
// 			"api_token_file: file to store the token for the API\n"+
// 			"\n"+
// 			"options:\n",
// 		os.Args[0])
// 	flag.PrintDefaults()
// }

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("freebox-exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting freebox-exporter", "version", version.Info())
	level.Debug(logger).Log("msg", "Build context", "context", version.BuildContext())

	// flag.Usage = usage
	// debugPtr := flag.Bool("debug", false, "enable the debug mode")
	// hostDetailsPtr := flag.Bool("hostDetails", false, "get details about the hosts connected to wifi and ethernet. This increases the number of metrics")
	// httpDiscoveryPtr := flag.Bool("httpDiscovery", false, "use http://mafreebox.freebox.fr/api_version to discover the Freebox at the first run (by default: use mDNS)")
	// apiVersionPtr := flag.Int("apiVersion", 0, "Force the API version (by default use the latest one)")
	// // listenPtr := flag.String("listen", ":9091", "listen to address")
	// flag.Parse()

	// args := flag.Args()
	// if len(args) < 1 {
	// 	fmt.Fprintf(flag.CommandLine.Output(), "ERROR: api_token_file not defined\n")
	// 	usage()
	// 	os.Exit(1)
	// } else if len(args) > 1 {
	// 	fmt.Fprintf(flag.CommandLine.Output(), "ERROR: too many arguments\n")
	// 	usage()
	// 	os.Exit(1)
	// }
	// if *debugPtr {
	// 	log.InitDebug()
	// } else {
	// 	log.Init()
	// }

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
	// http.Handle("/metrics", promhttp.Handler())
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

	// level.Info(logger).Log("msg", "Starting HTTP server", "port", listen)
	// log.Error.Println(http.ListenAndServe(*listen, nil))
	// srv := &http.Server{Addr: *listen}
	// if err := web.ListenAndServe(srv, webConfig, logger); err != nil {
	// 	level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
	// 	os.Exit(1)
	// }

	server := &http.Server{}

	if err := web.ListenAndServe(server, webConfig, logger); err != nil {
		log.Fatal(err)
	}
}
