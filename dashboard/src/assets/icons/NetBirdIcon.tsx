import * as React from "react";
import { memo } from "react";
import { NetBirdyMark } from "@/components/NetBirdLogo";

type Props = {
  size?: number;
  className?: string;
};

function NetBirdIcon({ size = 16, className }: Props) {
  return (
    <span className={className} role="img" aria-label="NetBirdy">
      <NetBirdyMark size={size} />
    </span>
  );
}

export default memo(NetBirdIcon);
