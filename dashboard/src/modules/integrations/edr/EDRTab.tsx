import Breadcrumbs from "@components/Breadcrumbs";
import InlineLink from "@components/InlineLink";
import Paragraph from "@components/Paragraph";
import * as Tabs from "@radix-ui/react-tabs";
import { ExternalLinkIcon, ShieldCheck } from "lucide-react";
import React from "react";
import IntegrationIcon from "@/assets/icons/IntegrationIcon";
import { Account } from "@/interfaces/Account";
import { CrowdStrike } from "@/modules/integrations/edr/crowdstrike/CrowdStrike";
import { FleetDM } from "@/modules/integrations/edr/fleetdm/FleetDM";
import { Huntress } from "@/modules/integrations/edr/huntress/Huntress";
import { SentinelOne } from "@/modules/integrations/edr/sentinel-one/SentinelOne";
import { useIntegrations } from "../idp-sync/useIntegrations";
import { Intune } from "./intune/Intune";

type Props = {
  account: Account;
};
export default function EDRTab({ account }: Props) {
  useIntegrations();

  return (
    <Tabs.Content value={"edr"}>
      <div className={"p-default py-6"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/integrations"}
            label={"Integrations"}
            icon={<IntegrationIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/integrations?tab=edr"}
            label={"MDM & EDR"}
            icon={<ShieldCheck size={15} />}
            active
          />
        </Breadcrumbs>
        <h1>MDM & EDR</h1>
        <Paragraph className={"max-w-3xl"}>
          Endpoint Detection and Response (EDR) and Mobile Device Management
          (MDM) integrations allow you to restrict network access only to
          devices managed by the IT department.
        </Paragraph>
        <Paragraph>
          <InlineLink
            href={
              "https://docs.netbird.io/how-to/endpoint-detection-and-response"
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
