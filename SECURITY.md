# Security Policy

ReelWarden is local-first and defaults to `127.0.0.1`. Do not expose it directly to untrusted networks.

## Reporting

Please report security issues privately to the maintainers before public disclosure.

## v0.1.1 Security Boundaries

- No real media file writes.
- No arbitrary shell API.
- AI is optional and disabled by default.
- AI must not receive provider content or secrets.
- Provider credentials must not be logged or returned in full by APIs.
