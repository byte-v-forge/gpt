//go:build !private_plugins

package main

import "github.com/byte-v-forge/gpt/pkg/gptplugin"

func privatePlugins() []gptplugin.Plugin {
	return nil
}
