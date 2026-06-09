# PayCore Architecture

PayCore owns the payment gateway lifecycle:

- merchant and payer records
- payment authorization
- payment capture
- payment holds
- settlement batches
- lifecycle event emission

PostgreSQL is the durable source of truth.

Redis is used for fast admission control and response caching, but correctness must not depend on Redis durability.

Kafka is used to publish payment lifecycle events to downstream systems such as LedgerFlow.

## High-Level Flow

```text
Client
  -> PayCore API
      -> Redis rate limiting
      -> Redis idempotency cache
      -> PostgreSQL durable state
      -> PostgreSQL outbox
          -> Kafka
              -> LedgerFlow
```

## Durability Rule

Redis may improve latency, but PostgreSQL remains authoritative for payment state, payer balances, idempotency records, settlement records, and outbox events.
