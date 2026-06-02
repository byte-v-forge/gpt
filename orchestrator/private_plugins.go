//go:build private_plugins

package main

import (
	privateplugins "github.com/byte-v-forge/gpt-private/plugins"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
)

func privatePlugins() []gptplugin.Plugin {
	return privateplugins.Plugins()
}
