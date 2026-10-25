---
title: Connect Your First Peer
description: Create a setup key and join a machine to the overlay network.
order: 30
---

Peers join the network with a reusable setup key or interactive SSO sign-in.

## Create a setup key

1. Open the dashboard and go to **Setup Keys**.
2. Create a reusable key and copy it.

## Join a peer

```bash
netbirdy up \
  --management-url http://localhost:8081 \
  --setup-key <paste-your-key>
netbirdy status
```

When `Management` and `Signal` show `Connected`, the peer has an overlay IP
under `NetBirdy IP`. Other peers in the same account can ping it.

> **Warning** A setup key is a credential. Rotate or revoke it from the
> dashboard when it is no longer needed.
