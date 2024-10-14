package github

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/wayfair-incubator/telefonistka/pkg/githubapi"
)

func Event(eventType string, eventFilePath string) {
	mainGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	prApproverGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	githubapi.ReciveEventFile(eventFilePath, eventType, mainGhClientCache, prApproverGhClientCache)
}
