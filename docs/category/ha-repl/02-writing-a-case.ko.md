# 2. `ha_repl` 케이스 쓰기

*[English](02-writing-a-case.md) · 한국어*

[← ha_repl 카테고리로](README.ko.md)

`ha_repl` 케이스는 오라클이 두 번째 노드인 [`sql` 케이스](../sql/02-writing-a-case.ko.md)다. 그
문서를 먼저 읽어라 — 구문들, `holdcas` 틀, 이름 짓기, 함정들이 전부 같다 — 그리고 이 문서는
페어가 무엇을 바꾸는지를 위해 읽어라.

- [변환이 무엇을 바꾸는가](#변환이-무엇을-바꾸는가)
- [이 문서가 말해주지 않는 것](#이-문서가-말해주지-않는-것)
- [케이스 쓰기](#케이스-쓰기)
- [페어](#페어)
- [함정들](#함정들)

## 변환이 무엇을 바꾸는가

`sql` 케이스는 구문들의 파일과 그 옆의 기대 텍스트 파일이다:

```
sql/_13_issues/_26_1h/
├── cases/cbrd_26541.sql       the statements
└── answers/cbrd_26541.answer  what running them produced
```

**`ha_repl` 은 구문들을 가져가고 답을 버린다.** 케이스가 마스터에서 돌고, 러너가 복제를 기다리고,
두 노드를 덤프해서 덤프를 비교한다. 케이스가 단언하는 것은 더 이상 *"엔진이 찍는 것이 이것이다"*
가 아니라 *"슬레이브가 마스터가 가진 것을 갖고 있다"* 다.

결과가 셋이고, 이것이 `sql` 케이스를 쓰는 것과 다른 점의 전부다:

1. **요점이 출력 형태인 케이스는 여기서 할 말이 없다.** `--[er]` 케이스들 — `sql` 코퍼스의 2,911
   개가 오류의 텍스트를 고정하려고 존재한다 — 은 두 노드가 아무 일도 없었다는 데 동의하는
   케이스로 변환된다. 하나를 변환하는 것이 틀린 것은 아니고, 비어 있는 것이다.
2. **요점이 데이터에 대한 side effect 인 케이스는 정확히 옳다.** 쓰는 모든 것, 그중에서도 특히
   복제 로그가 그 쓰기 경로를 표현해야 하는 모든 것 — 타입, 기본값, auto-increment, 트리거,
   파티션, `ON UPDATE` — 이 복제 결함이 숨을 수 있고 답 파일은 볼 수 없는 자리다.
3. **비결정성은 문제이기를 그만두고 요점이 된다.** 런마다 다른 답을 내는 `sql` 케이스는 거기서는
   쓸 수 없다. 여기서는 두 노드가 같은 것을 보므로 비교가 성립한다 — 그 값이 노드마다 *다시
   계산되는* 것이 아니라 *복제되는* 한에서. 그 마지막 조건이 [첫 번째 함정](#함정들)이다.

## 이 문서가 말해주지 않는 것

**변환된 코퍼스의 디스크상 배치.** 그것은 이 저장소가 아니라 CTP 의 트리 (`cubrid-testtools`) 에
살고, 이 문서를 쓰기 위해 읽히지 않았다. 여기 기록된 것은
[`module-ha.md`](../../project/design/module-ha.md) §1 이 그 스위트에 대해 측정한 것 — 코퍼스는
변환된 `sql` 코퍼스이고, 오라클은 마스터 대 슬레이브의 덤프 대 덤프이며, 페어는 러너가 만든다 —
그리고 `internal/sandbox` 와
[ADR-022](../../project/adr/ADR-022-topology-provider.md) 가 토폴로지에 대해 establish 하는 것이다.

케이스를 더하기 전에 파일 배치는 트리 안의 이웃 케이스를 읽어라. 거기서 찾은 것이 여기 적힌
무엇과 어긋나면, 옳은 것은 트리이고 고쳐야 할 것은 이 문서다.

## 케이스 쓰기

구문들은 `sql` 케이스의 구문들이다. 바뀌는 것은 무엇을 쓰기로 고르느냐이고, 그것은
[변환](#변환이-무엇을-바꾸는가)에서 따라 나온다:

```sql
--+ holdcas on;
--auto-increment continues across the replica

create table t_seq (i INT AUTO_INCREMENT PRIMARY KEY, v VARCHAR(10));
insert into t_seq (v) values ('a'), ('b'), ('c');
delete from t_seq where v = 'b';
insert into t_seq (v) values ('d');

--+ holdcas off;
```

끝에 `select` 도 없고 답 파일도 없다. 케이스의 주장은 이 구문들 뒤에 `t_seq` 가 두 노드에서 같은
객체라는 것이다 — 엔진이 할당한 칼럼까지 포함해서. 그것이 답 파일이라면 한 노드에서 고정하고
다른 노드에서는 결코 확인하지 않았을 부분이다.

**케이스 안의 무엇도 토폴로지를 만들거나 움직이지 않는다.** 러너는 첫 구문이 돌기 전에 페어를
갖고 있고, `ha_repl` 은 `hb stop` 도, `service stop` 도, `kill` 도 하지 않는다. 노드가 사라지기를
원하는 케이스는 [`ha-shell`](../ha-shell/README.ko.md) 케이스이고, 노드가 *닿을 수 없게* 되기를
원하는 케이스는 둘 다 아니다 — 그것은
[`module-ha.md`](../../project/design/module-ha.md) §4 의 group B 이고 fault verb 가 필요하다.

## 페어

`ha_repl.conf` 가 그것을 지고 있고, 키 이름들은 CTP 의 동결된 표면이다
([ADR-003](../../project/adr/ADR-003-external-surface-freeze.md)):

```
env.instance1.master.ssh.host=...
env.instance1.slave.ssh.host=...
env.instance1.slave2.ssh.host=...      # a master may have several
ha_db_list=...
```

**샌드박스 경로에서는** — 새로 쓰는 것에 대해서는 그쪽이 기본이다
([ADR-022](../../project/adr/ADR-022-topology-provider.md)) — 그 파일을 쓰지 않는다. 페어는
요청하는 것이다:

```bash
make -C extensions/cluster-sandbox dist
export CSB_HOME=/somewhere
extensions/cluster-sandbox/bin/csb cluster create --name tkha --build /path/to/install.out
```

그러면 `csb cluster describe --format ctp --instance instance1` 이 위의 조각을 정확히 그대로
렌더링한다. 키 이름들은 치환 하나와 함께 의미를 유지한다: 노드들은 sshd 를 돌리지 않으므로
`ssh.host` 는 *이 instance 가 어디 있는지* 를 가리키고, 전송은 포트가 아니라 `csb node exec` 다.
`internal/sandbox` 는 그 조각을 conf 파일이 만들어내는 것과 같은 `topology.Instance` 로 읽어들여서,
하류의 어떤 것도 노드가 무엇인지에 대한 두 번째 모델을 배우지 않는다.

**슬레이브 여럿이 동작하고, 전에는 그러지 않았다.** `conf/ha_repl.conf` 는 늘 `slave1`, `slave2`
를 문서화해 왔는데 `internal/topology` 는 고정된 목록이 이름을 댄 역할만 읽었다 — 그래서
`env.instance1.slave2.*` 는 통째로 버려졌다. 이제는 키들이 언급하는 역할이라면 그 목록이 이름을
대든 말든 지고 간다 ([ADR-022](../../project/adr/ADR-022-topology-provider.md) Consequence 4). 두
번째 슬레이브에 의존하는 첫 케이스를 쓰고 있다면, 그것이 동작하는 이유가 그 수정이다.

## 함정들

- **다시 계산되는 것은 복제되는 것이 아니다.** `SYSDATETIME`, `RANDOM`, `SYS_GUID`, 그리고 노드마다
  평가되는 그 밖의 무엇이든 복제 결함이 하나도 없는데도 마스터와 슬레이브 사이에서 달라진다.
  그런 것을 담은 케이스는 시계를 시험하고 있는 것이다. 그 값이 다시 평가되는 것이 아니라
  복제된다는 것이 *요점* 이라면, 첫 줄 주석에 그렇게 적어라. 그러지 않으면 다음 독자가 flaky
  하다고 지울 것이다.
- **비교하는 것은 덤프이고, 그래서 덤프에 없는 것은 확인되지 않는다.** 카탈로그 테이블, 시리얼의
  내부 위치, 덤프가 지고 가지 않는 로그에 떨어지는 결함은 여기서 보이지 않는다. 이 스위트가 전부를
  비교하는 것처럼 보이는데도 그렇다.
- **변환은 동결돼 있지 않다.** 동결은 HA *shell* 코퍼스를 덮지 이것을 덮지 않는다. 변환된 케이스는
  편집될 수 있고, 그것은 곧 아무도 알아채지 못한 채 원래의 `sql` 케이스에서 멀어질 수도 있다는
  뜻이다.
- **sleep 은 기다림이 아니다. 여기서도 그렇다.** 러너 자신의 동기화는
  `ha_sync_detect_timeout_in_ms` 까지 폴링되는 플래그다. "복제가 따라잡게" 하려고 케이스 안에
  지연을 더하지 마라 — 러너의 기다림이 충분하지 않다면 바꿀 것은 그 한계이고, 타이밍 문제를 지연
  뒤에 숨기는 케이스는 그것이 존재한다는 유일한 신호를 없애버린다.
