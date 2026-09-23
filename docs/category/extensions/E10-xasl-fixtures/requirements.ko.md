# E10 — XASL Fixture Production and Storage (Requirements)

*[English](requirements.md) · 한국어*

**Source:** E9 재정의 과정에서 도출 (2026-09-04) · `E9-storage-fuzzing/requirements.md` §4a·§4b
**Status:** incubating (신규 — E9 Tier 2 의 선결 조건)
**축 매핑:** 축 1·3 보조 설비 — 자체 오라클이 없다. *다른 항목이 쓰는 자산* 을 만든다
**사다리 위치:** 없음. 사다리는 fuzzing 항목의 착수 순서이고 본 항목은 그 밑의 설비다
**Companion docs (후속):** `design.md`, `io-contract.md`, `test-corpus.md` (모두 STUB)

---

## 1. 이 항목이 해결하는 문제

**CUBRID 서버는 SQL 을 컴파일하지 않는다.** 확인된 사실이다 — `libcubrid.so`(SERVER_MODE)
에 `parser_main`, `pt_compile`, `do_prepare_select`, `xts_map_xasl_to_stream` 이 없다.
컴파일과 XASL 직렬화는 **클라이언트 몫** 이고, 서버는 직렬화된 스트림을 받아
`stx_map_stream_to_xasl ()` 로 되돌려 실행한다.

그래서 **소비 절반은 엔진에 있고, 생산·보관 절반은 어디에도 없다.** 트리 안에 XASL 스트림을
파일로 쓰거나 읽는 코드가 없다.

본 항목은 그 빈자리다 — 질의로부터 **XASL 픽스처를 만들어 보관하고, 버전을 식별하고,
재생 가능한 형태로 내주는** 설비.

## 2. 소비자

E9 하나가 아니다. 그래서 E9 안의 한 절이 아니라 독립 항목이다.

| 소비자 | 무엇을 위해 |
|---|---|
| **§6a-E9** (storage concurrency) | 연산을 합성하지 않고 재생하기 위해. Tier 2 의 선결 조건 |
| §6a-E3 (SQLancer) | 같은 질의의 플랜이 빌드 간에 달라졌는지 비교 (잠재) |
| 플랜 안정성 회귀 | 같은 질의 · 같은 통계에서 플랜이 흔들리는지 (잠재) |

E3 와의 관계는 **생산자–소비자** 다. E3 는 질의를 만들고, 본 항목은 그 질의를 픽스처로
굳히고, E9 는 굳은 것을 재생한다. 겹치지 않는다.

## 3. 픽스처 스키마

정본은 클라이언트가 실행 요청에 싣는 것이다 — `sqmgr_execute_query ()` 의 unpack 순서:

```c
OR_UNPACK_XASL_ID  (ptr, &xasl_id);
ptr = or_unpack_int (ptr, &dbval_cnt);
ptr = or_unpack_int (ptr, &data_size);
ptr = or_unpack_int (ptr, &query_flag);
OR_UNPACK_CACHE_TIME (ptr, &clt_cache_time);
ptr = or_unpack_int (ptr, &query_timeout);
```

| 항목 | 필수 | 근거 |
|---|---|---|
| `sql_user_text` | 예 | 사람이 읽고 재현·triage 하는 단위. **`sql_hash_text` 는 안 된다** — 재작성된 해시 키다 (`xasl_cache.h` `EXECUTION_INFO`) |
| **엔진 빌드 식별자** | 예 | §4. 엔진이 알려주지 않는다 |
| XASL 스트림 `buffer` + `buffer_size` | 예 | 실행 대상 |
| host variable 묶음 + `data_size` | 예 | 요청에 실려 온다 |
| `query_flag` | 예 | 실행 의미를 바꾼다 |
| `query_timeout` | 예 | 요청에 실려 온다 |
| `dbval_cnt` | 선택 | 스트림 헤더에 이미 있다 (`or_unpack_int (p, &xasl->dbval_cnt)`). 교차 검증용 |
| `clt_cache_time` | 아니오 | 클라이언트 캐시 협상용. 재생에서는 null 고정 |

## 4. 버전 식별이 필수인 이유

`stx_map_stream_to_xasl ()` 이 검사하는 것은 **포인터 non-null 과 `xasl_stream_size > 0`
뿐이다.** 곧바로 `or_unpack_int` 로 헤더 크기를 읽고 오프셋을 계산해 트리를 복원한다.

**포맷 검사도 버전 검사도 없다.** 다른 빌드에서 만든 스트림은 거부되지 않고, 의미가 달라진
오프셋으로 역직렬화된다 — 실패가 아니라 *조용한 오작동* 이다. 재생을 전제로 하는 설비에서
이것은 치명적이므로, **빌드 식별자를 픽스처에 기록하고 불일치 시 거부하는 것이 본 항목의
핵심 요구사항** 이다. 엔진을 고치는 것이 아니라 픽스처 계층에서 막는다.

## 5. 사용자 요구사항 (incubating 추정)

1. **생산** — 질의(텍스트)로부터 픽스처를 만든다. 클라이언트 측 컴파일 경로를 거쳐야 한다
2. **보관** — 파일 포맷. 사람이 목록을 읽을 수 있어야 하고(§3 의 `sql_user_text`), 스트림은
   바이너리다
3. **버전 거부** — 빌드 불일치 시 조용히 통과시키지 않는다 (§4)
4. **재생성** — 엔진이 바뀌면 같은 질의 목록으로 다시 뽑는다. 스크립트 한 번으로
5. **NG1** — 픽스처는 testcases 레포 밖

## 6. 비기능 요구

| 항목 | 의제 | 비고 |
|------|------|------|
| 도입 비용 | 중 | 클라이언트 측 도구 하나 + 포맷 하나 |
| 엔진 변경 | **없음** | 직렬화는 이미 클라이언트에 있다. 엔진에 새로 넣을 것이 아니다 |
| 유지비 | **XASL 포맷 종속** | 엔진 내부 구조가 바뀌면 픽스처가 썩는다. 재생성 스크립트가 그 대가 |
| 선결 | 없음 | E5·E9 와 독립적으로 시작할 수 있다 |

## 7. incubating 진입 조건

1. **생산 경로 선정** — csql / CCI / JDBC 중 어느 클라이언트로 뽑을지
2. **포맷 확정** — §3 스키마의 파일 표현
3. **빌드 식별자 정의** — 무엇을 비교할지 (커밋 해시 / 빌드 번호 / XASL 구조 해시)
4. **픽스처 보관 위치** — NG1 점검
5. **질의 1차 목록** — E9 가 쓸, 경합을 만들도록 고른 소수

**ADR placeholder:** ADR-EXT-010 — 생산 경로 + 포맷 + 버전 식별 방식 + 보관 위치

## 8. 위험

1. **XASL 포맷 종속이 유지비의 전부다.** 엔진이 내부 구조를 바꾸면 픽스처가 일괄로
   무효가 된다. 재생성이 스크립트 한 번이어야 하는 이유
2. **버전 식별을 빠뜨리면 조용히 틀린다** — §4. 실패가 아니라 오작동이라 발견이 늦다
3. **생산이 클라이언트에 의존** — 빌드 파이프라인에 클라이언트가 필요하다
4. **NG2 / NG4 충돌 없음** — 신규 진입점, CUBRID 가 SUT
