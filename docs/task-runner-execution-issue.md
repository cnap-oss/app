# Task 실행 실패 원인 및 해결안

## 현상
- `OPEN_CODE_API_KEY=... ./bin/cnap task run task-002` 실행 시 에러:
  - `runner not found for task: task-002`
- Task 생성(`task create`) 직후 다른 CLI 프로세스에서 실행(`task run`)하면 발생.

## 원인
- `RunnerManager`는 프로세스 단위 싱글톤이며, 메모리에 runner 인스턴스를 보관.
- `task create` 시 runner가 생성되지만, 커맨드가 종료되면 프로세스 메모리와 함께 runner도 사라짐.
- `task run`은 새 프로세스에서 시작되어 `RunnerManager`가 빈 상태이므로 runner를 찾지 못함.

## 해결 방향
1) **단일 서비스 프로세스에서 runner 유지 (권장 장기안)**
   - `cnap start` 등 장기 실행 프로세스에서 runner를 생성·보관.
   - CLI는 gRPC/HTTP/IPC로 해당 프로세스에 명령을 전달.
   - 장점: runner 라이프사이클이 프로세스 종료에 영향받지 않음. 배포 시 서비스 구조가 명확.
   - 단점: RPC 계층 추가 개발 필요.

2) **실행 시 runner를 재생성 (단기 패치)**
   - `controller.ExecuteTask`에서 `runnerManager.GetRunner`가 `nil`이면:
     - Task와 Agent 정보를 DB에서 로드
     - `runnerManager.CreateRunner(taskID, agentInfo, logger)`로 재생성 후 실행
   - 장점: CLI 단일 실행 프로세스에서도 바로 동작.
   - 단점: Runner 내부 상태를 유지하지 못하고 매 실행 시 새로 만들어야 함.

## 제안
- 빠른 해소가 필요하면 **2안** 적용으로 즉시 CLI 사용성 확보.
- 장기적으로 서비스형 구조가 필요하면 **1안**으로 runner를 서버 프로세스에서 관리하고, CLI는 RPC 호출자로 단순화.***
