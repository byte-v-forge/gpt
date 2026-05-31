//go:build private_plugins

package dashboard

func init() {
	registerActionHandlerRegistrar(n8nRegisterActionBindings)
}
