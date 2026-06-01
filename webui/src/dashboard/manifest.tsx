import { DashboardNavSection } from '@byte-v-forge/common-ui';
import type { DashboardModuleRegistration } from '@byte-v-forge/common-ui';
import { GptAccountsPage } from './accounts-page';
import { OpenAIIcon } from './brand-icons';
import { registerGptWorkflowActionRenderers } from './gpt-workflow-actions';
import { registerPrivateDashboardExtensions } from './private-extension';
import './account-actions-layout.css';
import './account-detail-actions.css';
import './account-proxy-history.css';
import './account-workflow-timeline.css';
import './styles.css';

registerGptWorkflowActionRenderers();
registerPrivateDashboardExtensions();

const registration: DashboardModuleRegistration = {
  manifest: {
    id: 'gpt',
    nav: [
      {
        key: 'accounts',
        label: 'GPT',
        icon: 'openai',
        section: DashboardNavSection.DASHBOARD_NAV_SECTION_MAIN,
        required_services: ['gpt-service'],
        order: 10
      }
    ]
  },
  icons: {
    openai: <OpenAIIcon size={17} />
  },
  views: {
    accounts: () => <GptAccountsPage />
  }
};

export default registration;
