import { Meta, StoryObj } from "@storybook/react";
import CommandPalette from "./CommandPalette";

const meta: Meta<typeof CommandPalette> = {
  component: CommandPalette,
  title: "Components/CommandPalette",
};
export default meta;
type Story = StoryObj<typeof CommandPalette>;

export const WithQuery: Story = {
  play: async () => {
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "k", ctrlKey: true, shiftKey: true }),
    );
  },
};
