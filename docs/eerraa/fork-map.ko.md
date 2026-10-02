# AgentDock Workbench fork 맵

구현·마이그레이션 에이전트용 현재 상태 지도다. 경위와 수치 기록은 두지 않는다. 경위가 필요하면 `git log`를 본다. 이 문서와 코드가 다르면 코드가 기준이며, 이 문서를 고친다.

고출력 MCP의 Activity 저장 구조, 승인된 capture 정책과 후속 개선안은 [Activity payload 검토 핸드오버](activity-payload-review.ko.md)에 있다. 현장 증거와 소스 확인, 미확인 host 경계를 구분하며 미구현 대안과 실제 구현 범위를 구분한다.

## 0. 정체성

| 항목 | 값 |
|---|---|
| 저장소 | origin `eerraa/AgentDock-Workbench` (push 대상). upstream·workbench `A-m-o-r-F-a-t-i/AgentDock-Workbench` (읽기 전용, push 금지). uvwt `uvwt/agentdock` (원조, 참고용) |
| GitHub CI 정책 | 이 fork의 repository Actions permissions는 `enabled=false`다. 자동·수동 Actions 빌드 모두 차단한다. workflow 파일은 유지하고 로컬 clean-clone 빌드는 계속 사용한다. 사용자의 별도 지시 없이 Actions를 다시 켜지 않는다. 확인: `gh api repos/eerraa/AgentDock-Workbench/actions/permissions`; 종료된 실행·실패 기록은 삭제하지 않는다 |
| 소스 기준선 | upstream `v1.1.8` 프리릴리스 `4bd778d4077bbe58cfe19e4abb777f660694377b`를 main에 병합한 상태. 이전 기준선은 `v1.1.7` `b367eaab` |
| 이력 구조 | upstream main의 커밋 전부(뒤진 커밋 0) → upstream main 밖의 기준 태그 병합(`v1.1.8`, 원본 SHA 그대로) → fork 커밋을 기능·결함 수리 단위로 둔다. 2026-09-28 사용자 요청으로 한 번 재구성했다. 이전 이력은 게시 태그 `v1.1.8100`에만 남는다. 이 태그는 CI upgrade 기준 source를 붙잡으므로 옮기거나 지우지 않는다 |
| 버전 규칙 | 기본 `1.1.<upstream patch>100(+수정 번호)`. 한국어 보완은 사용자 지정 `1.1.8200`을 사용하며 upstream 기준선은 여전히 `1.1.8`이다. 현재 정식 latest `1.1.8100`, 공개 Pre-release `1.1.8200`. 게시된 fork 버전(`1.1.6100`, `1.1.8100`, `1.1.8200`)은 재사용하지 않는다 |
| 설치파일 소스 버전 | `1.1.8200`: clean-clone 로컬 x64 후보 빌드, 실제 Setup payload·한국어 리소스 확인, 사용자 요청의 GitHub Pre-release 공개와 Draft/공개 자산 재다운로드 검증 완료. 태그는 실제 빌드 source를 가리킨다. 범위·identity는 [한국어 보완 핸드오버](korean-presentation-completion.ko.md), [사용자 등록 절차](windows-release.ko.md)에 기록한다. `1.1.8101`·`1.1.8102`·`1.1.8103` 후보 bytes와 실패 기록은 보존한다. 실제 설치·isolated runner 수용은 미실행이다 |
| 버전 선언 위치 | `internal/buildinfo/buildinfo.go`, `desktop/windows/control-panel/AgentDock.ControlPanel.csproj`. `go run ./tools/release version`과 `verify-version v<ver>`로 확인 |
| 업데이트 | Setup으로만 한다. 트레이·창의 업데이트 항목은 fork Releases 페이지 안내만 하고, `agentdock update`는 `--local-archive`만 허용한다 |
| 배포 대상 | Windows x64 Setup만. arm64·macOS·Linux·Android 코드는 업스트림 그대로 두고 빌드하거나 게시하지 않는다 |
| 작업 방식 | 메인 워크트리 하나에서 `git switch -c fix/<주제>`(또는 `feat/`, `docs/`) → 검증 → main에 ff 또는 merge → origin에만 push. force push 금지, 추가 worktree와 증거 archive 금지 |

## 1. fork 변경 지도

v1.1.8 대비 fork 차이는 아래 영역뿐이다. 새 차이를 만들면 이 표에 행을 추가한다.

| 영역 | 소유 파일 | 불변식 | 회귀 시험 |
|---|---|---|---|
| 관리자 Core 설치 trial | `cmd/agentdock-shim/main_windows.go`의 `installerHostEntry`, 기존 native host의 root 검사 후 admission | 기존 `--run-core-task --runtime-root <stable root>`만 live Installer 검사를 요청한다. id/version/root/start·health/실제 lock 보호를 유지하고 일반 UI·management를 허용하지 않는다. 기존 task 등록 형식과 WPF host·Job lifetime 유지 | `installer_host_windows_test.go`: 실제 GUI shim → 유한 generation fixture, trial start/health·committed·native 진입, 잘못된 root/id/phase·owner 종료·UI 거부, child exit 보존. 실제 Setup GUI/CUI도 별도 fixture 확인. [상세](setup-upgrade-trial-failure.ko.md) |
| 내장 인터페이스 OFF 검증 | 기존 `internal/config/display.go`, `internal/mcp/presentation.go`, `response_additions.go`, 제어판 `MainWindow.Display.cs` (실행 코드 변경 없음) | hot reload는 도구 실행·외부 MCP 연결을 유지한다. host cache 채택은 unknown, 이전 URI는 30분 한정 안내 응답. ChatGPT 연결 Refresh 후 새 대화로 표시를 확인해야 한다 | `display_dynamic_test.go`: stateless HTTP, 갱신하지 않은 host 목록, in-flight OFF, 외부 성공/오류 보존. [판단·미확인 경계](embedded-interface-toggle.ko.md) |
| 배포 정체성 | `internal/selfupdate/update.go` `defaultReleaseAPI`; `scripts/install/install.ps1` `Get-ReleaseBaseUrl`; `packaging/windows/AgentDock.iss` `App*URL`; workflow `github.repository == 'eerraa/AgentDock-Workbench'` 조건; `packaging/windows/build-windows-release.ps1` build report `upstream_version/upstream_commit`; README.md/README.zh-CN.md·docs/releases/·windows-release.ko.md의 다운로드 안내 | 세 대상 실행 파일에 업스트림 소유자 주소(`a-m-o-r-f-a-t-i/`, 대소문자 무관)가 없다. build report의 기준선 커밋은 AGENTS.md와 같다. fork 다운로드는 origin Releases의 실제 Setup에 연결하며 upstream attribution은 보존한다 | `scripts/test/fork_windows_release_test.go`; 공개 자산 redownload와 SHA-256/API digest·공개 URL 검사 |
| Setup 전용 업데이트 | `cmd/agentdock/main.go` `errForkUpdateThroughSetup`; `cmd/agentdock/server.go` usage; `desktop/windows/control-panel/App.xaml.cs` `CheckForUpdatesAsync`, `_releasesPromptOpen` | 온라인 확인·다운로드는 네트워크·generation 접근 전에 거부한다. `internal/selfupdate` 패키지, `--local-archive`, 시작 시 데스크톱 복구, 업데이트 transaction 재개 UI는 유지한다 | `cmd/agentdock/main_test.go`, `scripts/test/desktop_windows_test.go` |
| 고정 rg 15.2.0 (windows/amd64) | 정의: `internal/bundledrg/` (`windows-amd64.json` pin). 사용: `internal/tool/file/search.go`, `search_rg_selection.go`. 전달: `install.ps1` `Get-AgentDockBundledRgPayloadPaths`와 `Assert-AgentDockBundledRgPayload`; `internal/installer/apply.go` `copyWindowsGenerationPayload`; `internal/selfupdate/release_payload_windows.go`, `desktop_update_windows.go`, `generation_windows.go`. 패키징: `packaging/windows/prepare-bundled-rg.ps1`, `build-windows-release.ps1`, `scripts/test/verify-windows-release-assets.ps1` | payload 경로는 `share/agentdock/bin/{manifest.json,rg.exe,COPYING,LICENSE-MIT,UNLICENSE}` 정확히 5개다. 부재는 legacy로 허용하고 부분·변조는 `ErrIntegrity`로 거부한다. x64 Setup은 부재도 거부한다. 검색은 실행 중인 바이너리 옆의 번들을 쓰며, 손상된 번들이 있으면 PATH rg로 내려가지 않는다. arm64 rg는 없다 | `internal/bundledrg/*_test.go`; `internal/tool/file/search_rg_*_test.go`; `internal/installer/component_delivery_windows_test.go`; `internal/selfupdate/component_archive_windows_test.go`, `release_payload_windows_test.go`; `scripts/ci/parallel-installer/test-setup-archive.ps1`; `scripts/test/test-bundled-rg-package.ps1` |
| 대형 Activity 이력 | `internal/activity/payload.go` (`CaptureHistoryPayload`); `internal/app/execution_dispatch.go`, `execution_payload.go`; `internal/config/display.go`; 제어판 `DisplayPreferenceService`, `ExecutionDialogs`, `ExecutionPayloadView` | normal은 redacted formatted JSON 256 KiB 초과를 최대 2 KiB preview-only로 journal에 보존한다. debug는 명시적 설정과 root admission snapshot을 사용하며 16 MiB/256 MiB 제한을 유지한다. 도구 결과·continuation source·기존 이력은 유지하고 Core를 재시작하지 않는다 | `payload_policy_test.go`, `activity_capture_test.go`(config/MCP), `execution_payload_policy_test.go`, `OutputPolicyTests.cs`; [상세 근거](activity-payload-review.ko.md) |
| Activity 기록 초기화 | `internal/activity/reset.go`, `store.go`; `internal/app/activity_reset.go`, `callObserved`, `RecordToolResponse`, `executePrepared`; `internal/httpx/execution_routes_overview.go`; 제어판 `ActivityHistoryService`, `ExecutionWindow.ActivityReset`, `ExecutionDialogs.Confirm` | 인증된 local management의 명시적 확인만 허용. 실행/최종 응답/명령/승인 대기 보호. 삭제 전에 단조 증가 replay floor commit, 회전으로 floor를 낮추지 않음. 최근 5분 blob grace와 실제 잔여 용량·부분 실패를 표시. 초기화 확인문은 화면 높이 안에서 자동 크기/스크롤을 허용. 작업·대화·권한·설정·프로젝트 불변; Core restart와 자동 GC 없음 | `reset_test.go`, `activity_reset_test.go`(app/HTTP), `ActivityResetContract.cs`, `ActivityResetDialogTests.cs`(runner 전용); [사용법·경계](activity-history-reset.ko.md) |
| 한국어: 제어판 | `desktop/windows/control-panel/Localization/` (`UiText`, `LocExtension`, `OwnedText`, `NativeDiagnosticText`, `ActivityText`); `Models/ExecutionModels.cs`; `Resources/UiStrings{,.zh-CN,.ko-KR}.resx` (각 1,171키); XAML `{local:Loc Key}`, C# `UiText.Get/Format` | 세 언어의 키 집합과 `{n}` 자리표시가 같다. 기존 zh-CN 값은 보존한다. synthetic 그룹 ID·빈 ID+미귀속 marker·fallback title_source로만 제품 제목을 번역한다. 승인 scope는 완전한 기존 제품 template만 표시에서 번역하며 경로·세션 ID와 사용자 rule·reviewer 응답·기술 진단을 보존한다. 기존 보안·실행 의미 불변 | pure policy 및 `scripts/test/testdata/activity-center` `--localization-only`: 실제 모델 초기화/갱신, 동명 사용자 제목, management hash/status/외부 tool 보호, 승인 selector 보존. WPF layout은 runner 전용 |
| 한국어: 서버 생성 문구 | `internal/activity/localized_text.go`, `management_text.go` (canonical recognizer), `calls.go` (read-only old-data 경계); `internal/app/execution_localized_text.go`, `execution_completion.go`, `execution_runtime.go`, `conversation_sidebar.go`; `internal/config/display.go`, `internal/app/display_settings.go` | 새 management 기록은 일반·예약 쓰기 모두 hash metadata를 보존한다. 이전 local management는 원문/label/tool/status가 일치할 때 조회 사본에만 metadata를 보충하며 journal과 cached projection·future metadata 불변. batch message_text도 skipped/failed 상태를 보존한다. 사용자 라벨·외부 결과를 번역하지 않는다 | activity `management_text_test.go`, app `execution_localized_text_test.go` (실제 persisted reserved events), HTTP `execution_localization_test.go`; 기존 redaction/unknown/failure 보호 유지 |
| 한국어: 브라우저·MCP Apps·Setup | `internal/httpx/oauth.go` `writeAuthorizeBrowserError`, `oauth_authorize_text.go`, `oauth_authorize_page.*`, `status_page.go`; `internal/mcp/apps_localization.go`, `apps_ko-KR.json`; `packaging/windows/languages/Korean.isl`(+`korean-source.json`, `LICENSE-InnoSetup.txt`), `includes/messages.iss`, `includes/code.iss` | 비밀번호, 동의, 등록 콜백 정확 일치 검사를 유지한다. `Korean.isl`은 공식 바이트 그대로 둔다(`.gitattributes -text`) | `internal/httpx/browser_localization_test.go`, `internal/mcp/apps_localization_test.go`, `scripts/test/install_windows_korean_test.go` |
| 프로젝트 ACL | `internal/config/config.go` `Normalize` | Windows에서는 `AgentDockHome`만 `EnsurePrivate` 대상이다. 사용자 프로젝트 DACL은 건드리지 않는다 | `internal/config/workspace_acl_windows_test.go` |
| MCP discovery 형태 | `internal/mcp/client/protocol.go` sending middleware | SDK 필터보다 먼저 null Tool과 비객체 inputSchema를 `MCP_INVALID_RESPONSE`로 거부하고 last-good catalog를 유지한다 | `internal/mcp/client/discovery_null_test.go`, `discovery_schema_test.go` |
| Core 역할·root 한정 | `internal/desktopruntime/service_process_role_windows.go`, `service_windows.go`, `service_process_windows.go` `queryProcessHandlePath` | 같은 handle로 이미지 경로, Core 역할, runtime root를 확인한다. 열 수 없는 같은 이름 프로세스(`ERROR_ACCESS_DENIED`)는 선택하지 않는다. controller만 있으면 Running으로 보지 않는다 | `internal/desktopruntime/service_process_role_windows_test.go` (+`testdata/core-role`) |
| 예약 작업 복구 | `desktop/windows/control-panel/Services/TaskAdminService.cs`, `TaskDefinitionPolicy.cs` | schema 1 기록은 읽기 전용 호환이며, source 복원 뒤에 재개한다. 구형 `powershell.exe -File start-agentdock.ps1` action은 기존 task·복구 경계에서만 식별한다. 새 task는 stable tray action만 쓴다 | `desktop/windows/control-panel-native-tests/Task*.cs` (`--security-contract-only`는 로컬에서 실행 가능) |
| Setup 롤백 예약 작업 | `install.ps1` 롤백의 `$taskTransactionStarted` 조건, `-RuntimeRoot $runtimeDir` | task transaction이 시작되지 않았으면 `Stop-ScheduledTask`를 하지 않는다 | `scripts/test/test-windows-installer-task-ownership.ps1`, `fork_windows_release_test.go` |
| Funnel 검증 시간 초과 | `internal/desktopruntime/tailscale_probe_windows.go` | 시간 초과와 취소도 pending으로 기록한다. 이전 `verified_at`으로 Ready를 재구성하지 않는다 | `tailscale_probe_windows_test.go` |
| 제어판 loopback | `desktop/windows/control-panel/Services/LoopbackHttp.cs` (`ActivityClient`와 `RuntimeService`가 공유) | Bearer 요청에 시스템 프록시와 리다이렉트를 쓰지 않는다 | `desktop/windows/control-panel-tests/LoopbackHttpTests.cs` |
| 외부 MCP 권한 범위 | `internal/app/execution_dispatch.go` `permissionAction`; `internal/permission/policy.go` `validRuleAction` | `mcp_tool_call`의 규칙·승인 action은 `<server>:<tool>`이다. 승인 시 "이 작업공간 허용"은 그 도구 하나만 허용한다. action이 빈 기존 규칙은 전처럼 모든 외부 MCP 호출에 일치한다. 다른 도구의 action 형식은 그대로다 | `internal/permission/permission_test.go` `TestDynamicMCPWorkspaceRuleIsScopedToOneTool`, `internal/app/execution_permission_profile_test.go` |
| 외부 MCP 이진 결과 | `internal/mcp/server.go` `dynamicMCPToolEnvelope`, `withoutDuplicatedBinary` | 이미지·오디오 `data`와 리소스 `blob`은 `content`에 한 번만 싣는다. `structuredContent` 사본에는 종류, MIME, `base64_length`만 남긴다 | `internal/mcp/server_test.go` `TestToolEnvelope*DynamicMCP*` |
| CUA 데스크톱 제어 플러그인 | `plugins/cua-driver/` (`plugin.json`, `mcp.json`, `skills/cua-desktop/SKILL.md`) | 비Heavy Agent Plugins 패키지다. MCP 멤버 `cua-driver`는 사용자 데몬에 `cua-driver mcp --socket \\.\pipe\cua-driver`로 붙으며, 요소 캐시·세션·권한 수준은 데몬에 남는다. 데몬(예약 작업 `\cua-driver-serve` 또는 `cua-driver serve`)이 없으면 탐색이 실패한다. 스킬은 관찰→동작→검증, 화면 내용 불신, 비가역 작업 확인, 승인 대기 후 재관찰을 규정한다. CLI 업데이트 뒤 이전 데몬이 남으면 계약 버전이 어긋나므로, 스킬은 데몬 재시작(`cua-driver stop` → `autostart kick`)을 사용자에게 맡기고 `mcp_manage` refresh로 다시 붙게 한다. 에이전트가 별도 데몬을 띄우는 우회는 금지다. Setup이 자동 공급한다(다음 행). 수동 설치와 cua-driver 전제는 `plugins/cua-driver/README.md` | `internal/plugin/cua_driver_plugin_test.go`; 실제 cua-driver 탐색(0.30.2, 59개 도구, 데몬 재시작 후 refresh 복구)은 로컬 수동 확인 |
| 번들 플러그인 공급 | `plugins/`(저장소) → build가 payload `share/agentdock/plugins/`로 복사; `install.ps1` 선택 목록과 `Invoke-AgentDockBundledPluginBootstrap`(설치 확정 후, 터널 전); `agentdock plugin bootstrap --bundle --home`; `internal/plugin/bundled.go` `ProvisionBundled`; 검증기의 packaged Core bootstrap | 처음이면 설치, Setup이 공급한 것은 버전이 바뀌면 갱신(켜기/끄기 유지), 사용자가 지웠거나 같은 이름을 직접 설치했으면 건드리지 않는다. 기록은 `plugins/.bundled.json`. 실패는 경고(`bundled-plugins-deferred`)이며 Core를 롤백하지 않는다 | `internal/plugin/bundled_test.go`, `cmd/agentdock/command_plugin_test.go`, `test-setup-archive.ps1`, `fork_windows_release_test.go` `TestWindowsSetupProvisionsBundledPluginsAfterCommit` |
| 설치 저널 소유자 | `internal/installer/journal_copy.go`; `journal_metadata_windows.go` `assignableBackupNativeMetadata` | 스냅숏·복원 사본에는 현재 토큰이 지정할 수 있는 소유자만 기록한다. 관리자 모드 AgentDock이 쓴 `Administrators` 소유 파일(`server-url.txt`, `logs`)을 일반 권한 Setup이 스냅숏하면 소유자만 현재 사용자로 기록하고 그룹·DACL·속성은 그대로 둔다. 관리자 권한 설치는 변화 없음 | `internal/installer/journal_metadata_windows_test.go` `TestUnassignableBackupOwnerKeepsAccessControl`; 실제 파일 재현은 로컬 수동 확인(CI는 관리자 권한이라 재현 불가) |
| 게시·수용 관문 | `.github/workflows/windows-package.yml` (resolve-source → linux-contracts → native-acceptance → build → publish); `workbench-acceptance.yml`; `scripts/test/verify-windows-release-assets.ps1` `Assert-WindowsReleaseAcceptance`; `test-windows-setup-e2e.ps1`, `test-windows-upgrade-isolated.ps1` | publish에는 workflow_dispatch, installation_tests, amd64, prerelease=false가 모두 필요하다. 버전은 1.1.8 초과이고 게시된 fork 버전이 아니어야 한다. 필수 한국어 시험은 5개다. upgrade 기준은 게시된 1.1.8100의 source `dc6c861`(태그 `v1.1.8100`)을 재빌드한 것이며, 게시된 fork 버전이어야 한다. `verification-scope.json`에는 실행하지 않은 단계를 그대로 기록한다 | `scripts/test/windows_publication_policy_test.go`, `upgrade_evidence_windows_test.go`, `windows_installer_invocation_test.go`, `ci_workflow_test.go` |

Setup receipt 공유 위반 대기는 업스트림 `internal/desktopruntime/setup_receipt_windows.go`가 대신한다. fork 시험 `setup_launcher_windows_test.go`는 유지한다.

## 2. 다음 upstream 병합 절차

1. `git fetch upstream --tags` → main에서 `feat/rebase-<ver>` → upstream main에 새 커밋이 있으면 `git merge --no-ff upstream/main`, 새 기준 태그가 upstream main 밖에 있으면 `git merge --no-ff --no-commit v<ver>`. upstream 커밋을 rebase나 cherry-pick으로 복제하지 않는다. 그래야 upstream 대비 뒤진 커밋이 0으로 남는다.
   fork 변경은 기능·결함 수리 하나당 커밋 하나로 둔다. 같은 주제의 보정 커밋은 push 전에 합친다.
2. 충돌은 양쪽을 보존한다. 업스트림 동작과 fork 불변식(1절)을 함께 만족시킨다. 1.1.8 병합 때 충돌한 곳은 workflow repository 조건, 제어판 csproj·XAML·ExecutionModels·InsertionPresentation·PermissionSettingsEditor·Sidebar, buildinfo, setup_launcher, oauth.go, selfupdate/update.go, AgentDock.iss, governance inventory, install.ps1이었다.
3. **조용히 빠지는 곳을 반드시 확인한다.** 충돌 없이 병합돼도 다음은 깨질 수 있다.
   - 새 payload 허용 목록이나 추출기가 `share/agentdock/bin`을 빠뜨리는지(1.1.8의 install.ps1과 selfupdate가 실제로 빠뜨렸다).
   - 업스트림이 새로 넣은 중국어 UI 문구. 제어판과 `SidebarResponseValidation` 같은 새 파일에 `\p{Han}` 리터럴이 있는지 grep한다. 찾으면 키로 옮기고 zh-CN 값은 원문 그대로 둔다.
   - 한국어로 실행되는 레이아웃 시험에 새로 들어온 중국어 단언(1.1.8의 `关闭`).
   - 업스트림 저장소 주소 변경, 새 workflow의 게시 권한, build report 기준선, 버전.
4. 병합 뒤 `git diff v<ver> HEAD`가 1절의 영역만 포함하는지 확인한다.
5. 3절의 검증을 전부 실행한 뒤 AGENTS.md의 기준선 커밋과 이 문서의 0절을 갱신한다.

## 3. 검증 명령

| 대상 | 명령 |
|---|---|
| Go 전체 | `GOTOOLCHAIN=go1.26.5 env -u AGENTDOCK_INSTRUCTIONS_FILE go test ./... -count=1`; `gofmt -l cmd internal scripts tools`; `go vet ./...` |
| 실제 rg | `pwsh packaging/windows/prepare-bundled-rg.ps1 -Destination <tmp>/rg-bundle` (캐시: `%TEMP%\agentdock-build-cache`) → `AGENTDOCK_TEST_RG_BUNDLE=<tmp>/rg-bundle go test -tags=bundled_rg_integration ./internal/bundledrg/ ./internal/selfupdate/ ./internal/installer/ ./internal/tool/file/` |
| 제어판 | `dotnet build` control-panel, control-panel-tests, control-panel-layout-tests (Release, 경고·오류 0) → `dotnet run --project desktop/windows/control-panel-tests/AgentDock.ControlPanel.Tests.csproj -c Release --no-build` |
| 한국어 | `dotnet run --project scripts/test/testdata/activity-center/ActivityCenterTests.csproj -c Release -- --localization-only` |
| Setup 스크립트 | `$env:RUNNER_TEMP=<tmp>; pwsh scripts/ci/parallel-installer/test-setup-archive.ps1` (powershell.exe 5.1도 실행); `pwsh scripts/test/test-install-windows.ps1 -StaticOnly` |
| native 계약 | `dotnet run --project desktop/windows/control-panel-native-tests/AgentDock.ControlPanel.NativeTests.csproj -c Release -- --security-contract-only`. 실제 예약 작업·NTFS fixture는 Actions에서 돌리거나, 명시적인 `--local-isolated`로만 돌린다 |
| Actions 전용 | WPF 레이아웃, 실제 Setup 설치·업그레이드·롤백. 현재 이 fork의 Actions 차단 정책으로 실행하지 않는다. 가드를 우회하거나 운영 PC의 Setup으로 대신하지 않는다 |

## 4. 로컬 빌드 (사용자가 요청할 때만)

1. 커밋된 HEAD를 새 일반 clone으로 받는다. linked worktree에서 빌드하면 `vcs.revision`이 빠져 검증기가 거부한다.
2. `$env:GOTOOLCHAIN='go1.26.5'`, `$env:PATH="D:\Engineering\.agentdock-build-tools\inno;$env:PATH"`.
3. `pwsh packaging/windows/build-windows-release.ps1 -Architectures amd64 -OutputDirectory <out> [-Candidate]`. 공식 cloudflared를 내려받아 Authenticode를 검사한다. 산출물은 `<out>/release/AgentDockSetup-amd64.exe`.
4. `scripts/test/verify-windows-release-assets.ps1`로 버전, commit, rg 5개 파일, checksum을 검사한다. 서명 상태는 unsigned다.
5. 게시하려면 head 커밋과 annotated 태그 메시지에 `[skip ci]`를 넣고, 태그 `v<ver>`, `docs/releases/v<ver>.md`를 준비한다. 운영 PC에서 Setup을 실행하지 않는다(사용자가 직접 한다).
6. draft로 올린다 → 다시 내려받아 로컬 게시 자산과 해시를 비교한다 → 공개한다 → 한 번 더 내려받아 확인한다. 정식 수용을 완료한 배포는 latest(prerelease 아님)로 공개한다. 미수용 후보의 다운로드를 사용자가 요청한 경우 검증 범위와 unsigned를 명시한 Pre-release, latest=false로 공개하고 빌드 시점 metadata를 보존한다. 절차는 [Windows 릴리즈 등록 안내](windows-release.ko.md)를 따른다. CI의 정식 수용 관문은 변경하지 않는다.

## 5. 제약·보류

| 구분 | 내용 |
|---|---|
| 알려진 제약 | `--local-archive`와 데스크톱 복구는 실행 중인 Core의 rg pin으로 payload를 검사하므로, rg pin 변경은 Setup으로만 배포한다. selfupdate의 구형 flat 이관(`PrepareWindowsLegacyGeneration`)은 rg를 복사하지 않는다. fork 설치로는 이 경로에 닿지 않는다. 시작 시 복구는 Core·Tray 버전이 다를 때만 Core 시작마다 GitHub API를 1회 조회하고, 아무것도 바꾸지 않는다. `RuntimeService`의 온라인 업데이트 메서드는 호출되지 않지만 업스트림 계약 시험 때문에 남아 있다. `rules` 모드에서 사용자가 승인한 호출의 결과는 모델에 돌아가지 않는다(모델은 다시 관찰해야 한다). ChatGPT가 MCP 이미지 결과를 모델에게 보여 주는지는 실제 연결로 확인해야 한다 |
| 설치 진단 한계 | Core task가 시작 전에 종료되어도 `startCore`는 120초 health 실패로만 표시한다. trial 진입 결함은 1.1.8103에서 수정하지만 task 조기 종료 stderr/status 수집 개선은 별도 P1이다. 실제 isolated Setup 수용은 Actions 차단과 구분해 미실행으로 기록한다. [증거·수정 경계](setup-upgrade-trial-failure.ko.md) |
| 게시 자산 | 로컬 게시는 `AgentDockSetup-amd64.exe`, `build-report.json`, `verification-scope.json`과 각 `.sha256`만 올린다(ZIP·`install.ps1` 제외). CI의 `windows-package.yml` 게시 경로와 검증기는 아직 10개 기준이다. CI로 게시하려면 먼저 맞춘다 |
| 보류 후보 (요청 시만) | 메인 창 활동 요약 주기 갱신, 비JSON health 수용, Core 정지 중 버전 표시 프로세스 반복, 상태 조회 실패의 '중지됨' 표시, 앱 밖 Tailscale 주소 변경 시 캐시 혼합, 생성 후 Job 할당 틈, 관리자 작업 UAC 재시도, tunnel configure 롤백, 폴더 보안 재설정 성능, 브라우저 시작 20초 제한 |
| upstream 1.1.8 이후 미반영 | PR #22 (`0591339a`: 시작 시 DACL이 이미 같으면 재설정 생략 + 시작 단계 시간 기록 → '폴더 보안 재설정 성능'과 겹침; `f1df7381`: 설치 스테이징 분리·launcher rollback journal; `813e7d5f`: 스트리밍 복사). PR #21: 설치 파일만 게시. 재현되는 것만 채택한다 |
| 제품 결함 아님 | 운영 로그의 30초 주기 `GET /hello` 404는 사용자 Edge에서 온 외부 요청이다 |

## 6. 환경 함정

- 이 PC의 세션 환경에 `AGENTDOCK_INSTRUCTIONS_FILE`이 존재하지 않는 파일로 설정돼 있다. 빼지 않으면 config·cmd 시험 6개가 가짜로 실패한다.
- `internal/taskstate` `TestManagedTaskFirstPage1000`의 2초 목표는 병렬·부하 상태에서 main에서도 실패한다. 단독으로 다시 확인한다.
- `core.autocrlf=true`라 작업 트리의 resx 값 줄바꿈은 CRLF다. 문자열 비교 시험은 `\r\n`과 `\n`을 모두 허용해야 한다.
- `install.ps1`은 PowerShell 5.1 호환과 ASCII만 허용한다.
- Bash 도구의 heredoc에서 백슬래시가 해석될 수 있다. 백슬래시가 들어가는 편집은 파일로 쓴 스크립트나 편집 도구로 한다.
