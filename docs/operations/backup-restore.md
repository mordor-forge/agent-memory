# Backup, Export, and Restore

## Purpose
This document explains the current backup and recovery story for `agent-memory`.

There are **two different categories** of data recovery to keep separate:

1. **Database-level backup/restore**
   - protects the full CockroachDB cluster state
2. **Tenant-level logical export/delete**
   - lets operators export or remove one tenant’s data from the service

These are complementary, not interchangeable.

## Current Recovery Layers

### Layer 1: CockroachDB backups
Use CockroachDB-native backups for:

- full-cluster recovery
- accidental schema damage
- host failure recovery
- disaster recovery planning

This is the primary backup strategy for the service as a whole.

### Layer 2: Tenant export
Use `ExportTenant` for:

- tenant-scoped data extraction
- portability
- pre-delete safety snapshots
- operator inspection

This is **not** a replacement for CockroachDB backups.

## Current Capability Summary

### Supported today
- export tenant-scoped service data
- delete tenant-scoped service data
- restore the full database using CockroachDB’s own mechanisms

### Not supported today
- in-place “import tenant export” workflow
- point-and-click tenant restore inside the service
- partial replay from a tenant export back into a live cluster

So the current honest story is:

- use CockroachDB backups for full recovery
- use tenant exports for scoped inspection or offline portability

## Tenant Export
### HTTP

```bash
curl -H 'Authorization: Bearer <token>' \
  http://127.0.0.1:8080/v1/tenants/<tenant-uuid>/export
```

### MCP

Use:

- `export_tenant`

### What the export contains
The export currently includes:

- tenant
- agents
- threads
- episodes
- memories
- projection checkpoints
- consolidation runs
- worker leases relevant to the tenant

### When to use it
Use export:

- before deleting a tenant
- before doing risky experiments on tenant data
- when you need a portable snapshot for inspection or migration planning

## Tenant Delete
### HTTP

```bash
curl -X DELETE \
  -H 'Authorization: Bearer <token>' \
  http://127.0.0.1:8080/v1/tenants/<tenant-uuid>
```

### MCP

Use:

- `delete_tenant`

### Important warnings
- This operation is destructive.
- It is intended for operator-controlled use.
- It currently performs explicit ordered cleanup in the service store layer.
- There is no built-in undo.

Recommended operator sequence:

1. export tenant
2. verify export stored safely
3. delete tenant
4. verify tenant is no longer queryable

## CockroachDB Backup Guidance
The project does not currently orchestrate CockroachDB backups for you.

You should use CockroachDB-native backup tooling and procedures appropriate to your deployment.

At minimum, decide:

- where backup artifacts live
- how often they run
- how you validate restore
- who owns the credentials and retention policy

For this service, backups should be thought of as:

- the source of truth for disaster recovery
- independent of the app process lifecycle

## Recommended Operator Policy

### For local development
- exports are usually enough
- full backups are optional unless you care about preserving a particular state

### For any serious environment
- schedule real CockroachDB backups
- test restore periodically
- use tenant exports only as a complementary scoped mechanism

## Restore Scenarios

### Scenario A: Accidental tenant deletion
Current best path:

1. restore the CockroachDB backup to a safe recovery environment
2. inspect the tenant data there
3. export the tenant from the restored environment
4. manually reintroduce data only after a future import workflow exists

Today, there is no built-in “re-import this tenant export” command.

### Scenario B: Full database loss
Current best path:

1. restore CockroachDB from its native backup
2. restart `memoryd`
3. run `memoryd doctor`
4. validate recalls and worker state

### Scenario C: Operator wants a snapshot before risky changes
Current best path:

1. export tenant
2. save export artifact
3. proceed with change

## Secret Hygiene
### Current expectations
- Do not commit real `MEMORY_OPENAI_API_KEY` values.
- Do not commit real `MEMORY_HTTP_AUTH_PRINCIPALS_JSON` values.
- Prefer `.envrc.local` or another local secret source for development.
- Rotate secrets by updating the source of truth and restarting the affected processes.

### What still needs to be built later
- formal secret rotation runbooks
- supported secret manager integrations
- import/restore workflows for exported tenant data

## Practical Checklist

Before calling your setup “recoverable”, make sure you can answer:

- Where are CockroachDB backups stored?
- How often are they taken?
- Have you tested a restore?
- Can you export a tenant before deleting it?
- Who can call delete/export APIs?
- Where do auth and provider secrets live?

If any answer is “I’m not sure”, you do not yet have a complete backup and restore story.
