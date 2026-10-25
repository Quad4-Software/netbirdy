import { cn } from "@utils/helpers";
import { cva, VariantProps } from "class-variance-authority";
import * as React from "react";

export type BadgeVariants = VariantProps<typeof variants>;

interface Props extends React.HTMLAttributes<HTMLDivElement>, BadgeVariants {
  children: React.ReactNode;
  className?: string;
  useHover?: boolean;
  disabled?: boolean;
}

const variants = cva("", {
  variants: {
    variant: {
      blue: [
        "bg-sky-100 border-sky-500 text-sky-800 border border-transparent",
      ],
      blueDark: ["bg-sky-50 border-sky-300 text-sky-700 border dark:bg-sky-900 dark:border-sky-500 dark:text-white"],
      "blue-darker": ["bg-sky-50 border-sky-300 text-sky-700 border dark:bg-sky-900 dark:border-sky-500 dark:text-white"],
      red: ["bg-red-50 border-red-300 text-red-600 border dark:bg-red-950/40 dark:border-red-500 dark:text-red-500"],
      purple: ["bg-purple-50 border-purple-300 text-purple-600 border dark:bg-purple-950/50 dark:border-purple-500 dark:text-purple-500"],
      yellow: ["bg-yellow-50 border-yellow-300 text-yellow-700 border dark:bg-yellow-950 dark:border-yellow-500 dark:text-yellow-400"],
      gray: ["bg-nb-gray-930/60 border-nb-gray-800/40 text-nb-gray-300 border"],
      lightGray: ["bg-nb-gray-910  text-nb-gray-200 border border-nb-gray-900"],
      grayer: [
        "bg-nb-gray-900/40 border-nb-gray-800/40 text-nb-gray-300 border",
      ],
      "gray-ghost": [
        "bg-nb-gray-900 border-nb-gray-800 text-nb-gray-300 border border-nb-gray-800/50",
      ],
      green: ["bg-green-50 border-green-300 text-green-700 border dark:bg-green-950 dark:border-green-500 dark:text-green-400"],
      netbirdy: ["bg-[#ffe1e4] border-[#d92a41]/40 border text-[#d92a41] dark:bg-netbirdy-950 dark:border-netbirdy-500 dark:text-netbirdy-500"],
    },
    size: {
      default: "text-[0.75rem] py-1.5 px-3",
      xs: "text-[0.6rem] py-[0.3rem] px-2",
    },
    hover: {
      none: [],
      blue: ["hover:bg-sky-200"],
      purple: ["hover:bg-purple-100 dark:hover:bg-purple-950/40"],
      yellow: ["hover:bg-yellow-100 dark:hover:bg-yellow-950/40"],
      blueDark: ["hover:bg-sky-100 dark:hover:bg-sky-800"],
      "blue-darker": ["hover:bg-sky-100 dark:hover:bg-sky-800"],
      red: ["hover:bg-red-100 dark:hover:bg-red-950/40"],
      gray: ["hover:bg-nb-gray-900"],
      lightGray: ["hover:bg-nb-gray-900"],
      grayer: ["hover:bg-nb-gray-900"],
      "gray-ghost": ["hover:bg-nb-gray-800 cursor-pointer"],
      green: ["hover:bg-green-100 dark:hover:bg-green-950/50"],
      netbirdy: ["hover:bg-[#ffc9cf] dark:hover:bg-netbirdy-950/50"],
    },
  },
});

export default function Badge({
  children,
  className,
  variant = "blue",
  size = "default",
  useHover = false,
  disabled = false,
  ...props
}: Readonly<Props>) {
  return (
    <div
      className={cn(
        "relative z-10 cursor-inherit whitespace-nowrap rounded-md font-normal flex gap-1.5 items-center justify-center transition-all",
        variants({ variant, hover: useHover ? variant : "none", size }),
        disabled && "cursor-not-allowed opacity-50 select-none",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}
