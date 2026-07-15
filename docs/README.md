# Documentation

This directory contains current engineering references, operational runbooks,
and active design work. Start with the repository [README](../README.md) for
setup and product context.

## Current references

- [How AnimeEnigma works](HOW-IT-WORKS.md) — system overview and request flow.
- [aePlayer](aeplayer-reference.md) — current player architecture and provider model.
- [Scraper framework](scraper-framework.md) and [scraper health](scraper-health-reference.md) — provider execution and health authority.
- [Watch Together](watch-together-reference.md) — room and synchronization contract.
- [Stream proxy security](stream-security.md) — URL admission, provenance signing, and runtime poison detection.
- [API versioning](api-versioning.md), [environment variables](environment-variables.md), and [dependencies](dependencies.md) — shared development contracts.
- [Design-system lint rules](design-system-lint-rules.md) and [spotlight guidance](spotlight-card-guidelines.md) — frontend behavior and conventions.

## Operations

- [Git and deploy workflow](git-workflow.md)
- [Kubernetes target](k8s-deploy.md)
- [Secret rotation](runbooks/secret-rotation.md)
- [ClickHouse backup and restore](../docker/clickhouse/BACKUP-RESTORE.md)
- [Host automation](../infra/host/README.md)
- [Grafana alert rules](../docker/grafana/provisioning/alerting/) and [dashboards](../infra/grafana/dashboards/README.md)

## Plans, specs, and incident evidence

- [`plans/`](plans/) is reserved for short-lived cross-cutting plans.
- [`superpowers/plans/`](superpowers/plans/) is reserved for feature implementation plans.
- [`superpowers/specs/`](superpowers/specs/) contains design contracts that still explain current or pending behavior.
- [`issues/`](issues/) contains the live incident ledger, provider-recovery log, and audit evidence.
- [UX map](UX_MAP.md) is a dated April 2026 audit snapshot retained as evidence, not a current UI contract.
- [Documentation retirement log](documentation-history.md) records cleanup decisions; Git stores the retired source text.

## Lifecycle rules

1. Update a reference or runbook in the same change that alters its contract.
2. Treat dated implementation plans as temporary. Remove them after delivery,
   abandonment, or supersession; do not preserve them in an in-tree archive.
3. Keep a design spec only while it is active, referenced by code, or contains
   rationale not captured by a current reference.
4. Keep audit and experiment output only when it supports a live decision or
   repeatable operator procedure. Fold durable findings into the relevant
   reference before retiring the raw report.
5. Use Git history, not duplicate `legacy/` or `archive/` trees, for archaeology.
