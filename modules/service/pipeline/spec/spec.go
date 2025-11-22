package spec

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PipelineSpec represents the parsed pipeline definition extracted from YAML.
type PipelineSpec struct {
	Name      string
	Workspace string
	Steps     []StepSpec
}

// StepSpec describes a single build step.
type StepSpec struct {
	Name       string
	Image      string
	Commands   []string
	Secrets    []string
	Env        map[string]string
	Settings   map[string]any
	Volumes    []string
	Privileged bool
	Kind       StepKind
	Approval   *ApprovalSpec
	Conditions *StepConditions
}

type StepKind string

const (
	StepKindCommands StepKind = "commands"
	StepKindApproval StepKind = "approval"
)

type ApprovalSpec struct {
	Message   string
	Approvers []string
	Timeout   int64
	Strategy  string
}

type StepConditions struct {
	Branches []string
}

// Parse parses a pipeline YAML definition and returns a PipelineSpec.
// The parser focuses on the subset of the Woodpecker/Drone schema used by our UI:
func Parse(yamlContent string) (*PipelineSpec, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &root); err != nil {
		return nil, fmt.Errorf("解析流水线 YAML 失败: %w", err)
	}

	if len(root.Content) == 0 {
		return nil, fmt.Errorf("流水线配置为空")
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("流水线配置格式无效")
	}

	spec := &PipelineSpec{}

	for i := 0; i < len(doc.Content); i += 2 {
		key := strings.ToLower(strings.TrimSpace(doc.Content[i].Value))
		value := doc.Content[i+1]

		switch key {
		case "name":
			spec.Name = strings.TrimSpace(value.Value)
		case "workspace":
			spec.Workspace = strings.TrimSpace(value.Value)
		case "steps":
			steps, err := parseSteps(value)
			if err != nil {
				return nil, err
			}
			spec.Steps = steps
		}
	}

	if len(spec.Steps) == 0 {
		return nil, fmt.Errorf("流水线未定义任何步骤")
	}

	return spec, nil
}

func parseSteps(node *yaml.Node) ([]StepSpec, error) {
	switch node.Kind {
	case yaml.MappingNode:
		return parseMappingSteps(node)
	case yaml.SequenceNode:
		return parseSequenceSteps(node)
	default:
		return nil, fmt.Errorf("steps 必须为 mapping 或 sequence 结构")
	}
}

func parseMappingSteps(node *yaml.Node) ([]StepSpec, error) {
	steps := make([]StepSpec, 0, len(node.Content)/2)

	for i := 0; i < len(node.Content); i += 2 {
		stepName := strings.TrimSpace(node.Content[i].Value)
		stepBody := node.Content[i+1]

		if stepName == "" {
			return nil, fmt.Errorf("发现空的步骤名称")
		}

		var decoded struct {
			Image      string            `yaml:"image"`
			Commands   []string          `yaml:"commands"`
			Secrets    []string          `yaml:"secrets"`
			Env        map[string]string `yaml:"env"`
			Settings   map[string]any    `yaml:"settings"`
			Volumes    []string          `yaml:"volumes"`
			Privileged bool              `yaml:"privileged"`
			When       map[string]any    `yaml:"when"`
			// allow singular/plural spellings
			Certificate  yaml.Node `yaml:"certificate"`
			Certificates yaml.Node `yaml:"certificates"`
		}
		if err := stepBody.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("解析步骤 %q 失败: %w", stepName, err)
		}

		extraSecrets, err := collectCertificateAliases(&decoded.Certificate, &decoded.Certificates)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 certificate 字段失败: %w", stepName, err)
		}

		approvalSpec, err := extractApprovalSpec(decoded.Settings)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的审批配置失败: %w", stepName, err)
		}
		conditions, err := parseStepConditions(decoded.When)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 when 条件失败: %w", stepName, err)
		}

		image := strings.TrimSpace(decoded.Image)
		kind := StepKindCommands
		if approvalSpec != nil {
			kind = StepKindApproval
		} else {
			if image == "" {
				return nil, fmt.Errorf("步骤 %q 缺少镜像定义", stepName)
			}
			if len(decoded.Commands) == 0 && decoded.Settings == nil && len(decoded.Volumes) == 0 && !decoded.Privileged {
				return nil, fmt.Errorf("步骤 %q 未提供 commands", stepName)
			}
		}

		stepSettings := decoded.Settings
		if approvalSpec != nil {
			stepSettings = nil
		}

		steps = append(steps, StepSpec{
			Name:       stepName,
			Image:      image,
			Commands:   decoded.Commands,
			Secrets:    sanitizeSecrets(append(decoded.Secrets, extraSecrets...)),
			Env:        sanitizeEnvMap(decoded.Env),
			Settings:   stepSettings,
			Volumes:    sanitizeVolumes(decoded.Volumes),
			Privileged: decoded.Privileged,
			Kind:       kind,
			Approval:   approvalSpec,
			Conditions: conditions,
		})
	}

	return steps, nil
}

func parseSequenceSteps(node *yaml.Node) ([]StepSpec, error) {
	steps := make([]StepSpec, 0, len(node.Content))

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("steps 序列元素必须为 mapping 结构")
		}
		var decoded struct {
			Name         string            `yaml:"name"`
			Image        string            `yaml:"image"`
			Commands     []string          `yaml:"commands"`
			Secrets      []string          `yaml:"secrets"`
			Env          map[string]string `yaml:"env"`
			Settings     map[string]any    `yaml:"settings"`
			Volumes      []string          `yaml:"volumes"`
			Privileged   bool              `yaml:"privileged"`
			When         map[string]any    `yaml:"when"`
			Certificate  yaml.Node         `yaml:"certificate"`
			Certificates yaml.Node         `yaml:"certificates"`
		}
		if err := item.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("解析 steps 条目失败: %w", err)
		}
		name := strings.TrimSpace(decoded.Name)
		if name == "" {
			return nil, fmt.Errorf("steps 序列中的条目缺少 name 字段")
		}
		extraSecrets, err := collectCertificateAliases(&decoded.Certificate, &decoded.Certificates)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 certificate 字段失败: %w", name, err)
		}

		approvalSpec, err := extractApprovalSpec(decoded.Settings)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的审批配置失败: %w", name, err)
		}

		conditions, err := parseStepConditions(decoded.When)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 when 条件失败: %w", name, err)
		}

		image := strings.TrimSpace(decoded.Image)
		kind := StepKindCommands
		if approvalSpec != nil {
			kind = StepKindApproval
		} else {
			if image == "" {
				return nil, fmt.Errorf("步骤 %q 缺少镜像定义", name)
			}
			if len(decoded.Commands) == 0 && decoded.Settings == nil && len(decoded.Volumes) == 0 && !decoded.Privileged {
				return nil, fmt.Errorf("步骤 %q 未提供 commands", name)
			}
		}

		stepSettings := decoded.Settings
		if approvalSpec != nil {
			stepSettings = nil
		}

		steps = append(steps, StepSpec{
			Name:       name,
			Image:      image,
			Commands:   decoded.Commands,
			Secrets:    sanitizeSecrets(append(decoded.Secrets, extraSecrets...)),
			Env:        sanitizeEnvMap(decoded.Env),
			Settings:   stepSettings,
			Volumes:    sanitizeVolumes(decoded.Volumes),
			Privileged: decoded.Privileged,
			Kind:       kind,
			Approval:   approvalSpec,
			Conditions: conditions,
		})
	}

	return steps, nil
}

func parseStepConditions(raw map[string]any) (*StepConditions, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var conditions StepConditions
	for key, value := range raw {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "branch", "branches":
			branches, err := normalizeConditionValues(value)
			if err != nil {
				return nil, err
			}
			if len(branches) > 0 {
				conditions.Branches = branches
			}
		}
	}
	if len(conditions.Branches) == 0 {
		return nil, nil
	}
	return &conditions, nil
}

func normalizeConditionValues(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return []string{trimmed}, nil
		}
		return nil, nil
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("when.branch 数组仅支持字符串")
			}
			if trimmed := strings.TrimSpace(str); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	default:
		return nil, fmt.Errorf("when.branch 必须为字符串或字符串数组")
	}
}

func sanitizeSecrets(secrets []string) []string {
	if len(secrets) == 0 {
		return nil
	}
	out := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if trimmed := strings.TrimSpace(secret); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func collectCertificateAliases(nodes ...*yaml.Node) ([]string, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	result := make([]string, 0)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Kind == 0 {
			continue
		}
		switch node.Kind {
		case yaml.ScalarNode:
			value := strings.TrimSpace(node.Value)
			if value != "" {
				result = append(result, value)
			}
		case yaml.SequenceNode:
			for _, child := range node.Content {
				if child.Kind != yaml.ScalarNode {
					return nil, fmt.Errorf("certificate 列表包含非字符串值")
				}
				value := strings.TrimSpace(child.Value)
				if value != "" {
					result = append(result, value)
				}
			}
		default:
			return nil, fmt.Errorf("certificate 字段必须是字符串或字符串数组")
		}
	}
	return result, nil
}

func sanitizeVolumes(volumes []string) []string {
	if len(volumes) == 0 {
		return nil
	}
	out := make([]string, 0, len(volumes))
	for _, v := range volumes {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeEnvMap(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	clean := make(map[string]string, len(env))
	for key, value := range env {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		clean[trimmedKey] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func extractApprovalSpec(settings map[string]any) (*ApprovalSpec, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	typeValue, ok := settings["type"]
	if !ok {
		return nil, nil
	}
	typeString := strings.ToLower(strings.TrimSpace(fmt.Sprint(typeValue)))
	if typeString != "approval" {
		return nil, nil
	}

	spec := &ApprovalSpec{
		Strategy: "any",
	}

	if message, ok := settings["message"]; ok {
		spec.Message = strings.TrimSpace(fmt.Sprint(message))
	}

	if strategy, ok := settings["approval_strategy"]; ok {
		normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(strategy)))
		if normalized == "all" {
			spec.Strategy = "all"
		} else if normalized != "" {
			spec.Strategy = normalized
		}
	}

	if rawApprovers, ok := settings["approvers"]; ok {
		parsed, err := parseStringSlice(rawApprovers)
		if err != nil {
			return nil, fmt.Errorf("approvers: %w", err)
		}
		spec.Approvers = parsed
	}

	if timeout, ok := settings["approval_timeout"]; ok {
		parsedTimeout, err := parseDurationSeconds(timeout)
		if err != nil {
			return nil, fmt.Errorf("approval_timeout: %w", err)
		}
		spec.Timeout = parsedTimeout
	}

	return spec, nil
}

func parseStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, nil
		}
		return []string{trimmed}, nil
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					out = append(out, trimmed)
				}
			default:
				if str := strings.TrimSpace(fmt.Sprint(typed)); str != "" {
					out = append(out, str)
				}
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported type %T", value)
	}
}

func parseDurationSeconds(value any) (int64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, nil
		}
		num, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, err
		}
		return num, nil
	default:
		parsed := strings.TrimSpace(fmt.Sprint(value))
		if parsed == "" {
			return 0, nil
		}
		num, err := strconv.ParseInt(parsed, 10, 64)
		if err != nil {
			return 0, err
		}
		return num, nil
	}
}
