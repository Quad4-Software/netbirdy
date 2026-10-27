import { iconProperties, IconProps } from "@/assets/icons/IconProperties";

export default function Quad4Icon(props: Readonly<IconProps>) {
  return (
    <svg
      viewBox="0 0 512 512"
      xmlns="http://www.w3.org/2000/svg"
      {...iconProperties(props)}
    >
      <circle cx="256" cy="256" r="256" fill="#fff" />
      <g fill="#191919">
        <rect x="116" y="116" width="120" height="120" rx="20" />
        <rect x="276" y="116" width="120" height="120" rx="20" />
        <rect x="116" y="276" width="120" height="120" rx="20" />
      </g>
      <rect
        x="276"
        y="276"
        width="120"
        height="120"
        rx="20"
        fill="none"
        stroke="#191919"
        strokeWidth="24"
      />
    </svg>
  );
}
