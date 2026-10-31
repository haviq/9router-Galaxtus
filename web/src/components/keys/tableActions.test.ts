// The API-key row menu collapses seven per-row controls into one trigger, and
// the batch bar adds actions over a selection. The rules the table depends on
// are stated once in tableActions.ts and pinned here.
import { describe, expect, it } from 'bun:test'
import {
  batchSummary,
  buildBulkPolicy,
  isBatchBusy,
  isSelected,
  isUnrestricted,
  needsAttention,
  runBatch,
  selectAllState,
  toggleAll,
  toggleOne,
  type NumericPolicyKey,
  type PolicyMode,
} from './tableActions'

describe('isBatchBusy', () => {
  it('is busy while a run is in flight', () => {
    expect(isBatchBusy(true, ['a'])).toBe(true)
  })

  it('is busy with nothing selected, because there is nothing to act on', () => {
    expect(isBatchBusy(false, [])).toBe(true)
  })

  it('is idle once a selection exists and no run is in flight', () => {
    expect(isBatchBusy(false, ['a'])).toBe(false)
  })
})

describe('runBatch', () => {
  it('counts every key that succeeded', async () => {
    const outcome = await runBatch(['a', 'b'], async () => {})
    expect(outcome).toEqual({ ok: 2, failed: 0, failedIds: [] })
  })

  // A batch that stopped at the first failure would leave the operator unable
  // to tell which keys had already been changed.
  it('keeps going after a failure and reports the failed keys', async () => {
    const attempted: string[] = []
    const outcome = await runBatch(['a', 'bad', 'c'], async (id) => {
      attempted.push(id)
      if (id === 'bad') throw new Error('nope')
    })
    expect(attempted).toEqual(['a', 'bad', 'c'])
    expect(outcome.ok).toBe(2)
    expect(outcome.failed).toBe(1)
    expect(outcome.failedIds).toEqual(['bad'])
  })

  it('labels failures with the caller-provided label', async () => {
    const outcome = await runBatch(
      ['a'],
      async () => {
        throw new Error('nope')
      },
      (id) => `key-${id}`,
    )
    expect(outcome.failedIds).toEqual(['key-a'])
  })
})

describe('batchSummary', () => {
  it('pluralises a fully successful multi-key run', () => {
    expect(batchSummary('enabled', { ok: 3, failed: 0, failedIds: [] })).toBe('3 keys enabled.')
  })

  it('uses the singular for one key', () => {
    expect(batchSummary('enabled', { ok: 1, failed: 0, failedIds: [] })).toBe('1 key enabled.')
  })

  it('names the keys that failed', () => {
    expect(batchSummary('enabled', { ok: 1, failed: 2, failedIds: ['b', 'c'] })).toBe(
      '1 key was enabled. 2 failed: b, c.',
    )
  })

  it('says so when nothing succeeded', () => {
    expect(batchSummary('deleted', { ok: 0, failed: 1, failedIds: ['a'] })).toBe(
      'No key was deleted. 1 failed: a.',
    )
  })
})

describe('selection', () => {
  it('reports a row selected only when its id is listed', () => {
    expect(isSelected(['a'], 'a')).toBe(true)
    expect(isSelected(['a'], 'b')).toBe(false)
  })

  it('treats an empty selection as none selected', () => {
    expect(selectAllState([], ['a', 'b'])).toBe('none')
  })

  it('reports none when there are no keys at all', () => {
    expect(selectAllState([], [])).toBe('none')
  })

  it('reports indeterminate when only some keys are selected', () => {
    expect(selectAllState(['a'], ['a', 'b'])).toBe('some')
  })

  it('reports all when every key is selected', () => {
    expect(selectAllState(['a', 'b'], ['a', 'b'])).toBe('all')
  })

  it('selects everything from a partial selection', () => {
    expect(toggleAll(['a'], ['a', 'b'])).toEqual(['a', 'b'])
  })

  it('clears the selection when everything is already selected', () => {
    expect(toggleAll(['a', 'b'], ['a', 'b'])).toEqual([])
  })

  it('adds a row without disturbing the others', () => {
    expect(toggleOne(['a'], 'b')).toEqual(['a', 'b'])
  })

  it('removes a row without disturbing the others', () => {
    expect(toggleOne(['a', 'b'], 'a')).toEqual(['b'])
  })
})
// A bulk edit is not "set these values on all": a `keep` column must be omitted
// so the server retains it, or the dialog silently rewrites a limit the operator
// never looked at.
describe('buildBulkPolicy', () => {
  const keep = (): Record<NumericPolicyKey, PolicyMode> => ({
    rateLimitRpm: 'keep',
    rateLimitTpm: 'keep',
    rateLimitConcurrency: 'keep',
    tokenQuota: 'keep',
    queueTimeoutMs: 'keep',
  })
  const vals = (over: Partial<Record<NumericPolicyKey, number>> = {}) => ({
    rateLimitRpm: 0,
    rateLimitTpm: 0,
    rateLimitConcurrency: 0,
    tokenQuota: 0,
    queueTimeoutMs: 0,
    ...over,
  })

  it('omits every kept column, so nothing is rewritten', () => {
    const policy = buildBulkPolicy({
      modes: keep(),
      values: vals(),
      expiresMode: 'keep',
      expiresAt: '',
    })
    expect(policy).toEqual({})
  })

  it('writes only the set columns', () => {
    const modes = { ...keep(), tokenQuota: 'set' as PolicyMode }
    const policy = buildBulkPolicy({
      modes,
      values: vals({ tokenQuota: 100000 }),
      expiresMode: 'keep',
      expiresAt: '',
    })
    expect(policy).toEqual({ tokenQuota: 100000 })
  })

  it('clears a column to the unrestricted default of 0', () => {
    const modes = { ...keep(), rateLimitRpm: 'clear' as PolicyMode }
    const policy = buildBulkPolicy({
      modes,
      values: vals(),
      expiresMode: 'keep',
      expiresAt: '',
    })
    expect(policy).toEqual({ rateLimitRpm: 0 })
  })

  it('floors a fractional or negative set value to a whole, non-negative one', () => {
    const modes = { ...keep(), rateLimitTpm: 'set' as PolicyMode }
    expect(
      buildBulkPolicy({ modes, values: vals({ rateLimitTpm: 120.9 }), expiresMode: 'keep', expiresAt: '' }),
    ).toEqual({ rateLimitTpm: 120 })
    expect(
      buildBulkPolicy({ modes, values: vals({ rateLimitTpm: -5 }), expiresMode: 'keep', expiresAt: '' }),
    ).toEqual({ rateLimitTpm: 0 })
  })

  it('sets an expiry from an RFC3339 string, and clears it to empty', () => {
    expect(
      buildBulkPolicy({
        modes: keep(),
        values: vals(),
        expiresMode: 'set',
        expiresAt: '2027-01-01T00:00:00.000Z',
      }),
    ).toEqual({ expiresAt: '2027-01-01T00:00:00.000Z' })
    expect(
      buildBulkPolicy({ modes: keep(), values: vals(), expiresMode: 'clear', expiresAt: '' }),
    ).toEqual({ expiresAt: '' })
  })
})

// The triage view is built on this: a key that is paused, expired or out of
// budget is one that is silently refusing traffic.
describe('needsAttention', () => {
  it('is false for a healthy active key', () => {
    expect(needsAttention({ isActive: 1 })).toBe(false)
  })

  it('is true for a paused key', () => {
    expect(needsAttention({ isActive: 0 })).toBe(true)
  })

  it('is true once the token budget is spent', () => {
    expect(needsAttention({ isActive: 1, tokenQuota: 100, usedTokens: 100 })).toBe(true)
    expect(needsAttention({ isActive: 1, tokenQuota: 100, usedTokens: 99 })).toBe(false)
  })

  it('is true for an expiry in the past, false for one in the future', () => {
    expect(needsAttention({ isActive: 1, expiresAt: '2000-01-01T00:00:00Z' })).toBe(true)
    expect(needsAttention({ isActive: 1, expiresAt: '2999-01-01T00:00:00Z' })).toBe(false)
  })
})

describe('isUnrestricted', () => {
  it('is true when no policy column is set', () => {
    expect(isUnrestricted({})).toBe(true)
  })

  it('is false when any single policy column is set', () => {
    expect(isUnrestricted({ rateLimitRpm: 60 })).toBe(false)
    expect(isUnrestricted({ tokenQuota: 1000 })).toBe(false)
    expect(isUnrestricted({ expiresAt: '2999-01-01T00:00:00Z' })).toBe(false)
  })
})
