import { cn } from "@utils/helpers";
import * as React from "react";

type Props = {
  size?: "default" | "large";
  mobile?: boolean;
};

const heights = {
  default: { desktop: 24, mobile: 30 },
  large: { desktop: 28, mobile: 40 },
};

export const NetBirdyMark = ({ size = 24 }: { size?: number }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 32 32"
    fill="none"
    aria-label="NetBirdy"
  >
    <path
      d="M8.5 22.5 14.2 9.4 23.5 22.5"
      stroke="var(--color-netbirdy)"
      strokeWidth="2.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <circle cx="8.5" cy="23.5" r="4.4" fill="var(--color-netbirdy)" />
    <circle cx="14.6" cy="8" r="4.4" fill="var(--color-netbirdy)" />
    <circle cx="23.5" cy="23.5" r="4.4" fill="var(--color-netbirdy)" />
  </svg>
);

export const NetBirdLogo = ({ size = "default", mobile = true }: Props) => {
  const h = heights[size];
  return (
    <>
      <svg
        height={h.desktop}
        viewBox="0 0 148 32"
        fill="none"
        role="img"
        aria-label="NetBirdy"
        className={cn(mobile && "hidden md:block")}
      >
        <path
          d="M8.5 22.5 14.2 9.4 23.5 22.5"
          stroke="var(--color-netbirdy)"
          strokeWidth="2.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle cx="8.5" cy="23.5" r="4.4" fill="var(--color-netbirdy)" />
        <circle cx="14.6" cy="8" r="4.4" fill="var(--color-netbirdy)" />
        <circle cx="23.5" cy="23.5" r="4.4" fill="var(--color-netbirdy)" />
        <text
          x="36"
          y="22.5"
          fontSize="19"
          fontWeight={600}
          letterSpacing="-0.5"
          fill="currentColor"
        >
          netbirdy
        </text>
      </svg>
      {mobile && (
        <span className="md:hidden ml-4 inline-flex">
          <NetBirdyMark size={h.mobile} />
        </span>
      )}
    </>
  );
};
