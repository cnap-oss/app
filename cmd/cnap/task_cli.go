package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/cnap-oss/app/internal/storage"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func buildTaskCommands(logger *zap.Logger) *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Task 관리 명령어",
		Long:  "Controller를 통한 Task 생성, 조회, 상태 변경 기능을 제공합니다.",
	}

	// task create
	var createPrompt string
	var forceCreate bool
	taskCreateCmd := &cobra.Command{
		Use:   "create <agent-name> <task-id>",
		Short: "새로운 Task 생성",
		Long:  "특정 Agent에 새로운 Task를 생성합니다. --prompt 옵션으로 초기 프롬프트를 설정할 수 있습니다.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskCreate(logger, args[0], args[1], createPrompt, forceCreate)
		},
	}
	taskCreateCmd.Flags().StringVarP(&createPrompt, "prompt", "p", "", "Task 초기 프롬프트")
	taskCreateCmd.Flags().BoolVarP(&forceCreate, "force", "f", false, "기존 Task가 있으면 삭제 후 생성")

	// task list
	taskListCmd := &cobra.Command{
		Use:   "list <agent-name>",
		Short: "Task 목록 조회",
		Long:  "특정 Agent의 모든 Task 목록을 조회합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskList(logger, args[0])
		},
	}

	// task view
	taskViewCmd := &cobra.Command{
		Use:   "view <task-id>",
		Short: "Task 상세 정보 조회",
		Long:  "특정 Task의 상세 정보를 조회합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskView(logger, args[0])
		},
	}

	// task update-status
	taskUpdateStatusCmd := &cobra.Command{
		Use:   "update-status <task-id> <status>",
		Short: "Task 상태 변경",
		Long:  "Task의 상태를 변경합니다. (pending, running, completed, failed, canceled)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskUpdateStatus(logger, args[0], args[1])
		},
	}

	// task cancel
	taskCancelCmd := &cobra.Command{
		Use:   "cancel <task-id>",
		Short: "Task 취소",
		Long:  "실행 중인 Task를 취소합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskCancel(logger, args[0])
		},
	}

	// task run
	taskRunCmd := &cobra.Command{
		Use:   "run <task-id>",
		Short: "Task 실행",
		Long:  "생성된 Pending 상태의 Task를 실행합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskRun(logger, args[0])
		},
	}

	// task send
	taskSendCmd := &cobra.Command{
		Use:   "send <task-id>",
		Short: "Task 실행 트리거",
		Long:  "Task의 메시지를 전송하고 실행을 트리거합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskSend(logger, args[0])
		},
	}

	// task add-message
	taskAddMessageCmd := &cobra.Command{
		Use:   "add-message <task-id> <message>",
		Short: "Task에 메시지 추가",
		Long:  "Task에 새로운 메시지를 추가합니다. 실행은 트리거하지 않습니다.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskAddMessage(logger, args[0], args[1])
		},
	}

	// task messages
	taskMessagesCmd := &cobra.Command{
		Use:   "messages <task-id>",
		Short: "Task 메시지 목록 조회",
		Long:  "Task에 추가된 메시지 목록을 조회합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskMessages(logger, args[0])
		},
	}

	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskViewCmd)
	taskCmd.AddCommand(taskUpdateStatusCmd)
	taskCmd.AddCommand(taskCancelCmd)
	taskCmd.AddCommand(taskRunCmd)
	taskCmd.AddCommand(taskSendCmd)
	taskCmd.AddCommand(taskAddMessageCmd)
	taskCmd.AddCommand(taskMessagesCmd)

	return taskCmd
}

func runTaskCreate(logger *zap.Logger, agentName, taskID, prompt string, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	// Task 생성 시도
	err = ctrl.CreateTask(ctx, agentName, taskID, prompt)

	// UNIQUE constraint 에러 확인
	if err != nil && contains(err.Error(), "UNIQUE constraint failed") {
		if force {
			// --force 플래그 사용 시 기존 Task 조회 후 삭제
			fmt.Printf("⚠ Task '%s'가 이미 존재합니다. 삭제 후 재생성합니다.\n", taskID)

			// 기존 Task 정보 조회
			existingTask, getErr := ctrl.GetTaskInfo(ctx, taskID)
			if getErr == nil {
				fmt.Printf("  기존 Task 정보: Agent=%s, Status=%s\n", existingTask.AgentID, existingTask.Status)
			}

			// 기존 Task 삭제 (soft delete)
			deleteErr := ctrl.DeleteTask(ctx, taskID)
			if deleteErr != nil {
				return fmt.Errorf("기존 task 삭제 실패: %w", deleteErr)
			}

			// 다시 생성 시도
			err = ctrl.CreateTask(ctx, agentName, taskID, prompt)
			if err != nil {
				return fmt.Errorf("task 재생성 실패: %w", err)
			}
		} else {
			// --force 없이 중복 시 사용자에게 안내
			fmt.Printf("❌ Task '%s'가 이미 존재합니다.\n", taskID)
			fmt.Printf("\n다음 옵션을 사용하세요:\n")
			fmt.Printf("  1. 다른 Task ID 사용: ./bin/cnap task create %s <new-task-id>\n", agentName)
			fmt.Printf("  2. 기존 Task 삭제 후 생성: ./bin/cnap task create %s %s --force\n", agentName, taskID)
			fmt.Printf("  3. 기존 Task 확인: ./bin/cnap task view %s\n", taskID)
			return fmt.Errorf("task ID 중복")
		}
	} else if err != nil {
		return fmt.Errorf("task 생성 실패: %w", err)
	}

	if prompt != "" {
		fmt.Printf("✓ Task '%s' 생성 완료 (Agent: %s, Prompt: %s)\n", taskID, agentName, truncateString(prompt, 50))
	} else {
		fmt.Printf("✓ Task '%s' 생성 완료 (Agent: %s)\n", taskID, agentName)
	}
	return nil
}

// contains는 문자열에 부분 문자열이 포함되어 있는지 확인합니다.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runTaskList(logger *zap.Logger, agentName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	tasks, err := ctrl.ListTasksByAgent(ctx, agentName)
	if err != nil {
		return fmt.Errorf("task 목록 조회 실패: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Printf("Agent '%s'에 등록된 Task가 없습니다.\n", agentName)
		return nil
	}

	// 테이블 형식 출력
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TASK ID\tSTATUS\tCREATED\tUPDATED")
	_, _ = fmt.Fprintln(w, "-------\t------\t-------\t-------")

	for _, task := range tasks {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			task.TaskID,
			task.Status,
			task.CreatedAt.Format("2006-01-02 15:04"),
			task.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
	_ = w.Flush()

	return nil
}

func runTaskView(logger *zap.Logger, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	task, err := ctrl.GetTaskInfo(ctx, taskID)
	if err != nil {
		return fmt.Errorf("task 조회 실패: %w", err)
	}

	// 상세 정보 출력
	fmt.Printf("=== Task 정보: %s ===\n\n", task.TaskID)
	fmt.Printf("Task ID:     %s\n", task.TaskID)
	fmt.Printf("Agent ID:    %s\n", task.AgentID)
	fmt.Printf("상태:        %s\n", task.Status)
	if task.Prompt != "" {
		fmt.Printf("프롬프트:    %s\n", task.Prompt)
	}
	fmt.Printf("생성일:      %s\n", task.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("수정일:      %s\n", task.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

func runTaskUpdateStatus(logger *zap.Logger, taskID, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	// 상태 검증
	validStatuses := []string{
		storage.TaskStatusPending,
		storage.TaskStatusRunning,
		storage.TaskStatusCompleted,
		storage.TaskStatusFailed,
		storage.TaskStatusCanceled,
	}

	isValid := false
	for _, s := range validStatuses {
		if status == s {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("유효하지 않은 상태: %s (사용 가능: %v)", status, validStatuses)
	}

	if err := ctrl.UpdateTaskStatus(ctx, taskID, status); err != nil {
		return fmt.Errorf("task 상태 변경 실패: %w", err)
	}

	fmt.Printf("✓ Task '%s' 상태 변경: %s\n", taskID, status)
	return nil
}

func runTaskCancel(logger *zap.Logger, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	if err := ctrl.CancelTask(ctx, taskID); err != nil {
		return fmt.Errorf("task 취소 실패: %w", err)
	}

	fmt.Printf("✓ Task '%s' 취소 완료\n", taskID)
	return nil
}

func runTaskRun(logger *zap.Logger, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	if err := ctrl.ExecuteTask(ctx, taskID); err != nil {
		return fmt.Errorf("task 실행 실패: %w", err)
	}

	fmt.Printf("✓ Task '%s' 실행 시작\n", taskID)
	fmt.Printf("  상태 확인: cnap task view %s\n", taskID)
	return nil
}

func runTaskSend(logger *zap.Logger, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	if err := ctrl.SendMessage(ctx, taskID); err != nil {
		return fmt.Errorf("task 실행 실패: %w", err)
	}

	fmt.Printf("✓ Task '%s' 실행이 트리거되었습니다.\n", taskID)
	return nil
}

func runTaskAddMessage(logger *zap.Logger, taskID, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	if err := ctrl.AddMessage(ctx, taskID, "user", message); err != nil {
		return fmt.Errorf("메시지 추가 실패: %w", err)
	}

	fmt.Printf("✓ Task '%s'에 메시지가 추가되었습니다.\n", taskID)
	return nil
}

func runTaskMessages(logger *zap.Logger, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	messages, err := ctrl.ListMessages(ctx, taskID)
	if err != nil {
		return fmt.Errorf("메시지 목록 조회 실패: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("Task '%s'에 메시지가 없습니다.\n", taskID)
		return nil
	}

	// 테이블 형식 출력
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "INDEX\tROLE\tFILE PATH\tCREATED")
	_, _ = fmt.Fprintln(w, "-----\t----\t---------\t-------")

	for _, msg := range messages {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			msg.ConversationIndex,
			msg.Role,
			truncateString(msg.FilePath, 40),
			msg.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	_ = w.Flush()

	return nil
}
