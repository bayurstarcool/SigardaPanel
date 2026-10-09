# Security policy

## Reporting a vulnerability

Please report security problems privately, not in public issues:

Use **Security → Report a vulnerability** on this repository (GitHub private vulnerability reporting).

Include the affected version, what an attacker can do, and steps to reproduce.
You will get an acknowledgement within a few days. Please give us reasonable time to
ship a fix before disclosing details.

## Supported versions

Only the latest release receives security fixes. Update with `sudo bash install.sh update`.

## Release integrity

Releases are signed with minisign. The public key is pinned in `install.sh` and listed in the
README. If a release ever fails verification, do not install it and tell us.
