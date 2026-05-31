import { createHotStreamURL, useHotStreamInvalidation } from '@byte-v-forge/common-ui';
import type { QueryKey } from '@tanstack/react-query';
import type { HotStreamEvent } from '@byte-v-forge/common-ui';

export type JobListInvalidationTarget = {
  queryKey: QueryKey;
};

export type JobEventCacheOptions = {
  apiBase?: string;
  enabled?: boolean;
  jobIds?: string[];
  lists?: JobListInvalidationTarget[];
  details?: QueryKey[];
  onEvent?: (event: HotStreamEvent) => void;
};

export function useJobEventCache(options: JobEventCacheOptions) {
  const rules = [
    ...(options.lists || []).map((target) => ({ queryKey: target.queryKey, eventTypes: ['gpt.job.updated'], resourceTypes: ['gpt.job'] })),
    ...(options.details || []).map((queryKey) => ({ queryKey, eventTypes: ['gpt.job.updated'], resourceTypes: ['gpt.job'], resourceIds: jobResourceIds(queryKey, options.jobIds) }))
  ];
  useHotStreamInvalidation({
    enabled: options.enabled !== false && rules.length > 0,
    url: createHotStreamURL(options.apiBase || '/api/gpt', { eventTypes: ['gpt.job.updated'], resourceTypes: ['gpt.job'], resourceIds: options.jobIds }),
    rules,
    onEvent: options.onEvent
  });
}

function jobResourceIds(queryKey: QueryKey, ids?: string[]) {
  const explicit = (ids || []).map((id) => id.trim()).filter(Boolean);
  if (explicit.length > 0) return explicit;
  const tail = queryKey[queryKey.length - 1];
  return typeof tail === 'string' && tail ? [tail] : undefined;
}
