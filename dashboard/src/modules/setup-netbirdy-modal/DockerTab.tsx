import Button from "@components/Button";
import Code from "@components/Code";
import InlineLink from "@components/InlineLink";
import Steps from "@components/Steps";
import TabsContentPadding, { TabsContent } from "@components/Tabs";
import { IconBrandUbuntu } from "@tabler/icons-react";
import { GRPC_API_ORIGIN } from "@utils/netbirdy";
import { ExternalLinkIcon } from "lucide-react";
import Link from "next/link";
import React from "react";
import { OperatingSystem } from "@/interfaces/OperatingSystem";
import { RoutingPeerSetupKeyInfo } from "@/modules/setup-netbirdy-modal/SetupModal";
import { docsLink } from "@/modules/docs/links";

type Props = {
  setupKey?: string;
  setupKeyContent?: React.ReactNode;
  setupKeyPlaceholder?: string;
  showSetupKeyInfo?: boolean;
  hostname?: string;
};

export default function DockerTab({
  setupKey,
  setupKeyContent,
  setupKeyPlaceholder,
  showSetupKeyInfo = false,
  hostname,
}: Readonly<Props>) {
  const offset = setupKeyContent ? 1 : 0;
  return (
    <TabsContent value={String(OperatingSystem.DOCKER)}>
      <TabsContentPadding>
        <p className={"font-medium flex gap-3 items-center text-base"}>
          <IconBrandUbuntu size={16} />
          Install on Ubuntu
        </p>
        <Steps>
          <Steps.Step step={1}>
            <p>Install Docker</p>
            <div className={"flex gap-4 mt-1"}>
              <Link
                href={"https://docs.docker.com/engine/install/"}
                passHref
                target={"_blank"}
              >
                <Button variant={"primary"}>
                  <ExternalLinkIcon size={14} />
                  Official Docker Installation Guide
                </Button>
              </Link>
            </div>
          </Steps.Step>
          {setupKeyContent && (
            <Steps.Step step={2}>{setupKeyContent}</Steps.Step>
          )}
          <Steps.Step step={2 + offset}>
            <p>
              Run NetBirdy container
              {showSetupKeyInfo && <RoutingPeerSetupKeyInfo />}
            </p>
            <Code>
              <Code.Line>docker run -d --restart=unless-stopped \</Code.Line>
              <Code.Line> --cap-add=NET_ADMIN \</Code.Line>
              <Code.Line>
                {" "}
                -e NB_SETUP_KEY=
                <span className={"text-netbirdy"}>
                  {setupKey ?? setupKeyPlaceholder ?? "SETUP_KEY"}
                </span>{" "}
                \
              </Code.Line>

              {hostname && (
                <Code.Line>
                  {" "}
                  -e NB_HOSTNAME=
                  <span className={"text-netbirdy"}>{`'${hostname}'`}</span> \
                </Code.Line>
              )}

              <Code.Line> -v netbirdy-client:/var/lib/netbird \</Code.Line>
              {GRPC_API_ORIGIN && (
                <Code.Line>
                  {" "}
                  -e NB_MANAGEMENT_URL=
                  <span className={"text-netbirdy"}>{GRPC_API_ORIGIN}</span> \
                </Code.Line>
              )}
              <Code.Line> ghcr.io/quad4-software/netbirdy:latest</Code.Line>
            </Code>
          </Steps.Step>
          <Steps.Step step={3 + offset} line={false}>
            <p>Read our documentation</p>
            <InlineLink
              href={docsLink("https://docs.netbird.io/how-to/installation/docker")}
              passHref={true}
              target={"_blank"}
            >
              Running NetBirdy in Docker
            </InlineLink>
          </Steps.Step>
        </Steps>
      </TabsContentPadding>
    </TabsContent>
  );
}
