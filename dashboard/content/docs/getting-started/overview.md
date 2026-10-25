---
title: Overview
description: What NetBirdy is and what you can do with it.
order: 10
---

NetBirdy is a self-hosted, WireGuard-based overlay network. Peers connect to
each other over encrypted tunnels, and a single combined server provides
management, signal, relay and an embedded identity provider.

## Components

- **Management** coordinates peers, policies, setup keys and accounts.
- **Signal** brokers ICE offers and answers between peers.
- **Relay** carries traffic when a direct path cannot be established.
- **Embedded IdP** handles sign-in for the dashboard and CLI.

## Where to start

1. [Run the local preview](./quickstart-podman.md) with Podman.
2. [Connect your first peer](./connect-your-first-peer.md).
3. Read about [offline mode](../guides/offline-mode.md) if peers must keep
   working when the management server is unreachable.
