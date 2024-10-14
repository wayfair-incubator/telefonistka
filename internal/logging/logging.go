package logging

import (
	log "github.com/sirupsen/logrus"

	"github.com/wayfair-incubator/telefonistka/pkg/utils"
)

var logLevels = map[string]log.Level{
	"debug": log.DebugLevel,
	"info":  log.InfoLevel,
	"warn":  log.WarnLevel,
	"error": log.ErrorLevel,
	"fatal": log.FatalLevel,
	"panic": log.PanicLevel,
}

func ConfigureLogging() {
	if logLevel, ok := logLevels[utils.GetEnv("LOG_LEVEL", "info")]; ok {
		if logLevel == log.DebugLevel {
			log.SetReportCaller(true)
		}
		log.SetLevel(log.DebugLevel)
	}
	log.SetFormatter(&log.TextFormatter{DisableColors: false, FullTimestamp: true})
}
