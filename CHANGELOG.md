## v0.3.1-fake1 (June 29, 2026)


BUG FIXES:

* This is a test change that will never be added to the real CHANGELOG

## v0.3.0 (June 22, 2026)


ENHANCEMENTS:

* The `harness install` command supports shell autocompletion for supported coding agents and support for the Amp coding agent has been added.

* Adds debug logging for token configuration sources.

* Hostnames are normalized before storage within profiles.

* The `api` command now accepts arbitrary URLs, such as Archivist, but does not send tokens to any host except the configured API host.


BUG FIXES:

* Profile configuration files are now created with read/write permissions for owner only.

* Hostname telemetry is anonymized when configured with a Terraform Enterprise host.

## v0.2.0 (June 12, 2026)

NOTES:

* tfctl is an agent-first, human-friendly CLI for accessing HCP Terraform and Terraform Enterprise. Future changes will be documented here. For now, see README.md for installation, usage, and a command reference.