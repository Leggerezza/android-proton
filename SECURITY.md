# Security policy

## Threat model

- Local attacker with access to the device: tokens are encrypted with an
  Android Keystore key (AES-GCM, non-exportable); the password is never stored.
- Network attacker (DPI): the tunnel is TLS + WireGuard; the API uses strict
  certificate validation.
- Malicious mihomo profile: the app only exposes a local SOCKS5 port; it does
  not execute rules or read other apps' traffic.

## Data we store

| Data | Where | Lifetime |
|---|---|---|
| Proton uid / access / refresh token | Keystore-encrypted prefs | until logout / rotation |
| Device Ed25519 seed + certificate | Keystore-encrypted prefs | 7 days (cert) |
| Cached server list | app storage | 7 days |
| Preferences | DataStore | until cleared |
| Protocol logs (optional) | app storage | ring buffer, redacted |

We never store: the account password, destination domains, traffic contents.

## Reporting

Open a private security advisory on the repository. We aim to respond within 72 hours.
