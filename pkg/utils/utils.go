package utils

import (
	"fmt"
	"log"
	"os"

	"github.com/mikefarah/yq/v4/pkg/yqlib"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func MustGetEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Fatalf("%s environment variable is required", key)
	os.Exit(3)
	return ""
}

func UpdateYaml(yamlContent, key, value string) (string, error) {
	yqExpression := fmt.Sprintf("(%s)=\"%s\"", key, value)
	preferences := yqlib.NewDefaultYamlPreferences()
	evaluate, err := yqlib.NewStringEvaluator().Evaluate(yqExpression, yamlContent, yqlib.NewYamlEncoder(preferences), yqlib.NewYamlDecoder(preferences))
	if err != nil {
		return "", err
	}
	return evaluate, nil
}
