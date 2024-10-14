package mocks

// This package contains generated mocks

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=argocd_application.go -package=mocks github.com/argoproj/argo-cd/v2/pkg/apiclient/application ApplicationServiceClient
