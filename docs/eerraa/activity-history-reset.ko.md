# Activity 기록 초기화 구현 핸드오버

## 사용 방법과 범위

- 버전: `1.1.8102`. 기존 `1.1.8101` 후보 설치파일은 덮어쓰지 않는다.
- Execution Center → 설정 메뉴 → **Activity 기록 초기화…** → 범위를 읽고 명시적으로 확인한다. 취소가 기본이다.
- 현재 AgentDock 홈 전체의 Activity journal, 호출 관리 overlay, 오래된 payload blob과 이어읽기 source를 삭제한다. 대화 하나나 작업공간 하나만 초기화하는 기능은 아니다.
- 프로젝트 파일, 작업/스레드 상태, 대화 registry/continuation binding, 권한, 삽입 메시지, 설정, skills/plugins, 운영 로그는 대상이 아니다. `.agentdock` 전체 초기화가 아니다.
- 도구 응답 최종 기록, 명령 프로세스, 승인 대기가 남아 있으면 초기화를 거부한다. 실행을 중단하거나 Core를 재시작하지 않는다.
- 최근 5분 이내 생성·재사용된 blob은 기존 publication grace를 유지한다. 완료 화면은 회수 용량, 남은 용량, 보호/부분 정리 상태를 보여준다. 도구 호출 없이 5분이 지난 뒤 다시 초기화하면 남은 보호 blob도 회수할 수 있다. 자동 타이머 GC를 추가하지 않는다.
- 영구 삭제는 되돌릴 수 없다. 이미 `not_stored`였던 출력은 복구되지 않는다. 원본 컴퓨터의 기록을 이 개발 PC에서 삭제·검증한 것은 아니다.

## 책임과 안전 경계

| 소유자 | 구현 |
|---|---|
| `internal/activity/reset.go` | 기존 payload → journal 파일 잠금 순서. 알려진 segment·hash blob·overlay만 개별 삭제. 링크/비정상 inventory 거부. injected `now`로 grace 경계 검증 |
| `sequence.json` | 삭제 전 `seq+1`, `pruned_through=seq+1`을 원자적으로 기록하는 commit 경계. 별도 epoch/schema/migration 없음. sequence를 0으로 되돌리지 않음 |
| `internal/activity/store.go` | 회전·시간 정리가 replay floor를 낮추지 않도록 `max` 적용. 부분 삭제 실패 뒤 오래된 파일이 남아도 옛 이벤트를 다시 재생하지 않음 |
| `internal/app/activity_reset.go` | local management와 영구 삭제 확인 필수. admission 잠금, active/pending/command/completion reservation 확인. closing 거부. 기존 sidebar 캐시 무효화 |
| `callObserved` / `RecordToolResponse` | admission부터 final adapter audit까지 reset read lease 유지. child는 root 바인딩을 유지함. root identity/총계 불변 |
| `executePrepared` / `watchApprovalCommand` / `RuntimeApprovalDecision` | 지연 승인 결과·progress·승인 결정 기록·명령 완료에 따른 승인 settlement가 끝날 때까지 보호함 |
| `internal/httpx/execution_routes_overview.go` | `POST /internal/runtime/execution/history/reset`, `{ "confirm_permanent": true }`. 기존 direct loopback·Bearer·Origin·proxy 거부·12초 deadline·strict bounded JSON 경계를 공유. MCP/AI 도구로 노출하지 않음 |
| 제어판 `ActivityHistoryService.cs` | 기존 `ActivityClient`의 인증·timeout·bounded response·취소·리다이렉트/프록시 거부를 사용. destructive request 자동 재시도 없음 |
| `ExecutionWindow.ActivityReset.cs` | 확인 대화상자, 완료 결과, 선택 상세/호출 목록/관리 목록 무효화. SSE gap은 다른 창의 옛 호출도 비우고 현재 목록을 다시 읽음 |

삭제 후 실패는 rollback으로 표현하지 않는다. `cleanup_incomplete=true`와 실제 잔여 용량으로 보고한다. 용량 accounting 최종 쓰기 실패 시 초기 preflight의 보수적 사용량이 남아 quota GC 때 재계산되므로 quota를 과소 집계하지 않는다. sequence 상태 손상/쓰기 거부나 링크 검출은 commit 이전에 중지한다.

## 회귀 범위

- Store: 실제 디스크 회수, cached/새 Store 재조회, SSE cursor 단조 증가, overlay 제거, grace 정확한 경계·hash 재사용·capture 후 publish, 취소·거부된 쓰기, 부분 삭제 실패 후 회전/restart/재시도, 링크·범위 외 파일 보호.
- Runtime: local/confirmation, reset 중 admission 거부, root 실행 중 거부, final envelope까지 보호 및 idempotent release, 승인 대기, task 없는 비동기 명령의 완료, task/settings/project/대화 identity 보존, 초기화 이후 동시 대화와 unattributed 호출.
- HTTP: 실제 MCP fixture에서 익명·method·확인 누락·임의 path 주입 거부, 기존 detail 제거, registry 보존, 새 호출 정상 처리.
- Desktop: `--activity-reset-contract-only`는 실제 ActivityClient와 메모리 HTTP handler만 사용한다. 인증 POST/명시적 확인, 큰 cursor 정밀도, 취소, 재시도 없음, busy 한국어 표시를 확인하며 UI·운영 runtime·Setup은 실행하지 않는다.
- 최종 Go 전체·vet, desktop pure policy, 한국어 key/format, client 계약, WPF 및 layout 프로젝트 build. 실제 WPF 수용·Setup 설치/업그레이드·Linux race는 실행하지 않은 단계로 구분한다. GitHub Actions는 사용자의 repository 차단 정책을 유지한다.

## 전달 상태

- Go 전체 최종 검사: Activity/app/HTTP/MCP/command 및 나머지 관련 패키지 통과. 기존 실패 2건 유지: `TestManagedTaskFirstPage1000` 4.7737344초(목표 2초), `TestSearchTextRGExitCodes`는 기대 `SEARCH_FAILED`와 실제 `INVALID_REGEX` 불일치. 이전 baseline에서도 확인한 실패이며 이번 변경의 전체 검사를 녹색으로 보고하지 않는다.
- 승인 정리 경계 보강 후 app/HTTP/MCP 전체 재검사 통과. 최종 추가 boundary 시험을 포함한 `go test ./internal/activity ./internal/app ./internal/httpx -run ActivityReset -count=1 -v` 통과. symlink fixture는 Windows의 링크 생성 권한 부재로 skip이며 별도 권한 우회는 하지 않았다. 범위 외 일반 파일 보존·빈 초기화 재시도·commit 후 취소 시험은 별도로 통과했다. 승인 명령 watcher의 journal 기반 settlement가 끝날 때까지 reset lease 유지.
- `go vet ./...`, 변경 Go 파일 gofmt, `git diff --check`, `verify-version v1.1.8102` 통과.
- Desktop pure policy 923개 단언, 한국어 presentation 6,942개 단언, 실제 ActivityClient reset 계약 통과. control-panel 및 layout 프로젝트 Release build 경고/오류 0.
- 로그: `%TEMP%/agentdock-history-reset-{go-final,boundary-final,focused-final,storage-final,vet,desktop-build,desktop-policy,client-contract,localization,layout-build}.log`. 실패 로그를 그대로 보존한다.
- 후보 Setup 패키징·자산 검증은 준비 단계다. GitHub 게시와 실제 설치·업그레이드·WPF runner 수용·Linux race는 실행하지 않았다. 운영 Core/설치/원본 로그는 변경하지 않았다.
