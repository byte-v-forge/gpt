import { useEffect } from 'react';
import { ToastMessage, api, useToastMessage } from '@byte-v-forge/common-ui';
import type { GptActionCatalog } from './action-catalog';
import { useGoPayData } from './gopay-data';
import { GoPayLabView } from './view';

export function GoPayLabPage({ actionCatalog }: { actionCatalog?: GptActionCatalog }) {
  const toast = useToastMessage();
  const data = useGoPayData(actionCatalog);

  useEffect(() => {
    if (data.loadError) toast.showError(data.loadError);
  }, [data.loadError, toast.showError]);

  async function loadState(showToast = true) {
    const result = await data.refreshState();
    if (!showToast) return;
    if (result.error) toast.showError(result.error);
    else toast.showToast(result.data?.error_message ? 'error' : 'ok', result.data?.error_message || 'GoPay state 已刷新');
  }

  async function cancelWorkflow(jobId: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/gpt/jobs/${jobId}/cancel`, { method: 'POST', body: JSON.stringify({ reason: 'manual workflow cancel' }) });
    toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || '流程已取消');
    await data.refreshJobs();
  }

  return (
    <>
      <ToastMessage toast={toast.toast} />
      <GoPayLabView
        actionCatalog={actionCatalog}
        state={data.state}
        loading={data.loading}
        currentJob={data.currentJob}
        onLoadState={loadState}
        onRefreshJobs={data.refreshJobs}
        onCancelWorkflow={cancelWorkflow}
        onGoPayActionDone={(message, error) => toast.showToast(error ? 'error' : 'ok', message)}
      />
    </>
  );
}
