# 1.1.8102 기존 설정 보존 설치 실패

## 판단

**관리자 Core 모드의 기존 설치 업그레이드에서 stable tray 예약 작업이 Installer trial을 허용받지 못하는 구현 결함을 확인했다.** 1.1.8102에 포함된 실제 tray shim으로 독립 재현했다. 기존 설정 손상이나 Activity quota를 원인으로 볼 증거는 없다. `recovered abandoned runtime lock`은 정상 복구 안내이며 직접 실패가 아니다.

원래 Activity 현장 조사 A–G의 원본 머신 로그를 재확인한 것은 아니다. 여기의 현장 증거는 사용자가 실행한 Setup의 **현재 PC 설치 로그**다. 후속 사용자 요청으로 아래 1.1.8103 수정과 신규 설치파일을 준비하며, 운영 설치/복구/재시작은 수행하지 않는다.

## 현장 증거

- 실행한 Setup: `D:/Engineering/release/agentdock-1.1.8102-50ae55c3/release/AgentDockSetup-amd64.exe`, source `50ae55c3f78f2044238056183d672fa817c59506`.
- 설치 로그: `%LOCALAPPDATA%/AgentDock/logs/installer/setup-20261002-215826-178.log`. 2026-10-02 21:54:05 KST에 기존 1.1.8100 감지. 검증·payload 복사·설정/skill 준비 통과, `service_start` 120,359 ms 뒤 `start_failed`.
- 실제 오류는 versioned 1.1.8102 Core의 `service start`가 `http://127.0.0.1:8765/healthz`를 기다리다가 실패한 것이다. Native Setup launcher exit 1은 이를 전달한 상위 오류다.
- Core log `%LOCALAPPDATA%/AgentDock/logs/agentdock.err.log`의 마지막 기록은 설치 전 21:53:52 KST. 신규 Core 초기화 기록은 발견되지 않았다.
- 설치 transaction id `7382437152e34b4390a34f0ba01d5e61`. 조회 시점 `install/transaction.json`은 `rolled_back`, active pointer는 `v1.1.8100`, versions에는 8100만 있었다. OS adapter의 최종 abandon 기록이 원래 start_failed를 덮었으므로 최초 실패는 설치 로그를 기준으로 읽는다.
- 조회 시점 AgentDock 예약 작업의 action은 stable `bin/agentdock-tray.exe --run-core-task --runtime-root <runtime>`였다. 프로세스 조회에서 AgentDock Core를 찾지 못했다. rollback으로 재등록된 작업의 LastRun 결과만으로 실패 당시 작업 실행 이력을 확정하지 않는다. TaskScheduler Operational log는 꺼져 있었고 Setup 임시 폴더는 종료 시 삭제되어 당시 shim stderr는 없다.

## 소스 인과 경로

1. `scripts/install/install.ps1`의 기존 설치 + Setup + RegisterStartup 경로는 Engine이 commit 전 trial 상태에서 Core를 시작/검증하도록 한다. fresh Setup은 no-start/skip-health 후 commit 및 별도 시작이라 이 경로와 다르다.
2. `TaskAdminService.cs:566,582`의 `ElevatedCoreArguments`는 예약 작업을 `--run-core-task --runtime-root "..."`로 등록한다. 실제 조회한 작업도 이 action이다.
3. `internal/desktopruntime/service_windows.go:82`의 `startCore`는 elevated 모드에서 이 예약 작업을 실행한다.
4. stable GUI shim `cmd/agentdock-shim/main_windows.go:82`는 `resolveActiveWithRecovery(..., !tray && coreLaunchRequiresParentLifetime(args))`를 호출한다. tray task이면 **allowInstallerHost=false**다. 동일 파일의 child Job lifetime 판정은 tray task를 인식하지만 그 이전 trial 입장 판정은 인식하지 않는다.
5. `resolveActiveWithRecovery:204`는 allow=true일 때만 `liveInstallerTrial`로 matching transaction id/version/root/phase와 실제 점유 중인 Installer lock을 검사한다. false이면 이 정상 소유자 검사를 건너뛰고 **selfupdate**의 `update/transaction.json`을 읽는다. Setup은 **install**의 transaction을 사용하므로, update journal 없는 정상 upgrade도 `active generation is still a trial without an authorized live installer host; refusing ordinary launch`로 종료한다.
6. generation WPF task host와 Core의 `service launch-core`에 도달하기 전에 종료하여 Core log가 생기지 않는다. 부모 `startCore`는 task RunEx 요청 이후 상태/종료 원인을 읽지 않고 2분 health를 기다린다. 그 결과 실제 진입 거부가 “건강 검사 실패”로만 전달된다.

`cmd/agentdock-shim/task_core_host_windows.go:65`의 별도 native task host도 현재 `resolveActiveWithRecovery`에 live Installer 허용을 전달하지 않는다. 단순히 예약 작업 argument를 `--task-core-host`로 바꾸는 것만으로 이 결함이 해결되지는 않는다.

## 독립 재현과 확신의 범위

실제 candidate payload의 `agentdock-tray-shim.exe`를 명시적 임시 fixture의 `bin/agentdock-tray.exe`로 복사했다. 임시 active pointer와 install transaction을 동일 id, 8102 target, start phase, 일치하는 root로 작성하고 .NET FileStream `FileShare.None`으로 Installer transaction lock을 점유했다. 운영 runtime/설정/credential/Task Scheduler에 쓰지 않았다.

- fixture: `%TEMP%/agentdock-installer-trial-repro-0e57f10d48f7435ab4fb5a2ababa99c2` (stdout/stderr 및 JSON 보존).
- binary SHA256: `289541ffab5c7b2ed93d232ca378f641bbcd4f9b67009ae648edd26f7f82cd97`.
- 인자: `--run-core-task --runtime-root <fixture>`.
- 결과: **exit code 1**, 위의 trial admission 오류. 임시 fixture에서 Core, UI, 예약 작업을 시작하지 않고 진입점에서 차단됐다.
- 기존 `TestInstallerTrialRequiresLiveMatchingOwner`도 실행하여 matching start/health의 승인과 잘못된 id/root/phase 및 일반 진입의 거부가 정상임을 확인했다. 즉 guard 자체보다 **예약 작업 caller가 guard에 전달하는 역할 판정의 불일치**가 문제다. 로그 `%TEMP%/agentdock-install-failure-trial-diagnosis.log`.

실제 설치 당시 shim stderr가 남지 않아 그 프로세스의 정확한 exit 메시지를 직접 읽은 것은 아니다. 다만 실제 설정/action, 정확한 shipped binary의 동일 조건 재현, 설치 전까지만 있는 Core log, 120초 health 실패가 같은 경로로 설명된다. installer 전체 재실행·isolated runner 수용은 하지 않았다.

## 수정 경계

- P0: 기존 task와 호환되는 정확한 Core task 진입을 canonical owner에서 식별하고, matching live Installer의 id/version/root/start·health phase/lock 확인을 통과한 경우만 target을 시작하도록 연결한다. 모든 tray/명령에 trial을 허용하거나 조기 commit하는 우회는 금지다.
- P0 회귀: 실제 stable tray task argument를 포함한 trial admission, 잘못된 root/id/target/phase, abandoned lock, 일반 background/UI/management 거부, 기존 committed task 시작을 확인한다. 현재 service generation 시험은 standard 모드 + 이미 응답 중인 가짜 health 서버라 elevated task admission 결함을 검출하지 못한다.
- P1: task/host 조기 종료 진단을 보존하고 health timeout에 연결한다. 정상 요청 수락과 Core 준비 완료를 구분한다.
- 배포: 수정한 새 bytes는 8102 후보를 덮지 않고 새 버전(다음 후보 1.1.8103)에 배정한다. 기존 Setup 파일/실패 증거를 보존한다. 설치/업그레이드 acceptance는 실제 isolated runner/VM에서만 수행한다. Actions 차단 유지.

현재 로그만으로 사용자 데이터 손실은 확인되지 않았고 설정을 삭제할 근거도 없다. active pointer rollback을 Core 정상 실행으로 오해하지 않아야 한다. 운영 복구/재시작은 이번 조사에서 수행하지 않았다.

## 1.1.8103 수정

`installerHostEntry`가 기존 tray 예약 작업의 정확한 인자 세 개와 absolute runtime root가 stable shim root와 일치하는지 확인한 뒤 기존 live Installer 검사를 요청한다. Core service-host의 기존 경로는 유지한다. 별도 기존 native task host도 executable/root 확인 후 같은 admission을 요청한다. `liveInstallerTrial`의 id/version/root/start·health/lock 검사, 일반 tray·management 거부, task 등록 문자열과 WPF host 및 Job lifetime은 유지한다. 조기 commit과 설정 이관/삭제는 없다.

`installer_host_windows_test.go`는 실제 production GUI/CUI shim을 빌드하고 generation 자식만 유한 native fixture로 대체한다. 이는 installer/Task Scheduler acceptance가 아니다. start/health/committed/native task의 target 도달 및 실제 자식 exit 7 보존, owner가 종료된 trial, 잘못된 id/root/phase, 일반 background/UI 및 management에 끼워 넣은 task flag 거부와 active pointer 불변을 검증한다. 기존의 보안 거부 단언은 유지한다.

### 소스 최종 검증

- `go test ./... -count=1`: shim(실제 런처 fixture 포함), installer, desktopruntime, Activity, app, MCP 및 scripts 시험 통과. 전체 suite는 기존 기준선에서도 재현한 두 실패 때문에 실패 상태다. `TestManagedTaskFirstPage1000`은 2초 목표 대비 5,144.8866 ms, `TestSearchTextRGExitCodes`는 예상 `SEARCH_FAILED` 대비 사전 검증 `INVALID_REGEX`다. 실패 단언/로그를 유지했고 이번 설치 수정과 무관한 코드와 시험은 바꾸지 않았다. 로그 `%TEMP%/agentdock-8103-go-final.log`.
- Go formatting, `go vet ./...`, `verify-version v1.1.8103`, `git diff --check` 통과.
- Release desktop build 경고·오류 0, pure policy 923 assertions, 한국어 6,942 assertions, native 계약 32+40+99 assertions 통과. 실제 예약 작업이나 UI·설치·권한 변경은 실행하지 않았다.
- 한국어 첫 실행은 desktop 동시 빌드의 공유 obj 잠금(CS2012)으로 실패했고, 재시도 한 번은 잘못 입력한 프로젝트 경로(MSB1009)로 실패했다. 순차 실행과 실제 프로젝트 경로로 통과했다. 세 로그 `agentdock-8103-localization-final.log`, `agentdock-8103-localization-recheck.log`, `agentdock-8103-localization-recheck-2.log`를 보존한다. 제품 코드로 우회하지 않았다.
- WPF offscreen, 실제 isolated Setup/기존 설정 보존 업그레이드·rollback, Linux/race 수용은 미실행이다. 새 파일은 로컬 candidate이며 GitHub 게시와 게시 자산 재다운로드는 별개로 미실행이다.

최종 산출물 identity는 clean clone 빌드와 payload 확인 후 추가한다.
