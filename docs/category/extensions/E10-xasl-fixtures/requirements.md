# E10 — XASL Fixture Production and Storage (Requirements)

*English · [한국어](requirements.ko.md)*

**Source:** derived while E9 was being redefined (2026-09-04) · `E9-storage-fuzzing/requirements.md` §4a·§4b
**Status:** incubating (new — a prerequisite of E9 Tier 2)
**Axis mapping:** supporting equipment for axes 1 and 3 — it has no oracle of its own. It makes
*an asset other entries use*
**Ladder position:** none. The ladder is the order in which the fuzzing entries are started, and
this entry is the equipment underneath them
**Companion docs (to follow):** `design.md`, `io-contract.md`, `test-corpus.md` (all STUB)

---

## 1. The problem this entry solves

**The CUBRID server does not compile SQL.** That is a confirmed fact — `libcubrid.so`
(SERVER_MODE) has no `parser_main`, no `pt_compile`, no `do_prepare_select`, no
`xts_map_xasl_to_stream`. Compilation and XASL serialization are **the client's job**, and the
server takes the serialized stream and turns it back with `stx_map_stream_to_xasl ()` to run it.

So **half of it — consumption — is in the engine, and the other half — production and storage — is
nowhere.** There is no code in the tree that writes an XASL stream to a file or reads one back.

This entry is that empty half — the equipment that **makes XASL fixtures from queries, keeps them,
identifies their version, and hands them out in a replayable form.**

## 2. The consumers

There is more than one, and E9 is only one of them. That is why this is an independent entry rather
than a section inside E9.

| Consumer | What for |
|---|---|
| **§6a-E9** (storage concurrency) | to replay operations instead of synthesizing them. A prerequisite of Tier 2 |
| §6a-E3 (SQLancer) | comparing whether the plan for the same query changed between builds (potential) |
| plan stability regression | whether the plan wobbles for the same query and the same statistics (potential) |

The relation to E3 is **producer–consumer**. E3 makes the queries, this entry sets those queries
into fixtures, and E9 replays what has been set. They do not overlap.

## 3. The fixture schema

The canonical form is what the client puts on the execution request — the unpack order of
`sqmgr_execute_query ()`:

```c
OR_UNPACK_XASL_ID  (ptr, &xasl_id);
ptr = or_unpack_int (ptr, &dbval_cnt);
ptr = or_unpack_int (ptr, &data_size);
ptr = or_unpack_int (ptr, &query_flag);
OR_UNPACK_CACHE_TIME (ptr, &clt_cache_time);
ptr = or_unpack_int (ptr, &query_timeout);
```

| Item | Required | Why |
|---|---|---|
| `sql_user_text` | yes | the unit a person reads to reproduce and triage. **`sql_hash_text` will not do** — it is a rewritten hash key (`xasl_cache.h` `EXECUTION_INFO`) |
| **the engine build identifier** | yes | §4. The engine does not tell you |
| the XASL stream `buffer` + `buffer_size` | yes | what is executed |
| the host variable bundle + `data_size` | yes | it arrives on the request |
| `query_flag` | yes | it changes the meaning of the execution |
| `query_timeout` | yes | it arrives on the request |
| `dbval_cnt` | optional | already in the stream header (`or_unpack_int (p, &xasl->dbval_cnt)`). For cross-checking |
| `clt_cache_time` | no | for negotiating the client cache. Fixed to null on replay |

## 4. Why identifying the version is mandatory

What `stx_map_stream_to_xasl ()` checks is **a non-null pointer and `xasl_stream_size > 0`, and
nothing else.** It goes straight on to read the header size with `or_unpack_int`, compute offsets
and restore the tree.

**There is no format check and no version check.** A stream made by a different build is not
rejected; it is deserialized with offsets whose meaning has changed — not a failure but a *silent
malfunction*. In equipment whose whole premise is replay this is fatal, so **recording the build
identifier in the fixture and refusing on a mismatch is this entry's core requirement**. It is
blocked at the fixture layer, not by changing the engine.

## 5. User requirements (incubating estimate)

1. **Production** — make a fixture from a query (its text). It has to go through the client-side
   compilation path
2. **Storage** — a file format. A person has to be able to read the list (the `sql_user_text` of
   §3), and the stream is binary
3. **Version refusal** — do not silently pass a build mismatch through (§4)
4. **Regeneration** — when the engine changes, pull them again from the same list of queries. With
   one run of a script
5. **NG1** — fixtures live outside the testcases repository

## 6. Non-functional requirements

| Item | Agenda | Note |
|------|------|------|
| Cost of adoption | medium | one client-side tool and one format |
| Engine change | **none** | serialization is already in the client. There is nothing new to put into the engine |
| Cost of upkeep | **tied to the XASL format** | if the engine's internal structure changes, the fixtures rot. The regeneration script is the price of that |
| Prerequisite | none | it can be started independently of E5 and E9 |

## 7. Conditions for entering incubating

1. **The production path chosen** — which client the fixtures are pulled with: csql / CCI / JDBC
2. **The format settled** — the file representation of the §3 schema
3. **The build identifier defined** — what is compared (commit hash / build number / a hash of the
   XASL structure)
4. **Where fixtures are kept** — NG1 check
5. **A first list of queries** — the small number, chosen to create contention, that E9 will use

**ADR placeholder:** ADR-EXT-010 — the production path + the format + how a version is identified +
where they are kept

## 8. Risks

1. **Being tied to the XASL format is the whole of the upkeep cost.** If the engine changes its
   internal structure the fixtures are invalidated in one go. That is why regeneration has to be
   one run of a script
2. **Leave version identification out and it goes wrong silently** — §4. It is a malfunction rather
   than a failure, so it is found late
3. **Production depends on a client** — the build pipeline needs a client
4. **No conflict with NG2 / NG4** — a new entry point, with CUBRID as the SUT
