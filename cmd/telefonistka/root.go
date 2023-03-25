package telefonistka

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "telefonistka",
	Version: "0.0.0",
	Short:   "telefonistka - Safe and Controlled GitOps Promotion Across Environments/Failure-Domains",
	Long: `Telefonistka is a Github webhook server/CLI tool that facilitates change promotion across environments/failure domains in Infrastructure as Code GitOps repos

see https://github.com/wayfair-incubator/telefonistka`,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func Execute() {
	switch getEnv("LOG_LEVEL", "info") {
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "panic":
		log.SetLevel(log.PanicLevel)
	}

	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		// ForceColors: true,
		FullTimestamp: true,
	}) // TimestampFormat
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
