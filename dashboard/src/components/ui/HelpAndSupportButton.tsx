"use client";

import Button from "@components/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@components/DropdownMenu";
import { cn } from "@utils/helpers";
import { isNetBirdCloud } from "@utils/netbirdy";
import {
  ArrowUpRightIcon,
  BookText,
  CircleQuestionMark,
  CodeXml,
  MailIcon,
  MessageSquareShare,
  MessagesSquareIcon,
  TriangleAlert,
} from "lucide-react";
import { useState } from "react";
import SlackIcon from "@/assets/icons/SlackIcon";
import { docsLink } from "@/modules/docs/links";

export default function HelpAndSupportButton() {
  const [dropdownOpen, setDropdownOpen] = useState(false);

  return (
    <DropdownMenu
      modal={false}
      open={dropdownOpen}
      onOpenChange={setDropdownOpen}
    >
      <DropdownMenuTrigger asChild={true}>
        <Button
          size={"xs"}
          variant={"default-outline"}
          className={cn(
            "!rounded-full h-[38px] w-[38px] !p-0",
            dropdownOpen && "text-white",
          )}
        >
          <CircleQuestionMark size={18} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-56" align="end" forceMount>
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col space-y-1 px-1">
            <div className="text-sm font-normal leading-none text-nb-gray-200 py-1">
              Help and Support
            </div>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          href="/docs"
          target="_blank"
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <BookText size={14} />
            Documentation
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          href={docsLink("https://docs.netbird.io/help/troubleshooting-client")}
          target="_blank"
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <TriangleAlert size={14} />
            Troubleshooting
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        {isNetBirdCloud() && (
          <DropdownMenuItem href="mailto:support@netbird.io?subject=Support Request">
            <div className={"flex gap-3 items-center"}>
              <MailIcon size={14} />
              support@netbird.io
            </div>
          </DropdownMenuItem>
        )}

        <DropdownMenuItem
          href="https://github.com/Sudo-Ivan/netbirdy"
          target="_blank"
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <CodeXml size={14} />
            Source code
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem
          href="https://forum.netbird.io/"
          target="_blank"
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <MessagesSquareIcon size={14} />
            NetBirdy Forum
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          href={docsLink("https://docs.netbird.io/slack-url")}
          target="_blank"
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <SlackIcon size={14} />
            NetBirdy Slack
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem
          href={"https://forms.gle/TeLw2zrXEdw6RcQ36"}
          target={"_blank"}
          rel="noopener noreferrer"
          asChild
        >
          <div className={"flex gap-3 items-center"}>
            <MessageSquareShare size={14} />
            Feedback
          </div>
          <DropdownMenuShortcut>
            <ArrowUpRightIcon size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
