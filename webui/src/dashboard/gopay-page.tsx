import { useEffect } from 'react';
import { ToastMessage, useToastMessage } from '@/dashboard/module-kit';
import { useGoPayData } from './gopay-data';
import { GoPayLabView } from './view';

export function GoPayLabPage() {
  const toast = useToastMessage();
  const data = useGoPayData();

  useEffect(() => {
    if (data.loadError) toast.showError(data.loadError);
  }, [data.loadError, toast.showError]);

  async function loadState(showToast = true) {
    const result = await data.refreshState();
    if (!showToast) return;
    if (result.error) toast.showError(result.error);
    else toast.showToast(result.data?.error_message ? 'error' : 'ok', result.data?.error_message || 'GoPay state 已刷新');
  }

  return (
    <>
      <ToastMessage toast={toast.toast} />
      <GoPayLabView
        state={data.state}
        loading={data.loading}
        currentJob={data.currentJob}
        onLoadState={loadState}
        onRefreshJobs={data.refreshJobs}
        onGoPayActionDone={(message, error) => toast.showToast(error ? 'error' : 'ok', message)}
      />
    </>
  );
}
