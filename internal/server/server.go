package server

import (
	"net/http"
	"time"

	"github.com/alexliesenfeld/health"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/wayfair-incubator/telefonistka/pkg/githubapi"
	"github.com/wayfair-incubator/telefonistka/pkg/utils"
)

func handleWebhook(githubWebhookSecret []byte, mainGhClientCache *lru.Cache[string, githubapi.GhClientPair], prApproverGhClientCache *lru.Cache[string, githubapi.GhClientPair]) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := githubapi.ReciveWebhook(r, mainGhClientCache, prApproverGhClientCache, githubWebhookSecret)
		if err != nil {
			log.Errorf("error handling webhook: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func Serve() {
	githubWebhookSecret := []byte(utils.MustGetEnv("GITHUB_WEBHOOK_SECRET"))
	livenessChecker := health.NewChecker() // No checks for the moment, other then the http server availability
	readinessChecker := health.NewChecker()

	// mainGhClientCache := map[string]githubapi.GhClientPair{} //GH apps use a per-account/org client
	mainGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	prApproverGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook(githubWebhookSecret, mainGhClientCache, prApproverGhClientCache))
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/live", health.NewHandler(livenessChecker))
	mux.Handle("/ready", health.NewHandler(readinessChecker))

	srv := &http.Server{
		Handler:      mux,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Infoln("server started")
	log.Fatal(srv.ListenAndServe())
}
