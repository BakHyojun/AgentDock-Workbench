# AgentDock Activity payload 아키텍처 검토 핸드오버

고출력 CUA 응답으로 Activity full payload 저장소가 포화된 현상을 설명하는 구현 근거와 개선안을 보존한다. 단기 구현은 **256 MiB quota를 유지하고 큰 Activity 이력은 프리뷰만 보존**한다. 장기적으로 독립 byte retention과 CUA output 정책을 검토한다. quota 포화와 ChatGPT 연결 해제의 직접 인과관계는 확인되지 않았다.

검토 대상 소스는 `d8317bad04d6be447a6303b3afa6962babf10799`이다. 아래 구현 위치는 이 revision 기준이다. 운영 PC 데이터에 접근하거나 사건을 재현하지 않았다. 아키텍처 복원과 용량 표는 이 기준 소스의 동작을 설명한다. 아래 승인 범위는 현재 작업 트리의 구현이며 그 밖의 대안은 미구현 제안이다. 원본 PC의 실행 바이너리와 이 소스의 revision 일치 여부도 확인하지 못했다.

사용자에게는 결정할 사항과 불확실성만 짧게 보고한다. 상세 구현 설명과 후속 시험 계획은 이 문서를 참조한다. 프로젝트 규칙은 [fork map](fork-map.ko.md)과 저장소 `AGENTS.md`를 따른다. 변경 이력은 Git에 남긴다.

## 승인된 구현 범위

- 일반 모드: 민감정보를 제거한 뒤 들여쓴 JSON이 **256 KiB 초과**이면 **최대 2 KiB UTF-8 프리뷰**와 원문 bytes/lines/reason을 journal에 남긴다. 정확히 256 KiB는 full 저장한다. `state=preview_only`, `ref` 없음으로 의도적 생략과 `not_stored` 실패를 구분한다.
- `activity.Store.CaptureHistoryPayload`가 기존 `capturePayload` 구현을 공유하며 preview 결정을 blob lock/hash/usage/GC 이전에 한다. 작은 이력은 기존 저장 경로를 사용한다.
- `display-settings.json`의 `activity_full_payload_debug`는 기본 false다. 기존 schema 1/2 파일은 변경 없이 읽는다. Execution Center 환경설정에서 명시적으로 켠다. 로컬 API와 원자적 revision 검사를 사용하며 Core 재시작은 필요 없다.
- `callObserved`가 입구에서 정책을 snapshot하고 `preparedExecution`과 adapter `ToolResponse`가 이를 보존한다. 지연 승인 및 최종 MCP envelope에도 같은 정책이 적용된다.
- 실제 도구 결과의 전달·기존 output truncation 정책은 바꾸지 않는다. 발급된 `activity://.../source`를 위한 `CapturePayload`는 normal/debug 이력 정책에 관계없이 기존 full capture를 유지한다. source가 있으면 Execution Center는 기존대로 source를 보여 준다.
- 16 MiB 단일 안전 상한과 256 MiB 전체 blob quota는 유지한다. compact 또는 formatted JSON이 16 MiB를 넘으면 기존처럼 프리뷰 생성 이전에 `not_stored`가 된다. 디버깅은 이 제한을 우회하지 않는다.
- 기존 이력을 삭제하거나 migration/GC/eviction/압축/CUA response 변경/자동 polling 변경을 추가하지 않는다. 이미 포화된 원본 PC 저장소는 이 변경으로 회수되지 않는다. continuation source와 작은 payload도 계속 전체 quota를 소비한다.
- 새 상태·정책 설명과 체크박스는 영어/중국어/한국어 공통 resource에 추가한다. 구형 UI는 새 상태를 알 수 없으므로 Core와 제어판은 같은 배포로 제공해야 한다. debug=true 파일은 신형에서 읽지만 필드를 모르는 구형 strict reader로 downgrade하면 설정 경고가 발생할 수 있다. false는 디스크에서 생략한다.

## 설치파일 준비 상태

- Windows x64 **1.1.8101** 미게시 후보: `D:/Engineering/release/agentdock-1.1.8101/release/AgentDockSetup-amd64.exe` (112,727,744 bytes).
- 설치파일의 소스는 `a74a5ced8e9330a19de5cb7f3ffe13936522800c`다. 기능 구현 `7872a12a` 이후 변경은 두 버전 선언과 fork map뿐이다. 새 일반 clone에서 source-clean 상태로 기존 `build-windows-release.ps1 -Architectures amd64 -Candidate`를 사용했다.
- Setup SHA-256: `03ddc2f54034bfd4fda6540aac46368f1b8a4ce1f553f0a006fe148f1fe57707`. 같은 폴더의 `.sha256`, `build-report.json`, `verification-scope.json`과 각 checksum을 함께 보존한다.
- 기존 자산 검증기 `verify-windows-release-assets.ps1 -IncludeMetadata`가 source/version/PE architecture/체크섬/rg 5개 파일/실제 Core skill·CUA plugin bootstrap을 임시 home에서 검증했다. Setup은 실행하지 않았다.
- 별도 unpacker로 **완성된 Setup**을 수동 설치 없이 추출했다. 안의 ZIP은 검증한 payload ZIP과 SHA-256이 같으며 cloudflared도 원본과 같다. 실제 Setup의 Korean 메시지, ZIP의 rg/CUA 파일, 실제 single-file tray 안의 현재 desktop assembly와 한국어 satellite assembly의 바이트 일치를 확인했다. satellite의 1,135 resource와 새 preview/debug 한국어 값도 확인했다.
- AgentDock/Setup은 **unsigned**, 포함된 공식 cloudflared 서명은 **Valid**다. 공식 `Korean.isl`의 obsolete font 지시문 네 개 경고는 원문을 유지하며 컴파일에 성공했다.
- 설치·업그레이드·운영 Core 교체/재시작·runner WPF acceptance·Linux/race·GitHub Release 게시·게시 자산 redownload는 실행하지 않았다. 전체 Go suite의 기준선 실패 두 건은 아래에 그대로 보존했다. 후보 준비와 정식 release acceptance를 혼동하지 않는다.
- 빌드 및 자산/내용 검사 로그는 `D:/Engineering/release/agentdock-1.1.8101/`에 있다. unpacker 출처와 hash도 `verification-scope.json`에 기록한다. 게시하려면 이 metadata의 미실행 관문을 실제 isolated runner evidence로 채워야 한다.
- 이 fork의 GitHub Actions는 사용자 정책에 따라 repository permissions `enabled=false`로 차단한다. 자동·수동 CI를 실행하거나 사용자 지시 없이 다시 켜지 않는다. 로컬 빌드와 Actions 차단은 별개다. 설정 적용 후 모든 non-terminal status의 workflow run이 0건임을 확인했으며 종료된 실패 기록은 보존한다.

## 1 Executive Summary

- **B:** 소규모 large capture 정책과 저장 상태 표시를 단기에 구현한다. quota 증설/configuration/warning은 후속 검토 대상으로 남긴다.
- **C:** quota 증설은 임시 대응이다. 참조된 blob을 독립적으로 expiry 또는 eviction할 수 있어야 지속적으로 최근 상세를 남긴다.
- **D:** CUA와 일반 MCP는 공통 저장소를 사용하되 capture와 output 정책을 다르게 적용한다.
- **A만으로 해결 완료라고 판단하지 않는다.** 2 MiB 고유 blob이면 256 MiB는 128개, 1 GiB도 512개다.

현재도 journal과 external blob은 분리되어 있고 SHA-256 dedup과 미참조 GC가 있다. 새 Activity 시스템이 필요한 것이 아니라 기존 `activity.Store`의 capture, accounting, retention, presentation 계약을 보완해야 한다.

## 2 제공받은 현장 사실

아래는 사용자 제공 원본 PC 조사 결과다. 이번 머신에서 다시 확인한 로그가 아니다.

| 사실 | 입력 증거 | 현재 소스로 설명 가능한 범위 |
|---|---|---|
| A | Activity usage 268,435,447 bytes, 256 MiB보다 9 bytes 작음 | 고정 quota와 일치. ledger와 실제 파일 합계는 일시적으로 다를 수 있음 |
| B | request/response `not_stored`와 `rpc_status=succeeded` 공존 | capture 실패가 business result를 바꾸지 않는 구현과 일치 |
| C | context RPC 성공, 약 44 KB와 285 lines, 상세는 미저장 | 결과 생성과 Activity 저장은 별도 단계 |
| D | MATLAB session을 4–6초 간격으로 observe, 대부분 성공 | 정상 외부 polling roots가 누적되는 구조 |
| E | MATLAB 이전 CUA 응답 약 2.27 MB와 52,845 lines, 다른 응답 2.3–2.7 MB | 16 MiB 아래의 큰 payload가 반복 full capture될 수 있음 |
| F | capability와 connection 정상, failed section 없음 | quota 포화만으로 Core crash를 뜻하지 않음 |
| G | context 열기 실패 뒤 ChatGPT 연결 해제 | host/transport 경계 증거가 없어 직접 원인 미확정 |

원본 JSONL, payload 파일, 호출 시각, CUA 버전과 당시 schema, host resource 요청, response delivery는 검증하지 않았다. 검토 세션에 `agentdock_context` 도구가 노출되지 않아 호출하지 못했다. 저장소 규칙과 소스는 직접 읽었으며 runtime을 새로 시작하지 않았다.

## 3 현재 Activity architecture

```text
MCP tools/call
  → Runtime.Call / callObserved
  → request CapturePayload
  → call.created + call.payload journal
  → binding / permission / admission
  → handler 또는 external MCP
  → call.completed journal
  → output policy
  → MCP final envelope
  → RecordToolResponse
  → response CapturePayload
  → redaction / formatting / hash / quota / blob
  → call.payload + call.rpc_returned journal
  → SDK 반환 → transport → host

journal → ExecutionCall projection → local HTTP → Execution Center
host → MCP App HTML resource → structuredContent 알림 → widget
```

| 책임 | package와 함수 | 소스 |
|---|---|---|
| 진입과 root identity | `app.Runtime.Call`, `callObserved` | [runtime.go](../../internal/app/runtime.go), [execution_dispatch.go](../../internal/app/execution_dispatch.go) |
| 실행과 최소 결과 기록 | `executePrepared`, `finishPrepared` | [execution_dispatch.go](../../internal/app/execution_dispatch.go) |
| external MCP | `tool/mcp.Service.Call`, `sdkProtocolClient.callTool` | [service_actions.go](../../internal/tool/mcp/service_actions.go), [protocol.go](../../internal/mcp/client/protocol.go) |
| 최종 응답 | `toolEnvelope`, `dynamicMCPToolEnvelope`, `finishResponse` | [server.go](../../internal/mcp/server.go), [response_additions.go](../../internal/mcp/response_additions.go) |
| adapter capture | `RecordToolResponse`, `executionPayloadEvent` | [execution_payload.go](../../internal/app/execution_payload.go) |
| 저장과 quota | `activity.Store.CapturePayload` | [payload.go](../../internal/activity/payload.go) |
| journal rotation | `AppendBatch`, `appendBatchLocked` | [store.go](../../internal/activity/store.go) |
| projection | `ExecutionCall`, `projectionLocked` | [calls.go](../../internal/activity/calls.go), [call_transitions.go](../../internal/activity/call_transitions.go) |
| 상세 조회 | `serveExecutionCalls`, `readExecutionPayload` | [execution_routes_calls.go](../../internal/httpx/execution_routes_calls.go), [execution_payload.go](../../internal/httpx/execution_payload.go) |
| desktop 표시 | `ExecutionPayloadView`, `LoadPayloadPageAsync` | [ExecutionPayloadView.cs](../../desktop/windows/control-panel/Models/ExecutionPayloadView.cs), [ExecutionWindow.Payload.cs](../../desktop/windows/control-panel/ExecutionWindow.Payload.cs) |

외부 root의 request와 response를 capture한다. 일반 internal child와 diagnostic 호출은 같은 full capture를 반복하지 않는다. adapter 응답은 catalog와 trusted additions를 붙인 최종 envelope를 once-only guard로 기록한다.

## 4 Quota 정의와 storage schema

`internal/activity/payload.go`의 compile-time 상수:

```go
const MaxPayloadBytes = 16 << 20
const MaxPayloadStorageBytes = 256 << 20
const PayloadPreviewBytes = 2048
```

runtime 설정은 없다. `Store.Options`는 segment와 append capacity를 설정하지만 payload quota는 설정하지 않는다. `Runtime`은 `<AgentDockHome>/tasks/activity`에 하나의 Store를 만든다. 같은 root를 공유하는 모든 workspace, conversation, tool의 request/response/output source blob이 256 MiB를 공유한다. 다른 Home/root는 별도다.

```text
tasks/activity/
  <20자리 sequence>.jsonl
  sequence.json
  payload-usage.json
  payloads/<SHA-256>.json
  .activity.lock
  .payload.lock
  call-management.json
```

`Payload` 필드: `state`, `format`, `ref`, `bytes`, `lines`, `preview`, `truncated`, `reason`. journal은 descriptor와 bounded preview를 inline으로 저장한다. full body는 작은 payload도 external blob에 저장하며 full inline 분기는 없다. byte quota는 redacted, indented JSON 파일 길이를 세며 journal, 파일시스템 할당, 임시 파일 비용은 포함하지 않는다. 압축은 없다.

## 5 Quota 도달 시 코드 동작

`CapturePayload`는 입력 marshal → compact JSON 16 MiB 검사 → decode → redaction → indented JSON 16 MiB 검사 → bytes/lines/2 KiB preview 생성 → payload lock → SHA-256 순서로 진행한다.

기존 hash의 regular blob이 있으면 quota 검사 전에 재사용한다. 신규 blob은 `payloadUsageLocked`로 usage를 읽고 `used + len(data) > MaxPayloadStorageBytes`이면 `prunePayloadsLocked`를 호출한다. GC 후에도 초과하거나 GC가 실패하면 `not_stored`, 빈 ref, quota reason을 반환한다. quota 단계에서 이미 만든 bytes/lines/preview는 남는다.

수용 가능하면 usage를 먼저 durable 예약하고 atomic blob write를 한다. crash 또는 write 실패 후에는 usage가 실제 파일 합계보다 클 수 있고 GC에서 재계산한다. 현장 9-byte 잔여량은 실제 디스크 여유 공간을 뜻하지 않는다.

저장 거부는 tool business result를 재실행하거나 실패로 바꾸지 않는다. descriptor와 summary 보존은 journal이 쓰기 가능할 때 성립한다. 16 MiB 초과처럼 preview 생성 이전에 실패하면 preview도 보존되지 않는다. 현재 quota 분기는 GC 오류를 실제 용량 부족과 같은 reason으로 표시할 수 있다.

## 6 Retention과 GC

`store.go` 기본 journal budget은 8 MiB segment 8개, 정상 rotation 기준 약 64 MiB다. append 후 초과 segment를 oldest-first로 삭제하고 `PrunedThrough`를 갱신한다. summary가 남는다는 안내는 영구 보존 보장이 아니다.

`Store.Cleanup(before)`는 현재 segment를 유지하면서 오래된 segment를 파일 mtime 기준으로 제거한다. local control은 최근 24시간을 보호한다. 기본 주기 time cleanup은 확인되지 않았다.

payload GC는 usage 파일 부재 또는 신규 capture quota 초과 때 실행된다. 현재 projection의 request/response/output source refs를 수집하고, 참조가 없으며 mtime이 5분보다 오래된 blob을 삭제한다. 5분은 capture와 journal publication 사이의 경쟁을 보호하는 grace이며 history TTL이 아니다. 재사용 blob은 필요 시 grace를 갱신한다.

참조된 payload의 oldest-first eviction, LRU, 독립 TTL, 주기 background payload GC는 없다. journal cleanup은 payload를 즉시 수거하지 않는다. 휴지통과 permanent-delete metadata도 즉시 byte 회수를 보장하지 않으며 blob GC는 projection의 삭제 상태를 직접 걸러 refs를 해제하지 않는다.

큰 응답 몇백 개로 payload budget이 먼저 차도 작은 journal records는 남는다. 이때 GC는 참조된 blob을 지우지 못해 신규 상세 미저장이 지속될 수 있다.

## 7 CUA workload와 dedup 한계

작은 JSON/text workload에는 256 MiB가 합리적이었을 가능성이 있다. 설계 당시 측정 자료나 의도는 소스에서 확정하지 못했다. CUA의 수 MiB 반복 observation은 기존 크기 가정과 다르다.

현재 default `tool_output.enabled=true`, `max_chars=20000`은 일반 text의 display 정책이다. [tool_output.go](../../internal/app/tool_output.go)는 external MCP `structuredContent`가 있거나 text block이 유효한 JSON이면 임의 절단하지 않는다. schema 보존에는 타당하지만 large structured output의 대체 정책이 없다.

hash 대상은 redacted final envelope 전체다. `decorateExecution`이 root `call_id` 등을 넣으므로 remote body가 같아도 서로 다른 호출의 response hash는 달라진다. CUA snapshot/token도 변할 수 있다. 현재 exact dedup을 CUA 반복 저장의 충분한 완화책으로 볼 수 없다. 원본 샘플의 실제 중복률은 측정하지 않았다.

dynamic envelope은 remote text를 최상위 content와 structured result 내부에 반복 포함할 수 있다. `withoutDuplicatedBinary`는 binary 중복만 줄인다. capture redaction은 image/audio base64와 resource blob을 metadata로 대체하지만 text/tree 비용은 남는다.

## 8 Capacity model

다음은 최종 저장 byte의 가정 모델이다. 원본 PC 실측, 다른 기존 데이터, GC, dedup, eviction을 포함하지 않는다.

```text
payload 증가량 = 고유 request + 고유 response + 필요 시 output source
포화 시간 ≈ 가용 quota / 시간당 고유 저장 byte
```

| workload | 가정 | 증가량 | 256 MiB | 512 MiB | 1 GiB |
|---|---|---:|---:|---:|---:|
| normal | 8 KiB/call, 1000 calls/day | 7.81 MiB/day | 32.8일 | 65.5일 | 131.1일 |
| long command | 4 KiB/poll, 5초 간격, 낮은 출력량 | 2.81 MiB/hour | 91시간 | 182시간 | 364시간 |
| moderate CUA | 2 MiB/call, 60 calls/hour | 120 MiB/hour | 2.13시간 | 4.27시간 | 8.53시간 |
| heavy CUA | 2.5 MiB/call, 5초 간격 | 1800 MiB/hour | 8.53분 | 17.07분 | 34.13분 |

| quota | 2 MiB/blob | 2.7 MiB/blob |
|---|---:|---:|
| 256 MiB | 128개 | 94개 |
| 512 MiB | 256개 | 189개 |
| 1 GiB | 512개 | 379개 |

현장 MB가 decimal인지 MiB인지, upstream 응답인지 저장 blob인지에 따라 실제 capacity는 다르다. overhead는 indentation, string escaping, structured/text 중복, binding/permission/guidance/catalog, output source, journal previews, 파일시스템 할당이다. normal workload는 journal rotation이 payload 한도보다 먼저 history를 제한할 수도 있다.

## 9 Quota 선택

fixed 512 MiB는 작은 긴급 변경이며 format migration이 없다. fixed 1 GiB도 format은 유지하지만 문제가 4배 뒤 재발하고 더 큰 inventory를 유지한다. configurable quota와 512 MiB는 후속 대응 후보다. **승인된 단기 선택은 기존 256 MiB 유지와 normal preview-only**다. 아래 증설 비교는 정책을 바꾸지 않는 경우의 용량 비교이며 적용한 설정이 아니다.

동일 root의 모든 writer는 같은 canonical 설정을 사용해야 한다. quota를 낮출 때 즉시 임의 삭제하지 않고 경고와 신규 capture 정책을 적용한다. display 설정이 Core를 재시작하는 구조로 만들지 않는다.

## 10 Large payload 정책

| 대안 | 평가 |
|---|---|
| per-call maximum | 16 MiB safety ceiling은 이미 있음. normal full capture threshold와 분리 필요 |
| preview-only | normal history의 byte 압력을 직접 줄임. 생략 원인 명시 |
| external blob과 CAS | 이미 있음. 기존 구현 확장 |
| compression | 실제 압축률과 CPU/read 비용 측정 후 도입 |
| exact dedup | 현재 있음. body/envelope 분리가 개선 후보 |
| semantic repeat hash | 유사 상태 표시용. 실제 결과 identity와 분리 |
| oldest-first eviction | 최근 상세를 계속 남기는 중기 핵심 |
| LRU | read마다 metadata 변경 등 복잡도 때문에 초기 제외 |
| time와 byte retention | TTL 목표와 disk hard cap을 함께 적용 |
| per-workspace/conversation quota | fairness에 유용하지만 shared/unattributed accounting 복잡 |
| per-tool 정책과 budget | CUA 독점에 직접 대응 |
| soft/hard quota와 warning | 단기 필요 |
| background GC | 반복 동기 전체 스캔을 줄이되 bounded worker로 구현 |
| compaction | 다른 최소 개선 효과 측정 후 필요할 때 검토 |

작은 payload는 full capture, 큰 normal 응답은 bounded preview와 metadata, 명시적 debug mode는 제한된 full capture를 제안한다. normal threshold 256 KiB는 시험할 후보일 뿐 확정값이 아니다. history capture를 생략한다고 모델에 반환할 structured response를 임의 절단해서는 안 된다.

## 11 CUA producer와 capture

로컬 [plugin](../../plugins/cua-driver/README.md)은 외부 daemon에 붙는 선언과 skill을 제공하며 producer 구현을 포함하지 않는다. [skill](../../plugins/cua-driver/skills/cua-desktop/SKILL.md)은 `query`, `max_elements`, `include_screenshot:false`로 응답을 좁히도록 안내한다.

현재 외부 공식 문서는 `max_elements`, `max_depth`로 bounded observation을 권장하며 structured elements와 Markdown tree를 함께 제공하는 계약을 설명한다. 이는 현재 문서 확인이고 원본 PC의 당시 버전 계약 증거가 아니다. [Window state reference](https://cua.ai/docs/cua-driver/reference/mcp-tools/window-state), [Connect your agent](https://cua.ai/docs/cua-driver/guides/connect-your-agent)

subtree, viewport, delta, changed nodes의 정확한 지원 여부는 이번 조사로 확정하지 못했다. 지원 schema 확인 후 기존 narrowing 기능부터 사용한다. delta는 baseline/reset/resync/expiry가 필요해 초기 최소 변경에서 제외한다.

normal history에는 window/snapshot identity, query 범위, node 수, truncated/degraded, 관찰과 검증 결과, 관련 요소 preview를 남기고 full tree는 실패 분석이나 debug capture에 선택적으로 보존한다. 과거 element token을 재실행 가능한 것으로 표시하지 않는다.

snapshot/token을 버려 서로 다른 실제 결과를 동일 full result로 취급하지 않는다. semantic hash는 유사 UI 상태 표시용, blob hash는 exact stored bytes identity용으로 구분한다.

## 12 Session observe polling

현장의 주요 byte consumer는 CUA라는 입력 증거를 유지한다. polling은 별도 event 수와 관찰 효율 문제다. 4–6초 간격은 시간당 600–900 calls이며 각 call은 여러 lifecycle events를 만든다.

`session_observe(status)`는 running output cursor를 전진시키는 snapshot을 반환한다. 현재 입력에는 long-poll wait나 호출자별 cursor가 없다. `exec_command` running 응답에는 `observe_after_ms=1000`이 있다.

권장 interval은 변화 없음에 따라 5 → 10 → 20 → 30 → 60초로 증가시키고 새 output/status 변화에 따라 감소시킨다. terminal 결과면 종료한다. 종료 예상 시점은 실제 progress가 있을 때만 사용한다. 원 command 재실행은 금지한다.

interval을 늘리면 output limit을 넘는 미수집 부분이 생길 수 있다. snapshot은 제한과 별개로 cursor를 전진시키므로 output completeness도 함께 시험한다. UI는 같은 session polling을 접어 보여줄 수 있으나 실제 외부 roots를 삭제하거나 총수를 왜곡하지 않는다. 내부 command activity의 300 ms 관찰과 외부 AI polling은 별개다.

## 13 Execution과 persistence 격리

quota 거부는 tool outcome을 변경하지 않지만 완전한 latency 격리는 아니다. `finishResponse`가 `RecordToolResponse`를 동기 호출한 뒤 SDK에 반환하므로 encode/decode/redaction, lock, quota/GC, blob write, journal append가 응답 critical path에 있다.

capture는 2초 context, journal append는 별도 5초 context를 사용한다. JSON marshal과 일부 filesystem write/sync는 context가 호출 중간을 강제 중단하지 않으므로 엄격한 전체 지연 상한이 아니다. quota 초과마다 GC가 반복되면 contention과 전달 지연 가능성이 있다. 현장 연결 해제가 이 때문이라는 증거는 없다.

초기 audit journal 실패/append saturation은 `AUDIT_UNAVAILABLE` 또는 `ACTIVITY_CAPACITY`로 dispatch를 거부한다. 이 admission 불변식을 optional history capture와 혼동하거나 제거하지 않는다.

display truncation의 `activity://call/.../source`는 모델 continuation에 쓰인다. 단순 optional history보다 강한 durable publication과 보존 계약이 필요하다. 발급 직후 eviction하거나 저장 전에 참조를 발급해서는 안 된다.

## 14 Context 열기 실패 boundary

Execution Center는 ref 없는 `not_stored` descriptor의 preview/reason을 표시하고 full read를 시작하지 않는다. server도 offset 0에서 preview를 반환한다. ref는 있지만 파일이 없거나 변형되면 read 오류를 표시하며 expired/evicted를 구조적으로 구분하지 않는다.

ChatGPT context App은 [apps.go](../../internal/mcp/apps.go)에서 `AgentDock context` HTML resource를 등록한다. pinned `agentdock-protocol` dependency는 `ui/initialize` → `ui/notifications/initialized` → host의 `ui/notifications/tool-result.params.structuredContent`로 렌더링한다. Activity ref 재조회 경로는 없다.

따라서 quota 포화만으로 context widget mount 실패를 직접 설명할 수 없다. resource 로드/URI/UI 설정, host bridge initialization, serialization/rendering, response-stream 연결은 독립 경계다. 관찰된 한국어 host 오류 문구의 생성 코드는 저장소에서 찾지 못했다. renderer initialization 오류 문구도 다르다.

`rpc_status=succeeded`는 서버가 반환 결과로 계산한 상태이며 최종 downstream delivery ACK가 아니다. AgentDock 결과 생성 성공과 host resource 로드/렌더링 성공을 구분한다. 추가 원인 확인에는 원본 시각의 transport/resource/host 증거가 필요하다.

## 15 설계안 비교

| 안 | 난이도 | 호환성과 migration | disk와 보존 | 위험과 debugging | CUA 및 일반 workload |
|---|---|---|---|---|---|
| A fixed 512 MiB | 낮음 | format 유지 | 2배 headroom | 동기 GC 구조 유지, 임시 개선 | CUA 재포화, 일반 영향 작음 |
| B fixed 1 GiB | 낮음 | format 유지 | 4배 headroom | 큰 inventory, 임시 window | heavy CUA에 여전히 부족 |
| C configurable와 warning | 낮음–중간 | config 확장과 fallback | 사용자 조정 | validation와 writer 일치 필요 | 다양한 workload에 적합 |
| D 증설과 large capture | 중간 | 기존 blob, descriptor 확장 | full 일부 포기, byte 절약 | 오분류 방지와 debug mode | 단기 적합 |
| E 독립 byte/time retention | 중간–높음 | catalog와 old ref reader 필요 | 최신 detail 유지, 오래된 detail 제거 | crash/read/evict 경쟁 시험 | 중기 핵심 |
| tool-aware capture | 중간 | 관리자 소유 정책과 fallback | CUA 독점 제한 | schema 변화 대응 | CUA에 직접 적합 |

E는 metadata/blob을 처음 분리하는 안이 아니다. 현재 분리된 구조에 독립 보존과 availability 계약을 추가하는 안이다. 단기는 large capture와 debug 분리, 중기는 필요성이 입증되면 E와 tool-aware를 검토한다.

## 16 단기 조치 제안

승인된 구현은 위 범위의 normal/debug capture 분리와 `preview_only` 표시다. configurable quota, usage와 warning, 일반 실패 reason code는 이번 범위에 포함하지 않는다. warning 80%/95%는 후속 실측 후보다.

반복 실패 때 같은 전체 GC scan을 동기 수행하지 않도록 bounded scheduling 또는 cooldown을 검토한다. quota만 512 MiB로 바꾸는 긴급 패치는 가능하지만 해결 완료로 보고하지 않는다.

## 17 중기 조치 제안

기존 Store의 payload catalog 후보 필드는 `ref`, `codec`, `logical_bytes`, `stored_bytes`, `created_at`, `expires_at`, `availability`, `protection`이다. journal보다 먼저 detail을 expiry/eviction할 수 있어야 한다.

TTL 만료를 우선하고 byte high-water 초과 때 oldest-first로 low-water까지 정리한다. 오류/debug capture/recent continuation은 bounded 보호를 둔다. 보호 데이터만으로 포화되면 신규 optional capture를 생략한다. 정상 eviction 이유를 durable하게 기록하고 구형 `.json` 읽기는 유지한다.

압축은 대표 샘플로 효과와 CPU/read 비용을 측정한다. stored와 logical bytes를 분리하고 decompression에도 크기 상한을 둔다.

## 18 장기 구조와 상태 모델

최소 durable audit, optional history detail, 발급된 continuation source, producer output contract, presentation을 같은 canonical 구현에서 구분한다. 별도 runtime이나 병렬 Activity 시스템은 만들지 않는다.

| 축 | 권장 상태 |
|---|---|
| Execution | running, succeeded, failed, partial, cancelled, unknown |
| 서버 RPC 결과 | succeeded, failed, pending approval, unknown |
| Capture | pending, full, preview only, not stored |
| Detail availability | available, expired, evicted, missing, read error |
| Completeness | source complete, source partial, preview truncated |
| Host connection | connected, disconnected, unknown |

현재 execution status, RPCStatus와 Payload descriptor는 이미 별개다. desktop은 RPC status를 독립적으로 충분히 드러내지 않으며 expiry/eviction 모델은 없다. `Payload.Truncated`는 preview 길이 제한을 뜻할 수 있어 full source completeness와 혼동하면 안 된다.

표시 예: 실행 성공 / 서버 RPC 성공 / Activity 미저장 / quota 초과 / preview 사용 가능 / host 전달 미확인.

## 19 구현 우선순위

| 우선순위 | 범위 |
|---|---|
| P0 | 승인된 normal/debug large capture와 상태 표시, 원응답·continuation 보존 |
| P1 | quota warning/config 필요성 평가, 독립 retention과 oldest-first eviction, bounded GC, continuation 보호, adaptive observe guidance |
| P2 | 압축, body/envelope dedup, semantic repeat 표시, fair budgets, producer paging/delta |

이번 구현은 승인된 capture 분리에 한정한다. 나머지 구조 변경은 별도 승인을 받아 진행한다. fork 변경은 feature branch와 map에 반영하며 운영 Setup, Core restart, credentials/tunnel 변경으로 검증하지 않는다.

## 20 검증과 회귀 시험 계획

승인된 구현의 최종 검증 결과:

- Go 1.26.5 `go test ./... -count=1`: 새 regression을 포함한 `activity`, `app`, `config`, `mcp`, `httpx`, `command`/`command/session` 등은 통과. **전체 suite는 아래 기존 실패 두 건 때문에 실패**했다. 이를 전체 통과로 보고하지 않는다.
- 변경 전 HEAD `d8317bad`를 `git archive`로 임시 디렉터리에 추출하고 실패한 두 시험만 한 번 검사했으며 둘 다 재현됐다. 기준선이나 테스트 단언을 수정하지 않았다.
  - `internal/taskstate/TestManagedTaskFirstPage1000`: 전체 검사 첫 페이지 4,951.043 ms, 변경 전 소스 2,303.322 ms. 기준은 2,000 ms다. 이번 머신에서 기준을 충족하지 못했으며 성능 원인을 확정하지 않았다.
  - `internal/tool/file/TestSearchTextRGExitCodes`: 시험은 `SEARCH_FAILED`와 `engine=rg`를 요구하지만 변경 전 `SearchText`도 정규식 사전검사에서 `INVALID_REGEX`를 반환한다. 이 둘은 이번 변경 파일이 아니다.
- Windows control-panel Release build와 layout-tests build: 경고/오류 0. 실제 WPF layout runner acceptance는 실행하지 않았다.
- desktop pure-policy: **923 assertions** 통과. Activity Center `--localization-only`: **6900 assertions** 통과. 창·사용자 설정·운영 runtime·설치기를 열지 않았다.
- `go vet ./...` 통과. `gofmt -l` 관련 디렉터리 출력 없음, `git diff --check` 통과. 세 언어 resource 1,135 keys 동일, 핸드오버의 상대 링크와 21개 섹션 검사 통과.
- 새 시험은 256 KiB 경계/indented bytes/redaction, 약 2.7 MB 응답 반복과 blob I/O 차단, preview journal replay, 기존 debug 상세 보존, 16 MiB 안전 한도, 설정 defaults/corruption/cancel/rejected atomic write/revision, concurrent conversations와 unattributed/no-task 호출, 실패 결과 보존, normal/debug admission snapshot과 adapter final envelope, protected continuation source, SDK/Invoke 원문 전달과 UI 상태를 검증한다.
- 최종 로그는 검토 머신의 `%TEMP%/agentdock-activity-{go-final,baseline-failures,desktop-build,desktop-policy,localization,layout-build,vet}.log`에 남긴다. 필요한 실패 내용은 위에 보존했다. 실제 원본 PC의 로그가 아니다.
- 구현 회귀검사 단계에서는 소스 구현과 검증만 수행했다. 이후 별도 설치파일 준비 단계의 패키징·실제 payload·checksum 결과와 미실행 관문은 위의 설치파일 준비 상태에 기록한다.



검토 단계에서 실행한 기존 검증:

- `go test ./internal/activity ./internal/app ./internal/mcp ./internal/httpx ./internal/tool/command/... -count=1`: 전 대상 통과. Go 1.26.5를 사용하고 fork map의 알려진 stale `AGENTDOCK_INSTRUCTIONS_FILE`을 시험 프로세스 환경에서 제외했다.
- 관련 Go 디렉터리 `gofmt -l`: 출력 없음.
- Windows desktop pure-policy: 920 assertions 통과.
- Activity Center `--localization-only`: 6876 assertions 통과.

기존 시험은 redaction/binary metadata/exact integers, dedup/grace/독립 lock, adapter once-only, admission capacity, command completion 등을 검증한다. 원본 사건 재현, 새 포화 실험, 전체 Go suite, 실제 host 연결, WPF runner acceptance, 설치/패키징/게시/asset redownload는 수행하지 않았다. 위 통과가 이 미실행 범위를 대신하지 않는다.

아래는 더 넓은 후속 구조 개선 시 사용할 시험 계획이다. 이번 승인 범위를 넘어선 retention/GC/producer 시험의 실행을 주장하지 않는다:

1. quota 정확한 경계, 신규 거부와 기존 hash 재사용, request/response/source accounting.
2. 포화 시 원 MCP envelope와 execution/RPC 성공 유지, detail 미저장, 재실행/빈 성공/가짜 continuation 금지.
3. usage 예약 후 write 실패, blob 후 journal 실패, crash/restart 후 accounting 회복, GC 오류 reason.
4. 동일 root 여러 Store와 concurrent conversations, task 없는 호출, unattributed 호출, shared refs, capture/publication/read/GC 경쟁.
5. 2–2.7 MiB CUA fixture 반복, 다른 root IDs의 동일 remote body, compact/indented 16 MiB 경계, bounded redaction/preview/work.
6. injected time으로 TTL/high-low water/grace/continuation 경계, expired/evicted/missing과 old unknown 구분.
7. 무출력 backoff, output/status 변화 reset, async completion, cursor/truncation 누락 표시, 원 command 재실행 금지.
8. execution/RPC/capture/completeness/availability/connection 독립 UI와 resource/initialize/result boundary.

120초 activity, 180초 request eligibility, 300초 insertion expiry는 별개로 유지한다. payload publication 5분 grace도 같은 정책으로 합치지 않는다. runner-only guard는 유지하며 실제 installer 시험은 isolated runner/VM에서만 한다.

## 21 최종 판단과 남은 결정

**확인된 결론:** 고정 quota와 참조 종속 GC는 고출력 반복 workload에서 최신 상세를 계속 저장하기 어렵다. payload 저장 거부는 business outcome을 보존하지만 동기 persistence는 delivery 지연에 영향을 줄 수 있다. session polling은 이번 주요 byte 원인으로 판단하지 않는다.

**확정된 단기 결정:** quota 256 MiB 유지, redacted JSON 256 KiB 초과는 2 KiB preview-only, 명시적인 full debug, continuation 원본 보존. **남은 결정:** 독립 byte/time retention이나 quota 조정이 필요한지는 이후 usage로 판단한다. CUA producer의 크기 contract는 별도 개선 대상이다.

**불확실성:** 원본 바이너리 revision, 실제 redacted blob 크기와 중복률, 압축률, 당시 CUA schema, context widget 오류와 연결 해제 원인, host delivery 성공 여부. 현재 소스에 Activity quota → context widget reopen 실패라는 직접 경로는 없다.
