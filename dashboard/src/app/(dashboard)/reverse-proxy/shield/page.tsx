"use client";

import Breadcrumbs from "@components/Breadcrumbs";
import Paragraph from "@components/Paragraph";
import SkeletonTable from "@components/skeletons/SkeletonTable";
import { RestrictedAccess } from "@components/ui/RestrictedAccess";
import React, { lazy, Suspense } from "react";
import ReverseProxyIcon from "@/assets/icons/ReverseProxyIcon";
import { usePermissions } from "@/contexts/PermissionsProvider";
import PageContainer from "@/layouts/PageContainer";

const ShieldSettings = lazy(
  () => import("@/modules/reverse-proxy/shield/ShieldSettings"),
);

export default function ReverseProxyShieldPage() {
  const { permission } = usePermissions();

  return (
    <PageContainer>
      <div className={"p-default py-6"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/reverse-proxy/services"}
            label={"Reverse Proxy"}
            icon={<ReverseProxyIcon size={16} />}
          />
          <Breadcrumbs.Item
            href={"/reverse-proxy/shield"}
            label={"Shield"}
            active={true}
          />
        </Breadcrumbs>
        <h1>Shield</h1>
        <Paragraph>
          The shield scores incoming sources per connected proxy and blocks
          scanners, abusive crawlers and probe traffic. Settings here apply to
          every connected proxy live; the status table below shows what each
          proxy is currently doing.
        </Paragraph>
      </div>
      <RestrictedAccess page={"Shield"} hasAccess={permission?.services?.read}>
        <Suspense fallback={<SkeletonTable />}>
          <ShieldSettings />
        </Suspense>
      </RestrictedAccess>
    </PageContainer>
  );
}
