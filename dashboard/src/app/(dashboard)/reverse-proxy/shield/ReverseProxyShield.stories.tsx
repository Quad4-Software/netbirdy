import type { Meta, StoryObj } from "@storybook/nextjs-vite";
import ShieldPage from "@/app/(dashboard)/reverse-proxy/shield/page";
import DashboardLayout from "@/layouts/DashboardLayout";
import { clickRole, clickTestId, findText, settle } from "@/storybook/platform";

const meta: Meta = {
  title: "Pages/Platform/Reverse Proxy/Shield",
  parameters: {
    nextjs: { navigation: { pathname: "/reverse-proxy/shield" } },
  },
  render: () => (
    <DashboardLayout>
      <ShieldPage />
    </DashboardLayout>
  ),
};
export default meta;
type Story = StoryObj;

const loaded = async () => {
  await findText("proxy-eu-01");
  await settle();
};

export const List: Story = {
  play: loaded,
};

export const Empty: Story = {
  parameters: {
    api: {
      "GET /proxy/shield": {
        config: {
          mode: "off",
          allow_cidrs: [],
          ai_crawlers: "deny",
          allow_search_engines: false,
          allow_local: true,
        },
        proxies: [],
      },
    },
  },
  play: async () => {
    await findText(/No proxies are connected/);
    await settle();
  },
};

export const EnforceWarning: Story = {
  play: async () => {
    await loaded();
    await clickTestId("shield-mode-trigger");
    await clickRole("option", "Enforce");
    await findText(/Enforce mode challenges/);
    await settle();
  },
};
