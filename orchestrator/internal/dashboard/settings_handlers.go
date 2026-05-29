package dashboard

import (
	"errors"
	"net/http"

	"orchestrator/internal/actionregistry"
	"orchestrator/pb"
)

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusBadGateway, errors.New("GPT settings store is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := s.settings.Get(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, &pb.GetGPTSettingsResponse{
			Settings:      settings,
			PluginSchemas: pluginConfigSchemasProto(s.actionRegistry.PluginConfigs()),
		})
	case http.MethodPut, http.MethodPost:
		var req pb.UpdateGPTSettingsRequest
		if err := readProtoJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		settings, err := s.settings.Update(r.Context(), req.GetSettings())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeProtoJSON(w, http.StatusOK, &pb.UpdateGPTSettingsResponse{Settings: settings})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut+", "+http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func pluginConfigSchemasProto(schemas []actionregistry.ConfigSchema) []*pb.GPTPluginConfigSchema {
	out := make([]*pb.GPTPluginConfigSchema, 0, len(schemas))
	for _, schema := range schemas {
		fields := make([]*pb.GPTPluginConfigField, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			fields = append(fields, &pb.GPTPluginConfigField{
				Key:          field.Key,
				Label:        field.Label,
				Kind:         pluginConfigFieldKindProto(field.Kind),
				DefaultValue: field.DefaultValue,
				Required:     field.Required,
				HelpText:     field.HelpText,
			})
		}
		out = append(out, &pb.GPTPluginConfigSchema{
			PluginKey:   schema.PluginKey,
			DisplayName: schema.DisplayName,
			Owner:       schema.Owner,
			Fields:      fields,
		})
	}
	return out
}

func pluginConfigFieldKindProto(kind actionregistry.ConfigFieldKind) pb.GPTPluginConfigFieldKind {
	switch kind {
	case actionregistry.ConfigFieldString:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_STRING
	case actionregistry.ConfigFieldSecret:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_SECRET
	case actionregistry.ConfigFieldInteger:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_INTEGER
	case actionregistry.ConfigFieldBoolean:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_BOOLEAN
	case actionregistry.ConfigFieldDurationSeconds:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_DURATION_SECONDS
	case actionregistry.ConfigFieldURL:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_URL
	default:
		return pb.GPTPluginConfigFieldKind_GPT_PLUGIN_CONFIG_FIELD_KIND_UNSPECIFIED
	}
}
