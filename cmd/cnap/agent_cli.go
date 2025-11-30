package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/text/unicode/norm"
)

// normalizeInput은 입력 문자열을 유니코드 NFC(Normalization Form Canonical Composition)로 정규화합니다.
// 이는 조합 중인 한글 문자를 완성된 음절로 변환하여 PostgreSQL에서 올바르게 처리되도록 합니다.
// 또한 단독 자음/모음(U+1100-U+11FF, U+3131-U+318E)을 제거합니다.
func normalizeInput(s string) string {
	// NFC 정규화 적용
	normalized := norm.NFC.String(s)

	// 한글 자음/모음 범위 필터링
	var filtered []rune
	for _, r := range normalized {
		// 한글 자음/모음 영역: U+1100-U+11FF (한글 자모), U+3131-U+318E (호환 자모)
		if (r >= 0x1100 && r <= 0x11FF) || (r >= 0x3131 && r <= 0x318E) {
			// 단독 자음/모음은 건너뛰기
			continue
		}
		filtered = append(filtered, r)
	}

	return string(filtered)
}

func buildAgentCommands(logger *zap.Logger) *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent 관리 명령어",
		Long:  "Controller를 통한 Agent 생성, 조회, 수정, 삭제 기능을 제공합니다.",
	}

	// agent create
	agentCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "새로운 Agent 생성",
		Long:  "대화형 입력을 통해 새로운 Agent를 생성합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentCreate(logger)
		},
	}

	// agent list
	agentListCmd := &cobra.Command{
		Use:   "list",
		Short: "Agent 목록 조회",
		Long:  "생성된 모든 Agent의 목록을 조회합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentList(logger)
		},
	}

	// agent view
	agentViewCmd := &cobra.Command{
		Use:   "view <agent-name>",
		Short: "Agent 상세 정보 조회",
		Long:  "특정 Agent의 상세 정보를 조회합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentView(logger, args[0])
		},
	}

	// agent delete
	agentDeleteCmd := &cobra.Command{
		Use:   "delete <agent-name>",
		Short: "Agent 삭제",
		Long:  "특정 Agent를 삭제합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentDelete(logger, args[0])
		},
	}

	// agent edit
	agentEditCmd := &cobra.Command{
		Use:   "edit <agent-name>",
		Short: "Agent 정보 수정",
		Long:  "대화형 입력을 통해 특정 Agent의 정보를 수정합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentEdit(logger, args[0])
		},
	}

	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentViewCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	agentCmd.AddCommand(agentEditCmd)

	return agentCmd
}

func runAgentCreate(logger *zap.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	reader := bufio.NewReader(os.Stdin)

	// 대화형 입력
	fmt.Print("Agent 이름: ")
	name, _ := reader.ReadString('\n')
	name = normalizeInput(strings.TrimSpace(name))

	fmt.Print("설명: ")
	description, _ := reader.ReadString('\n')
	description = normalizeInput(strings.TrimSpace(description))

	// Provider 선택
	fmt.Print("프로바이더 (opencode/gemini/claude/openai/xai) [opencode]: ")
	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "opencode"
	}

	// Provider별 추천 모델 표시
	var modelHint string
	switch provider {
	case "gemini":
		modelHint = "gemini-3-pro"
	case "claude":
		modelHint = "claude-sonnet-4-5"
	case "openai":
		modelHint = "gpt-5.1"
	case "xai":
		modelHint = "grok-code"
	default:
		modelHint = "claude-sonnet-4-5"
	}

	fmt.Printf("모델 (예: %s): ", modelHint)
	model, _ := reader.ReadString('\n')
	model = normalizeInput(strings.TrimSpace(model))

	// API Key 확인 및 설정
	if err := ensureAPIKey(provider); err != nil {
		return fmt.Errorf("API 키 설정 실패: %w", err)
	}

	fmt.Print("프롬프트 (역할 정의): ")
	prompt, _ := reader.ReadString('\n')
	prompt = normalizeInput(strings.TrimSpace(prompt))

	// 입력 검증
	if err := ctrl.ValidateAgent(name); err != nil {
		return fmt.Errorf("유효하지 않은 Agent 이름: %w", err)
	}

	// Agent 생성
	if err := ctrl.CreateAgent(ctx, name, description, provider, model, prompt); err != nil {
		return fmt.Errorf("agent 생성 실패: %w", err)
	}

	fmt.Printf("✓ Agent '%s' 생성 완료 (Provider: %s, Model: %s)\n", name, provider, model)
	return nil
}

// ensureAPIKey는 provider에 필요한 API 키가 설정되어 있는지 확인하고, 없으면 입력받습니다.
func ensureAPIKey(provider string) error {
	var envKey string
	switch provider {
	case "opencode":
		envKey = "OPEN_CODE_API_KEY"
	case "gemini":
		envKey = "GEMINI_API_KEY"
	case "claude":
		envKey = "ANTHROPIC_API_KEY"
	case "openai":
		envKey = "OPENAI_API_KEY"
	case "xai":
		envKey = "XAI_API_KEY"
	default:
		return fmt.Errorf("지원하지 않는 프로바이더: %s", provider)
	}

	// 환경 변수에 이미 설정되어 있는지 확인
	if os.Getenv(envKey) != "" {
		fmt.Printf("✓ %s가 환경 변수에서 발견되었습니다.\n", envKey)
		return nil
	}

	// 환경 변수에 없으면 입력받기
	fmt.Printf("⚠ %s가 설정되지 않았습니다.\n", envKey)
	fmt.Printf("API Key를 입력하세요 (Enter를 누르면 건너뛰기): ")

	reader := bufio.NewReader(os.Stdin)
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	if apiKey != "" {
		// 입력받은 API 키를 환경 변수로 설정
		if err := os.Setenv(envKey, apiKey); err != nil {
			return fmt.Errorf("환경 변수 설정 실패: %w", err)
		}
		fmt.Printf("✓ %s가 설정되었습니다.\n", envKey)
	} else {
		fmt.Printf("⚠ API Key가 설정되지 않았습니다. Task 실행 시 필요합니다.\n")
	}

	return nil
}

func runAgentList(logger *zap.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	agents, err := ctrl.ListAgentsWithInfo(ctx)
	if err != nil {
		return fmt.Errorf("agent 목록 조회 실패: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("등록된 Agent가 없습니다.")
		return nil
	}

	// 테이블 형식 출력
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSTATUS\tMODEL\tDESCRIPTION\tCREATED")
	_, _ = fmt.Fprintln(w, "----\t------\t-----\t-----------\t-------")

	for _, agent := range agents {
		desc := agent.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			agent.Name,
			agent.Status,
			agent.Model,
			desc,
			agent.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	_ = w.Flush()

	return nil
}

func runAgentView(logger *zap.Logger, agentName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	agent, err := ctrl.GetAgentInfo(ctx, agentName)
	if err != nil {
		return fmt.Errorf("agent 조회 실패: %w", err)
	}

	// 상세 정보 출력
	fmt.Printf("=== Agent 정보: %s ===\n\n", agent.Name)
	fmt.Printf("이름:        %s\n", agent.Name)
	fmt.Printf("상태:        %s\n", agent.Status)
	fmt.Printf("프로바이더:  %s\n", agent.Provider)
	fmt.Printf("모델:        %s\n", agent.Model)
	fmt.Printf("설명:        %s\n", agent.Description)
	fmt.Printf("프롬프트:\n%s\n\n", agent.Prompt)
	fmt.Printf("생성일:      %s\n", agent.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("수정일:      %s\n", agent.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

func runAgentDelete(logger *zap.Logger, agentName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	// 확인 메시지
	fmt.Printf("Agent '%s'을(를) 삭제하시겠습니까? (y/N): ", agentName)
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "y" && confirm != "yes" {
		fmt.Println("취소되었습니다.")
		return nil
	}

	if err := ctrl.DeleteAgent(ctx, agentName); err != nil {
		return fmt.Errorf("agent 삭제 실패: %w", err)
	}

	fmt.Printf("✓ Agent '%s' 삭제 완료\n", agentName)
	return nil
}

func runAgentEdit(logger *zap.Logger, agentName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	ctrl, cleanup, err := newController(logger)
	if err != nil {
		return fmt.Errorf("컨트롤러 초기화 실패: %w", err)
	}
	defer cleanup()

	// 기존 정보 조회
	agent, err := ctrl.GetAgentInfo(ctx, agentName)
	if err != nil {
		return fmt.Errorf("agent 조회 실패: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)

	// 현재 정보 표시 및 새 값 입력
	fmt.Printf("설명 (현재: %s): ", agent.Description)
	description, _ := reader.ReadString('\n')
	description = normalizeInput(strings.TrimSpace(description))
	if description == "" {
		description = agent.Description
	}

	fmt.Printf("프로바이더 (현재: %s) [opencode/gemini/claude/openai/xai]: ", agent.Provider)
	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = agent.Provider
	}

	fmt.Printf("모델 (현재: %s): ", agent.Model)
	model, _ := reader.ReadString('\n')
	model = normalizeInput(strings.TrimSpace(model))
	if model == "" {
		model = agent.Model
	}

	fmt.Printf("프롬프트 (현재: %s): ", agent.Prompt)
	prompt, _ := reader.ReadString('\n')
	prompt = normalizeInput(strings.TrimSpace(prompt))
	if prompt == "" {
		prompt = agent.Prompt
	}

	// Agent 수정
	if err := ctrl.UpdateAgent(ctx, agentName, description, provider, model, prompt); err != nil {
		return fmt.Errorf("agent 수정 실패: %w", err)
	}

	fmt.Printf("✓ Agent '%s' 수정 완료\n", agentName)
	return nil
}
