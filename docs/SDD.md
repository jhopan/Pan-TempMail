# SDD — Spec-Driven Development

Use this process for changes spanning API, Cloudflare, service deployment, menu behavior, or data schema.

## 1. Write a mini-spec before code

Add a section to the issue/PR/plan containing:

```text
Goal
Non-goals
Affected files/components
Input/output behavior
Security impact
Migration/deployment steps
Verification commands
Rollback
```

Example:

```text
Goal: add a Cloudflare profile CF B with a domain that reaches existing Armbian TempMail.
Non-goal: create a second tunnel or delete any DNS zone.
Affected: /etc/tempmail/cloudflare-state.json, Worker B, Email Routing B.
Verify: Worker B delivery to HTTP endpoint stores recipient under domain B.
Rollback: disable catch-all B; remove domain B from TempMail menu. Do not delete zone.
```

## 2. Choose smallest vertical slice

Avoid broad rewrite.

```text
Wrong: rewrite whole Cloudflare menu, then hope multiple accounts work.
Right: add profile → add one domain → deploy Worker for profile → route one domain → test → repeat.
```

## 3. Implementation constraints

- Reuse one Armbian TempMail instance and one tunnel unless a real isolation requirement exists.
- Cloudflare profiles remain separate: their account IDs/tokens/Workers/routing must not mix.
- Never persist an API token in Git; host state file is root-only.
- Do not turn optional email copies on during unrelated deployment work.
- Preserve the exact recipient email through worker/API/database.

## 4. Verification matrix

| Area changed | Verify |
|---|---|
| Go | `go test ./...`, `go vet ./...`, build target architecture |
| Worker | `node --check cloudflare-worker/src/index.js` |
| Shell menu | `bash -n deploy/armbian/tempmailctl` |
| Armbian deployment | service active, `/health`, UI HTTP 200 |
| Mail ingestion | local SMTP or HTTP Worker simulation, then actual domain test |
| Cloudflare routing | explicit test recipient per domain/profile |

## 5. Deployment record

After a change, record:

```text
Target host
Binary architecture
Service status
Configured domain(s)
Tunnel hostname
Worker profile(s)
Routing state
Tests passed
Known follow-ups
```

Store generic reusable facts in docs, not secrets or private tokens.

## 6. Rollback rules

- Binary issue: restore previous local-built binary, restart `tempmail`.
- Worker issue: redeploy previous Worker version for that profile.
- Routing issue: disable/update only that domain's catch-all; do not touch other profile domains.
- Menu state issue: back up `/etc/tempmail/cloudflare-state.json` before manual repair.
- Database issue: stop service, restore dated SQLite backup, repair ownership, restart.

## 7. Completion

A spec-driven task is complete only after docs and runbook are updated if an operator-visible behavior changed.

See `docs/PRD.md` for product requirements and `docs/RUNBOOK.md` for actual commands.
