import { DashboardNavSection } from '@/dashboard/module-kit';
import type { DashboardModuleRegistration } from '@/dashboard/module-kit';
import { GptAccountsPage } from './accounts-page';
import { GoPayWalletIcon, OpenAIIcon } from './brand-icons';
import { GoPayLabPage } from './gopay-page';
import { registerGptWorkflowActionRenderers } from './gpt-workflow-actions';
import { registerGoPayWorkflowRenderers } from './gopay-workflow-renderers';
import './gopay-actions.css';
import './gopay-workflow.css';
import './account-actions-layout.css';
import './account-detail-actions.css';
import './styles.css';

registerGoPayWorkflowRenderers();
registerGptWorkflowActionRenderers();

const registration: DashboardModuleRegistration = {
  manifest: {
    id: 'gpt',
    nav: [
      {
        key: 'accounts',
        label: 'GPT账号',
        icon: 'openai',
        section: DashboardNavSection.DASHBOARD_NAV_SECTION_MAIN,
        required_services: ['gpt'],
        order: 10
      },
      {
        key: 'gopay',
        label: 'GoPay',
        icon: 'gopay',
        section: DashboardNavSection.DASHBOARD_NAV_SECTION_LAB,
        required_services: ['gpt'],
        order: 100
      }
    ]
  },
  icons: {
    openai: <OpenAIIcon size={17} />,
    gopay: <GoPayWalletIcon size={17} />
  },
  views: {
    accounts: () => <GptAccountsPage />,
    gopay: () => <GoPayLabPage />
  }
};

export default registration;
