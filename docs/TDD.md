# TDD — TempMail By JhopanStore

## Rule

New behavior and bug fixes need a test first where the behavior is testable without external Cloudflare infrastructure.

```text
RED:   add focused test and run it; verify expected failure
GREEN: minimum production code to pass
REFACTOR: simplify while tests remain green
```

Do not add test-only production switches, mock Cloudflare behavior blindly, or use a manual successful click as proof.

## Current test baseline

The codebase currently has little/no committed unit-test coverage. Before large refactors, add tests around touched behavior. At minimum always run:

```bash
cd receiver
go test ./...
go vet ./...
```

## Required test areas for new work

| Change | Minimum test |
|---|---|
| Email parser | raw RFC822 fixture → subject/body/attachments expected |
| OTP/link extraction | text fixtures → expected codes and links |
| Inbox API | create/list/wait flow using temporary SQLite DB |
| Auth | missing/bad/expired JWT rejected |
| Worker receive endpoint | missing API key rejected; valid key stores recipient/message |
| Cleanup | expired data deleted, fresh data retained |
| Multi-domain menu | state mutation test through isolated state file/script helper where practical |

## Integration verification

Some paths require live resources. Keep these as explicit runbook checks, not fake unit tests:

```text
Cloudflare Worker deployment
Cloudflare Tunnel hostname
Email Routing catch-all
Forward/copy destination verification
```

Use a test domain and test recipient. Never run a destructive routing change against an unrelated production domain.

## Test data rules

- Use `.invalid` domains for local tests.
- Use fake OTPs, e.g. `593817`.
- Do not put real tokens, mail bodies, credentials, or personal addresses into fixtures.
- Temp SQLite databases go under test temp directories and are removed after tests.

## Definition of done

```text
[ ] Target test failed first, where practical
[ ] gofmt run on edited Go files
[ ] go test ./... passes
[ ] go vet ./... passes
[ ] Script syntax checks pass for changed shell files
[ ] Live verification recorded if deployment/menu/cloud path changed
```

## First coverage backlog

1. Extract `extractCodesFromText` / `extractLinksFromText` into testable pure helpers.
2. Add temporary SQLite integration tests for `POST /api/email/receive`.
3. Add ownership authorization tests before exposing multi-user instances.
4. Add shell state helper tests or move menu state operations into a small testable Python/Go helper.
