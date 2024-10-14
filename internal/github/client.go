package github

import (
	"context"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/wayfair-incubator/telefonistka/pkg/githubapi"
)

func NewGhClientPair(ctx context.Context, repository string) *githubapi.GhClientPair {
	var clientPair githubapi.GhClientPair
	clientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	clientPair.GetAndCache(
		clientCache,
		"GITHUB_APP_ID",
		"GITHUB_APP_PRIVATE_KEY_PATH",
		"GITHUB_OAUTH_TOKEN",
		strings.Split(repository, "/")[0],
		ctx,
	)
	return &clientPair
}
