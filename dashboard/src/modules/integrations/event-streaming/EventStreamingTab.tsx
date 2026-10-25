import Breadcrumbs from "@components/Breadcrumbs";
import InlineLink from "@components/InlineLink";
import Paragraph from "@components/Paragraph";
import * as Tabs from "@radix-ui/react-tabs";
import { ExternalLinkIcon, FileText } from "lucide-react";
import React from "react";
import IntegrationIcon from "@/assets/icons/IntegrationIcon";
import Firehose from "@/modules/integrations/event-streaming/amazon/firehose/Firehose";
import S3 from "@/modules/integrations/event-streaming/amazon/s3/S3";
import Datadog from "@/modules/integrations/event-streaming/datadog/Datadog";
import GenericHTTP from "@/modules/integrations/event-streaming/generic-http/GenericHTTP";
import { docsLink } from "@/modules/docs/links";

export default function EventStreamingTab() {
  return (
    <Tabs.Content value={"event-streaming"}>
      <div className={"p-default py-6"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/integrations"}
            label={"Integrations"}
            icon={<IntegrationIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/integrations"}
            label={"Event Streaming"}
            icon={<FileText size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Event Streaming</h1>
        <Paragraph>
          Event Streaming allows you to stream NetBirds audit & traffic events
          to different third-party services.
        </Paragraph>
        <Paragraph>
          <InlineLink
            href={docsLink("https://docs.netbird.io/how-to/activity-event-streaming")}
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
