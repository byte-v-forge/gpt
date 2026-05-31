import { useState } from 'react';
import { Button, Input, api, useAsyncActionRunner } from '@byte-v-forge/common-ui';
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
  const [values, setValues] = useState<Record<string, string>>({});
  const runner = useAsyncActionRunner();
  if (!interaction) return null;
  const activeInteraction = interaction;
  const value = values[activeInteraction.valueKey] || '';

  async function run(action: WorkflowJobInteractionAction) {
    await runner.tryRun(action.key, async () => {
      const resp = await api<{ success?: boolean; error_message?: string }>(action.url, { method: 'POST', body: JSON.stringify(action.body(value)) });
      props.onMessage?.(resp.error_message ? 'error' : 'ok', resp.error_message || action.successText);
      if (!resp.error_message) await props.onChanged?.();
      if (!resp.error_message && action.clearOnSuccess) setValues((current) => ({ ...current, [activeInteraction.valueKey]: '' }));
    }, { onError: props.onError });
  }

  return (
    <div className="gptWorkflowActionCard">
      <div className="gptWorkflowActionHead">
        <strong>{activeInteraction.title}</strong>
        <span>{activeInteraction.subtitle}</span>
      </div>
      <form className="gptWorkflowOtpForm" onSubmit={(event) => {
        event.preventDefault();
        if (activeInteraction.submit.enabled(value)) void run(activeInteraction.submit);
      }}>
        <Input {...activeInteraction.input} value={value} onChange={(event) => setValues((current) => ({ ...current, [activeInteraction.valueKey]: event.target.value }))} />
        <WorkflowInteractionButton type="submit" action={activeInteraction.submit} pending={runner.activeKey} value={value} />
        {activeInteraction.secondary && <WorkflowInteractionButton type="button" action={activeInteraction.secondary} pending={runner.activeKey} value={value} onClick={() => void run(activeInteraction.secondary!)} />}
      </form>
    </div>
  );
}

function WorkflowInteractionButton({ type, action, pending, value, onClick }: { type: 'button' | 'submit'; action: WorkflowJobInteractionAction; pending: string; value: string; onClick?: () => void }) {
  return <Button type={type} variant={action.variant} disabled={!!pending || !action.enabled(value)} onClick={onClick}>{action.icon}{pending === action.key ? action.pendingLabel : action.label}</Button>;
}
