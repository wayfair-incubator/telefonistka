package telefonistka

import (
	"log"
	"os"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getCrucialEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Fatalf("%s environment variable is required", key)
	os.Exit(3)
	return ""
}
