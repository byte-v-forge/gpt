//go:build !private_plugins

package app

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func configurePrivateDependencies(context.Context, orchestratorConfig, *orchestratorDependencies, redis.Cmdable) error {
	return nil
}
