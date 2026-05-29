import { api, type ListMailboxDomainsResponse, type ListMailboxProviderCapabilitiesResponse, useQuery } from '@byte-v-forge/common-ui';

export const createAccountMailboxQueryKeys = {
  domains: ['gpt', 'create-account', 'mailbox-domains'] as const,
  providerCapabilities: ['gpt', 'create-account', 'mailbox-provider-capabilities'] as const
};

export function useCreateAccountMailboxData(enabled: boolean) {
  const domainsQuery = useQuery({
    queryKey: createAccountMailboxQueryKeys.domains,
    queryFn: () => api<ListMailboxDomainsResponse>('/api/mailbox/domains'),
    enabled
  });
  const providerCapabilitiesQuery = useQuery({
    queryKey: createAccountMailboxQueryKeys.providerCapabilities,
    queryFn: () => api<ListMailboxProviderCapabilitiesResponse>('/api/mailbox/provider-capabilities'),
    enabled
  });

  return {
    domains: Array.isArray(domainsQuery.data?.domains) ? domainsQuery.data.domains : [],
    providerCapabilities: Array.isArray(providerCapabilitiesQuery.data?.providers) ? providerCapabilitiesQuery.data.providers : []
  };
}
