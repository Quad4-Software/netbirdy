import Breadcrumbs from "@components/Breadcrumbs";
import InlineLink from "@components/InlineLink";
import Paragraph from "@components/Paragraph";
import { SkeletonIntegration } from "@components/skeletons/SkeletonIntegration";
import * as Tabs from "@radix-ui/react-tabs";
import { ExternalLinkIcon, KeyRoundIcon } from "lucide-react";
import React from "react";
import IntegrationIcon from "@/assets/icons/IntegrationIcon";
import { useAccount } from "@/modules/account/useAccount";
import { OktaSSOIntegrationCard } from "@/modules/integrations/sso/okta/OktaSSOIntegrationCard";

export default function SSOTab() {
  const account = useAccount();

  return (
    <Tabs.Content value={"sso"}>
      <div className={"p-default py-6"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/integrations"}
            label={"Integrations"}
            icon={<IntegrationIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=sso"}
            label={"Single Sign-On"}
            icon={<KeyRoundIcon size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Single Sign-On</h1>
        <Paragraph>
          Configure your preferred Identity Provider (IdP) to enable Single
          Sign-On (SSO) for your team.
        </Paragraph>
        <Paragraph>
          <InlineLink
            href={
              "https://docs.netbird.io/how-to/okta-sync#configuring-sso-in-okta"
            }
            target={"_blank"}
          >
            Learn more
            <ExternalLinkIcon size={12} />
          </InlineLink>
        </Paragraph>

        

        
      </div>
    </Tabs.Content>
  );
}
