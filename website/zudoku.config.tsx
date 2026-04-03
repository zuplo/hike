import type { ZudokuConfig } from "zudoku";
import { Button } from "zudoku/components";
import { HomePage } from "./src/HomePage";
import { GithubIcon } from "./src/GithubIcon";

const config: ZudokuConfig = {
  basePath: '/hike',
  canonicalUrlOrigin: "https://zuplo.github.io",
  site: {
    title: "hike",
    logo: {
      src: { light: "/logo-light.svg", dark: "/logo-dark.svg" },
      alt: "hike",
      width: "150px",
    },
  },
  metadata: {
    title: "hike — Multi-repo workspaces for AI workflows",
    description:
      "A fast CLI for managing multi-repo workspaces with git worktrees. Built for AI-powered development.",
    favicon: "/favicon.svg",
  },
  search: {
    type: "pagefind",
  },
  slots: {
    "head-navigation-end": (
      <Button variant="ghost" size="icon" asChild>
        <a href="https://github.com/zuplo/hike" target="_blank">
          <GithubIcon className="w-4 h-4" />
        </a>
      </Button>
    ),
  },
  navigation: [
    {
      label: "Home",
      type: "custom-page",
      path: "/",
      element: <HomePage />,
      layout: "none",
    },
    {
      type: "category",
      label: "Documentation",
      icon: "book",
      items: [
        {
          type: "category",
          label: "Getting Started",
          icon: "sparkles",
          collapsed: false,
          items: ["overview", "install", "configuration", "mcp"],
        },
        {
          type: "category",
          label: "Commands",
          icon: "terminal",
          collapsed: false,
          items: [
            "commands/create",
            "commands/init",
            "commands/sync",
            "commands/pull",
            "commands/push",
            "commands/status",
            "commands/delete",
            "commands/list",
            "commands/update",
          ],
        },
        {
          type: "separator",
        },
        {
          type: "link",
          label: "GitHub",
          to: "https://github.com/zuplo/hike",
          icon: "github",
        },
        {
          type: "link",
          label: "Releases",
          to: "https://github.com/zuplo/hike/releases",
          icon: "tag",
        },
      ],
    },
  ],
};

export default config;
