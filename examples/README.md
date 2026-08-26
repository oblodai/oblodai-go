# Examples

Each directory is a runnable program. Point them at the sandbox first: a sandbox API key
(public id `test_oblodai_…`, secret `oblodai_test_…`) drives a chainless copy of the gateway, with
fake balance and simulated deposits.

```bash
export OBLODAI_PUBLIC_ID=test_oblodai_…
export OBLODAI_SECRET=oblodai_test_…
go run ./examples/accept-payment
```

| Example            | What it shows                                                                 |
| ------------------ | ----------------------------------------------------------------------------- |
| `accept-payment`   | Create an invoice, show the payer where to send funds, poll until it is final. |
| `payout`           | Validate a payout for free, then send it with your own idempotency key.        |
| `webhook-receiver` | An HTTP receiver that verifies deliveries and deduplicates them.               |

Against a local core add `OBLODAI_BASE_URL=http://127.0.0.1:8095`.
