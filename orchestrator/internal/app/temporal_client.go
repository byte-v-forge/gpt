package app

import (
	workflowruntime "github.com/byte-v-forge/workflow-runtime"
	temporalclient "go.temporal.io/sdk/client"
)

func newTemporalClient(cfg orchestratorConfig) (temporalclient.Client, func(), error) {
	client, err := workflowruntime.Dial(cfg.WorkflowRuntime)
	if err != nil {
		return nil, nil, err
	}
	return client, client.Close, nil
}
