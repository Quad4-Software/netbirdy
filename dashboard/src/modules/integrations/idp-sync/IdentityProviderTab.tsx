import Breadcrumbs from "@components/Breadcrumbs";
import { Callout } from "@components/Callout";
import InlineLink from "@components/InlineLink";
import Paragraph from "@components/Paragraph";
import { SkeletonIntegration } from "@components/skeletons/SkeletonIntegration";
import * as Tabs from "@radix-ui/react-tabs";
import { isNetBirdCloud } from "@utils/netbirdy";
import { ExternalLinkIcon, FingerprintIcon } from "lucide-react";
import React from "react";
import IntegrationIcon from "@/assets/icons/IntegrationIcon";
import { useAccount } from "@/modules/account/useAccount";
import { AzureAD } from "@/modules/integrations/idp-sync/azure-ad/AzureAD";
import { EntraSCIM } from "@/modules/integrations/idp-sync/entra-scim/EntraSCIM";
import { GenericSCIM } from "@/modules/integrations/idp-sync/generic-scim/GenericSCIM";
import { GoogleWorkspace } from "@/modules/integrations/idp-sync/google-workspace/GoogleWorkspace";
import { Jumpcloud } from "@/modules/integrations/idp-sync/jumpcloud/Jumpcloud";
import { Okta } from "@/modules/integrations/idp-sync/okta-scim/Okta";
import { useIntegrations } from "@/modules/integrations/idp-sync/useIntegrations";
import { docsLink } from "@/modules/docs/links";

export default function IdentityProviderTab() {
  const account = useAccount();

  useIntegrations();

  return (
    <Tabs.Content value={"identity-provider"}>
      <div className={"p-default py-6"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/integrations"}
            label={"Integrations"}
            icon={<IntegrationIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=identity-provider"}
            label={"Identity Provider Sync"}
            icon={<FingerprintIcon size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Identity Provider Sync</h1>
        <Paragraph>
          Configure your preferred Identity Provider (IdP) to synchronize your
          users and groups to NetBirdy.
        </Paragraph>
        <Paragraph>
          <InlineLink
            href={docsLink("https://docs.netbird.io/how-to/idp-sync")}
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
