# Policy Workbench

A small payment-policy application for a product engineering interview. All names, recipients, balances and transfers are synthetic.

## Run

### Docker

Requires Docker with Compose. No local Go installation is needed.

```sh
docker compose up --build
```

Open **http://127.0.0.1:8094**. The same container serves both the UI and API. State is saved in the named `workbench-data` volume and survives container recreation.

If port 8094 is already in use:

```sh
PORT=8095 docker compose up --build
```

Open **http://127.0.0.1:8095** instead. Stop the stack with `docker compose down`; its data volume remains. **Reset demo** in the UI restores the initial synthetic data in both profiles.

### Go

Requires Go 1.22 or newer. No npm, Docker, credentials or external service is needed.

```sh
go run .
```

Open **http://127.0.0.1:8094**. The first run creates `.data/state-v4.json`. Changes survive a server restart. **Reset demo** restores both profiles, policies, history and the simulation clock.

```sh
go run . -addr 127.0.0.1:8095 -data /tmp/my-policy-workbench.json
```

Use a distinct state file for each process. Format version 4 stores the amount condition as a nested `amount_policy`. Version 3 and earlier files are refused rather than reinterpreting their amounts as fiat. The previous default state file is not overwritten. Docker and local Go use separate state locations by default.

Storage supports one process per file. Atomic file replacement protects against partial JSON writes; this demo does not promise database-grade durability across a power loss.

## Task

Read [TASK.md](TASK.md), also available through **Interview brief** in the application. It describes the current product, two examples using the seeded policies, and the customer request. Clarify the new feature with the interviewer. The period feature is not implemented in this starter.

## Wallets and balances

**Wallets** is the starting view. Each wallet has one currency, USD or EUR, and a fixed synthetic opening balance. Wallets, groups, members and recipients are fixtures, not editable directories.

- **Balance** is the opening balance minus CONFIRMED outgoing transfers.
- **Reserved** is the sum of PENDING_SIGNATURE and SENT outgoing transfers.
- **Available** is balance minus reserved.
- FAILED, CANCELLED, REJECTED, REJECTED_BY_POLICY and NO_POLICY_FOUND do not consume funds.

An ALLOW decision must fit the available wallet balance. Otherwise creation fails without adding a transfer. The check and insertion share one serialized update. A policy rejection is still recorded without reserving funds. There are no deposits, fees, exchange rates, conversion or separate reservation records. Funds are not transferred to another internal wallet.

## Current policy behaviour

Policies belong to a profile and run top to bottom. The first rule whose conditions all match decides ALLOW or REJECT. New rules start at the bottom. Use the arrows to change the order. Create, delete and reorder are supported; editing is not.

| Field            | Meaning                                                                                             |
| ---------------- | --------------------------------------------------------------------------------------------------- |
| Initiator        | Any profile member or one selected member                                                           |
| Sender           | Any wallet, one wallet or wallet-group membership                                                   |
| Receiver         | Any recipient, one recipient or recipient-group membership                                          |
| Currency         | USD or EUR, matched against the source wallet's currency                                            |
| Amount condition | Optional `amount_policy`. Absent means any amount. `count`: each payment `amount <= limit`. `interval`: the source wallet's rolling total `used + amount <= limit` over `24h` or `30d`. Zero is a bound. |
| Decision         | ALLOW or REJECT                                                                                     |

An interval total belongs to the paying wallet, not to the policy or wallet group. It sums that wallet's PENDING_SIGNATURE, SENT and CONFIRMED transfers created in `(now − interval, now]` on the simulation clock, whichever policy allowed them. The wallet view shows both totals.

```json
"amount_policy": {"amount": "10000", "amount_type": "interval", "interval": "24h"}
```

Amount and currency are matching conditions. A non-match continues to the next rule. Exceeding a maximum does not itself reject a transfer. A matching REJECT records REJECTED_BY_POLICY. No match records NO_POLICY_FOUND.

The selected policy snapshot and condition trace are recorded at creation. Existing transfers retain them after policies change. No second policy selection happens at signing. There are no approval counts or approver actions.

## Transfer simulation

Choose an initiator, source wallet, recipient and amount. The UI shows the source wallet's currency and available funds. The server derives the currency from that wallet; clients cannot override it.

```text
Matched ALLOW -> PENDING_SIGNATURE -> SENT -> CONFIRMED
PENDING_SIGNATURE -> REJECTED or CANCELLED
PENDING_SIGNATURE / SENT -> FAILED
Matched REJECT -> REJECTED_BY_POLICY
No match -> NO_POLICY_FOUND
```

Terminal transfers cannot change state. **Simulate failure** is definitive, including after sending. There are no real signatures, network broadcasts or uncertain execution outcomes.

The simulation clock is fixed until **+1 hour** or **+1 day** advances it for both profiles. Creation and updates use this server-owned time. A restart preserves it. This clock is provided for the candidate's period feature.

## Code map

See [ARCHITECTURE.md](ARCHITECTURE.md) for the request flow.

```text
main.go                       Wiring, embedded assets, server lifecycle
internal/domain/              Policies, matching, exact money, transfer states, balances
internal/application/         Operations and workspace state
internal/storage/jsonfile/    Serialized copy/save/publish updates
internal/httpapi/             JSON routes and static assets
internal/demo/                Synthetic catalog and initial workspace
web/app.js                    Navigation, profiles and simulation controls
web/wallets.js                Wallet list and balances
web/policies.js               Create, order and delete rules
web/transfers.js              Create payments, inspect decisions and apply actions
web/catalog.js                Read-only members, groups and recipients
web/api.js                    HTTP client
web/presentation.js           Display formatting and summaries
web/ui.js                     DOM helpers and dialogs
```

Amounts are decimal strings, with at most two meaningful decimal places. The parser accepts up to 78 digits and arithmetic uses integer coefficients, never floating point. JSON rejects unknown fields, including client-supplied currency and timestamps on transfer creation.

## API

Mutations with a body use `Content-Type: application/json`.

| Method | Path                                     | Input                                                                                               |
| ------ | ---------------------------------------- | --------------------------------------------------------------------------------------------------- |
| GET    | `/api/state?profile_id=treasury`         | Profile, wallet balances, catalog, policies, history and clock                                      |
| POST   | `/api/policies`                          | Policy fields; server supplies ID and creation time                                                 |
| PUT    | `/api/policies/order`                    | `{"profile_id":"treasury","ids":["..."]}`, every rule exactly once                                  |
| DELETE | `/api/policies/{id}?profile_id=treasury` | Delete one rule                                                                                     |
| POST   | `/api/transfers`                         | `profile_id`, `initiator_id`, `wallet_id`, `recipient_id`, decimal-string `amount`, optional `note` |
| POST   | `/api/transfers/{id}/actions`            | `profile_id`, `action`                                                                              |
| POST   | `/api/clock`                             | `{"hours":1}`, positive integer up to 8760                                                          |
| POST   | `/api/reset`                             | `{}`                                                                                                |

```sh
curl http://127.0.0.1:8094/api/transfers \
  -H 'Content-Type: application/json' \
  -d '{"profile_id":"treasury","initiator_id":"alex","wallet_id":"operations","recipient_id":"supplier","amount":"100"}'
```

## Check

```sh
go test -race ./...
go vet ./...
```

Tests cover matching, exact money, currency and profile separation, policy order, frozen decisions, transfer states, wallet balances, concurrent creation/signing, restart persistence, HTTP workflows and failed saves.
