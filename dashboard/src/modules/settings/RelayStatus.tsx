import HelpText from "@components/HelpText";
import { Label } from "@components/Label";
import { SmallBadge } from "@components/ui/SmallBadge";
import useFetchApi from "@utils/api";
import React from "react";
import Skeleton from "react-loading-skeleton";
import { RelayStatus as RelayStatusType } from "@/interfaces/RelayStatus";

/**
 * Read-only status row for the embedded relay in the Networks settings tab.
 * The endpoint only exists while the embedded relay is running, so an error
 * means the relay is off rather than unreachable.
 */
export default function RelayStatus() {
  const { data, error, isLoading } = useFetchApi<RelayStatusType>(
    "/relay/status",
    false,
    true,
  );

  return (
    <div
      className={
        "flex flex-col gap-1 sm:flex-row w-full sm:gap-4 items-center"
      }
    >
      <div className={"min-w-[330px]"}>
        <Label>Relay</Label>
        <HelpText>
          Peers can route traffic through the relay embedded in this management
          service when a direct connection is not possible.
        </HelpText>
      </div>
      <div className={"w-full"}>
        <RelayStatusValue
          status={data}
          error={error}
          isLoading={isLoading}
        />
      </div>
    </div>
  );
}

function RelayStatusValue({
  status,
  error,
  isLoading,
}: Readonly<{
  status?: RelayStatusType;
  error?: unknown;
  isLoading: boolean;
}>) {
  if (isLoading) return <Skeleton width={200} />;
  if (error || !status) {
    return (
      <span className={"text-sm text-netbird-400 dark:text-nb-gray-400"}>
        Disabled
      </span>
    );
  }
  return (
    <div className={"flex items-center gap-2 flex-wrap"}>
      <span className={"text-sm font-mono"}>{status.address}</span>
      {status.transports.map((t) => (
        <SmallBadge key={t} variant={"blue"} text={t} />
      ))}
      <SmallBadge
        variant={"white"}
        text={`${status.connected_peers} connected`}
      />
    </div>
  );
}
