//go:build !private_plugins

package app

import (
	"google.golang.org/grpc"
	"orchestrator/internal/api"
)

func registerPrivateWorkflowServices(_ *grpc.Server, _ *api.Server) {}
