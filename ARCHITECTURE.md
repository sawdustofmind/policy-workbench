# Architecture

One Go process serves the API and native browser modules. No runtime framework, database or external service is required. Docker builds a static binary and runs it as UID 10001 in one container. Compose persists the JSON state in a named volume.

```text
Browser modules -> HTTP handlers -> Application service
                                      |          |
                                      v          v
                                Domain rules  Repository interface
                                                    ^
                                                    |
                                              JSON-file storage
```

- `domain` owns policy matching, exact decimal money, transfer states and balance calculation. It imports no HTTP or filesystem packages.
- `application` owns operations and the repository contract. There is one service and one repository, not an interface per entity.
- `storage/jsonfile` owns loading, cloning and atomic replacement. It publishes a successful saved copy under one mutex. A failed write leaves the previous state visible.
- `httpapi` owns routes, request decoding, status codes and embedded assets.
- `demo` owns fixed synthetic catalog data and initial history.

## Payment creation

1. Decode the transfer request. Currency and timestamps are server-owned.
2. Enter `Repository.Update`.
3. Validate the request and evaluate policies in order.
4. For ALLOW, derive available funds from the wallet's opening balance and current transfer statuses, then check the requested amount.
5. Create the transfer with the wallet's currency, policy snapshot and condition trace.
6. Save the proposed workspace state, then publish it.

There is no separately persisted balance or reservation ledger. `CalculateWalletBalance` derives balance, reserved funds and available funds from authoritative transfers. The same function serves the wallet view and creation checks. Changing a transfer status naturally changes those derived amounts.

## Money and stored meaning

Each wallet has one currency, USD or EUR. Policy currency is a matching condition, not a conversion target. Transfers use their wallet's currency. Amounts are exact nonnegative decimal strings with at most two meaningful fractional digits and up to 78 total digits on input. Internal arithmetic uses `big.Int` coefficients and scale.

Version-4 state contains policies, transfers, an ID sequence and simulation time. Version 3 and older files are refused rather than reinterpreted. The default filename is `state-v4.json`.

Interval amount limits read the source wallet's rolling total from an in-memory cache owned by `application.Service`. Keys are `spend:{profile_id}:{wallet_id}:{currency}:{interval}` (e.g. `spend:treasury:operations:USD:24h`); each holds the wallet's fund-holding transfers in creation order plus a running total, evicting expired entries on read. Every fund-holding transfer is added under both intervals. The cache is rebuilt from transfers at startup and on reset, and is updated only after a successful save while `Service.mu` serializes creation, actions and reset. `domain.WalletSpend` is the reference calculation the cache is tested against.

Policy snapshots, traces and nested condition arrays are copied at ownership boundaries. Policy order and balance checks use the same locked state as transfer insertion.

## Browser modules

`app.js` owns state, navigation and profile changes. `wallets.js`, `policies.js` and `transfers.js` own their views. They receive workspace access explicitly. `catalog.js` shows the fixed fixture directory. Shared formatting, DOM helpers and transport live in `presentation.js`, `ui.js` and `api.js`.

During a profile switch, old views remain hidden until the selected profile's state is loaded. Sequence checks ignore stale responses. An action response updates a detail dialog only if it still belongs to the selected transfer.

ES modules are served directly, without bundling or dependency installation. Root HTTP integration tests exercise the real composition, including embedded assets. All candidate tests run with `go test -race ./...`.
