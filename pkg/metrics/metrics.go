package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	Admitted = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "admitted",
		Namespace: "tobac",
		Help:      "number of requests admitted",
	})
	Denied = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "denied",
		Namespace: "tobac",
		Help:      "number of requests denied",
	})
)

func init() {
	prometheus.MustRegister(Admitted)
	prometheus.MustRegister(Denied)
}

func isAlive(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Alive.")
}

func isReady(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Ready.")
}

// Serve health and metric requests forever.
func Serve(addr, metrics, ready, alive string) {
	h := http.NewServeMux()
	h.Handle(metrics, promhttp.Handler())
	h.HandleFunc(ready, isReady)
	h.HandleFunc(alive, isAlive)
	log.Infof("Metrics and status server started on %s", addr)
	log.Infof("Serving metrics on %s", metrics)
	log.Infof("Serving readiness check on %s", ready)
	log.Infof("Serving liveness check on %s", alive)
	log.Info(http.ListenAndServe(addr, h))
}
