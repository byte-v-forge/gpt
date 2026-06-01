import { api, OneTimeOTPSubmit, type OneTimeOTPSubmitAction } from '@byte-v-forge/common-ui';
import { registerWorkflowJobActionRenderers, type WorkflowJobActionRendererProps } from './job-action-renderers';
import { workflowJobInteraction, type WorkflowJobInteractionAction } from './workflow-interaction-specs';

let registered = false;

export function registerGptWorkflowActionRenderers() {
  if (registered) return;
  registered = true;
  registerWorkflowJobActionRenderers([{
    id: 'gpt.workflow.actions',
    statuses: ['RUNNING'],
    render: (props) => <GptWorkflowActions {...props} />
  }]);
}

function GptWorkflowActions(props: WorkflowJobActionRendererProps) {
  const interaction = workflowJobInteraction(props.job, props.progress);
  if (!interaction) return null;
  const activeInteraction = interaction;

  return (
    <OneTimeOTPSubmit
      className="gptWorkflowActionCard"
      formClassName="gptWorkflowOtpForm"
      title={<strong>{activeInteraction.title}</strong>}
      subtitle={activeInteraction.subtitle}
      input={activeInteraction.input}
      submit={toOneTimeOTPAction(activeInteraction.submit, props)}
      secondary={activeInteraction.secondary ? toOneTimeOTPAction(activeInteraction.secondary, props) : undefined}
      onError={props.onError}
    />
  );
}

function toOneTimeOTPAction(action: WorkflowJobInteractionAction, props: WorkflowJobActionRendererProps): OneTimeOTPSubmitAction {
  return {
    key: action.key,
    label: action.label,
    pendingLabel: action.pendingLabel,
    icon: action.icon,
    variant: action.variant,
    enabled: action.enabled,
    clearOnSuccess: action.clearOnSuccess,
    onRun: async (value) => {
      const resp = await api<{ success?: boolean; error_message?: string }>(action.url, { method: 'POST', body: JSON.stringify(action.body(value)) });
      if (resp.error_message) throw new Error(resp.error_message);
      props.onMessage?.('ok', action.successText);
      await props.onChanged?.();
    },
  };
}
