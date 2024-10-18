package testutils

import (
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

// Quiet suppresses logs when running go test.
func Quiet() func() {
	log.SetOutput(io.Discard)
	return func() {
		log.SetOutput(os.Stdout)
	}
}
