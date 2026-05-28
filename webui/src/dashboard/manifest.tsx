import { DashboardNavSection } from '@byte-v-forge/common-ui';
import type { DashboardModuleRegistration } from '@byte-v-forge/common-ui';
import { GptAccountsPage } from './accounts-page';
import { OpenAIIcon } from './brand-icons';
import { registerGptWorkflowActionRenderers } from './gpt-workflow-actions';
import { registerGoPayWorkflowRenderers } from './gopay-workflow-renderers';
import './gopay-actions.css';
import './gopay-workflow.css';
import './account-actions-layout.css';
import './account-detail-actions.css';
import './account-proxy-history.css';
import './styles.css';

registerGoPayWorkflowRenderers();
registerGptWorkflowActionRenderers();

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
