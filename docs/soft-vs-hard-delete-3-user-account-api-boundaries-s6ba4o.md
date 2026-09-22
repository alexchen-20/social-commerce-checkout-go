# Soft vs Hard Delete: 3 User Account API Boundaries for GDPR

For a small healthtech SaaS, the operational constraint is that deletion can race with login callbacks, queues, and retries. **Short answer: use hard deletion for personal account data that has no valid retention need, keep only a narrowly justified non-personal tombstone where duplicate-event control requires one, and treat soft deletion as a temporary workflow state rather than the finished GDPR outcome.** Disable sign-in first, then erase each owned data set through an idempotent job whose completion can be proved.

I have been paged for missed jobs and duplicate deliveries. That experience changes the deletion design: an HTTP `204` cannot be the proof that every downstream copy vanished. In the bounded scenario here, a healthtech team is moving off a managed identity provider while wiring Google and GitHub sign-in. The invariant is simple: an old callback, replayed queue message, or provider retry must never recreate an account after deletion begins.

## Should a user account API soft delete or hard delete data?

A `deleted_at` column is useful while work is in flight. It immediately blocks normal reads and gives workers a stable state to resume from. But rows, profile fields, social-login links, session records, exports, and queued payloads still exist. Calling that final erasure confuses application visibility with data removal.

Hard deletion has the opposite operational weakness: deleting the primary row first can destroy the key needed to locate dependent records. It may also leave an active external login path long enough for a callback to race the cleanup. The practical answer is a staged state machine, not a single SQL verb.

Order matters.

Use three boundaries:

1. Access boundary: reject new sessions, revoke existing sessions, and reject social-login callbacks for an account in deletion state.
2. Data boundary: enumerate stores by ownership and erase or transform the records assigned to the account.
3. Evidence boundary: retain only the minimum completion marker required by an established retention decision, without email, provider subject, tokens, or health data.

This split also prevents a migration mistake. Google and GitHub identities should be detachable login methods, not the account's durable identity. During a provider migration, map both old and new login records to an internal opaque account ID; after deletion starts, no successful provider response may bypass that account state.

## Make deletion retryable before making it fast

The dangerous implementation is a request handler that performs five deletes, times out after the fourth, and cannot say which operation committed. A small SaaS does not need elaborate orchestration, but it does need durable progress and monotonic states: `active`, `deleting`, then `deleted`. Never transition backward from `deleting` because a late authentication callback arrives.

The following Go shape keeps the irreversible work behind named, idempotent steps. Each eraser must treat an already-absent record as success. The job records a step only after that eraser returns successfully, so a worker crash causes a repeat rather than a skipped deletion.

```go
package deletion

import (
	"context"
	"fmt"
)

type Store interface {
	BeginDeletion(ctx context.Context, accountID string) error
	StepDone(ctx context.Context, accountID, step string) (bool, error)
	MarkStepDone(ctx context.Context, accountID, step string) error
	FinishDeletion(ctx context.Context, accountID string) error
}

type Eraser func(context.Context, string) error

type Worker struct {
	Store   Store
	Erasers map[string]Eraser
}

func (w Worker) Delete(ctx context.Context, accountID string) error {
	if err := w.Store.BeginDeletion(ctx, accountID); err != nil {
		return fmt.Errorf("block access: %w", err)
	}

	steps := []string{"sessions", "social_links", "account_data"}
	for _, step := range steps {
		done, err := w.Store.StepDone(ctx, accountID, step)
		if err != nil {
			return fmt.Errorf("read step %s: %w", step, err)
		}
		if done {
			continue
		}
		if err := w.Erasers[step](ctx, accountID); err != nil {
			return fmt.Errorf("erase %s: %w", step, err)
		}
		if err := w.Store.MarkStepDone(ctx, accountID, step); err != nil {
			return fmt.Errorf("record step %s: %w", step, err)
		}
	}

	return w.Store.FinishDeletion(ctx, accountID)
}
```

The three step names are deliberately coarse. In a real data inventory, one step may fan out to object storage, analytics, support tooling, and backups under their documented lifecycle. Do not pass email addresses through this job when an opaque account ID will do. Logs should record the job ID, step, attempt, latency, and outcome, not the deleted fields.

Retries are normal.

They need backoff and a dead-letter path, but a dead-letter queue is not completion. Alert on deletion age and on any account stuck in `deleting`; count repeated attempts separately from completed accounts. This is where the postmortem question becomes useful: could the team prove which boundary failed without opening a database row containing the person's data?

## Compare outcomes, not database operations

Soft and hard delete are implementation terms. The decision should be made per data class and purpose.

| Data class | During deletion | Finished state | Main failure to test |
|---|---|---|---|
| Sessions and refresh credentials | Revoke immediately | Removed or unusable | An old session still authorizes |
| Social-login link | Block callback resolution | Provider subject and tokens removed | A callback recreates the account |
| Profile and health-related account data | Hide from normal reads, then erase | Removed unless a separately validated retention rule applies | A downstream copy is missed |
| Operational completion marker | Write only after all steps succeed | Minimal, purpose-limited tombstone if justified | The marker can identify the person |

This framing exposes the trade-off. Soft deletion buys recovery and simpler rollback, but it prolongs possession of the data and expands the set of systems that must enforce invisibility. Hard deletion narrows that exposure, but recovery becomes a restore or a new registration, and referential dependencies must be mapped before execution.

Do not let database foreign keys make the policy accidentally. For records that must remain for a separately established reason, detach account identifiers where possible and document the responsible owner, expiry condition, and access controls. Legal interpretation and retention obligations need qualified review; the engineering system should encode the resulting decision rather than invent it.

## Test the races that happy-path checks miss

Start with the awkward sequence: deletion enters `deleting`, a GitHub callback arrives, the worker crashes after removing sessions, and the queue delivers the same job twice. The correct result is still one deleted account, no new session, and a resumed cleanup.

Then test a Google identity that was linked to the same internal account, an expired callback state, an already-missing social link, and a store that times out after committing. These are deterministic integration tests, not production experiments. Keep a synthetic account fixture with no real health data and assert both absence and denied access after every retry pattern.

Backups are a separate lifecycle. The live deletion job should not pretend it rewrote immutable backup sets. The team needs a documented restoration procedure that prevents deleted accounts from silently returning to service, plus a retention decision for those sets. Without that restore test, the runbook has a hole.

## When should this advice change?

Do not use this pattern as a substitute for a validated retention schedule or legal analysis. It also does not fit data that was never associated with an identifiable account, nor does it require a tombstone when retry safety can be achieved without retaining one.

For the small SaaS in this scenario, the release gate is concrete: callbacks are blocked at `deleting`, all erasers tolerate repetition, the managed-provider migration preserves the internal account ID only until cleanup finishes, and monitoring distinguishes accepted requests from completed erasure. **The finish line is a provable data outcome, not a hidden row.**

## Sources

- https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html
