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
	Build      *BuildSpec
	Conditions *StepConditions
}

type StepKind string

const (
	StepKindCommands StepKind = "commands"
	StepKindApproval StepKind = "approval"
	StepKindBuild    StepKind = "build"
)

type ApprovalSpec struct {
	Message   string
	Approvers []string
	Timeout   int64
	Strategy  string
}

// BuildSpec 描述 kind=build 步骤的所有参数. 引擎会用这些字段在容器里跑
// moby/buildkit 的 daemonless 模式 (buildctl-daemonless.sh build ...) 直接
// 推送镜像, 不需要 dockerd. registry/username/password 缺省时, 引擎按
// step.certificate 找一个 docker 类型凭证自动回填.
type BuildSpec struct {
	Registry      string            `yaml:"registry,omitempty"`
	Repo          string            `yaml:"repo"`
	Username      string            `yaml:"username,omitempty"`
	Password      string            `yaml:"password,omitempty"`
	Dockerfile    string            `yaml:"dockerfile,omitempty"`
	Context       string            `yaml:"context,omitempty"`
	Tags          []string          `yaml:"tags,omitempty"`
	Platforms     []string          `yaml:"platforms,omitempty"`
	Push          *bool             `yaml:"push,omitempty"`
	BuildArgs     map[string]string `yaml:"build_args,omitempty"`
	Target        string            `yaml:"target,omitempty"`
	NoCache       bool              `yaml:"no_cache,omitempty"`
	BuildkitImage string            `yaml:"buildkit_image,omitempty"`
	Privileged    bool              `yaml:"privileged,omitempty"`
}

// StepConditions 是 step.when 的解析结果. 为兼容老代码 / task payload,
// 同时保留两种视图:
//   - 扁平字段 (Branches / Events / Paths / Statuses / Repos / Refs / ...) :
//     合并所有 when 条目里的对应值, 给只关心"出现过"的简单判定使用.
//   - Groups: 严格按 Woodpecker "list-of-mappings" 拆分, 每条目一个 group,
//     group 内子条件全 AND, 多个 group 之间 OR. runtime 优先使用 Groups
//     做精确判定.
//
// 当 when 写成单 mapping 时, Groups 长度为 1.
type StepConditions struct {
	Branches []string
	Events   []string
	Paths    []string
	Statuses []string

	Repos     []string
	Refs      []string
	Crons     []string
	Platforms []string
	Instances []string
	Evaluate  string
	Matrix    map[string]string

	BranchInclude []string
	BranchExclude []string

	PathInclude       []string
	PathExclude       []string
	PathIgnoreMessage string
	PathOnEmpty       *bool

	Groups []StepConditionGroup
}

// StepConditionGroup 是一个 when 条目内部的所有子条件 (AND).
type StepConditionGroup struct {
	Branches  []string
	Events    []string
	Statuses  []string
	Refs      []string
	Repos     []string
	Crons     []string
	Platforms []string
	Instances []string

	BranchInclude []string
	BranchExclude []string

	PathInclude       []string
	PathExclude       []string
	PathIgnoreMessage string
	PathOnEmpty       *bool

	Matrix   map[string]string
	Evaluate string
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
		return nil, fmt.Errorf("流水线配置格式无效: 顶层必须是 YAML 映射, 至少包含 steps:; 例如:\nsteps:\n  build:\n    image: alpine\n    commands:\n      - echo hi")
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

// ParseStepFragment 用于 kind=step 的步骤模板. 与 Parse 不同, 它接受三种
// 更宽松的顶层形态, 都规范化成 []StepSpec 返回:
//
//  1. 完整 mapping 含 steps:                与 Parse 一致, 复用 parseSteps.
//  2. 顶层 sequence (一个或多个步骤)        Woodpecker 的 `- name: clone\n  image: ...` 形态.
//  3. 顶层 mapping 但无 steps:, 含 name/image 等  视作单步骤的简写.
//
// 这样用户写 step 片段时不必每次都套一层 steps: 包裹, 也对齐 Woodpecker
// 文档示例. ResolveCompose 使用此函数, 同时模板 Create/UpdateDraft/Publish
// 在 kind=step 时也用它做校验.
func ParseStepFragment(yamlContent string) ([]StepSpec, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &root); err != nil {
		return nil, fmt.Errorf("解析步骤片段 YAML 失败: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("步骤片段为空")
	}
	doc := root.Content[0]

	switch doc.Kind {
	case yaml.SequenceNode:
		return parseSequenceSteps(doc)
	case yaml.MappingNode:
		// 顶层若包含 steps: 走完整解析, 与 Parse 行为一致.
		for i := 0; i < len(doc.Content); i += 2 {
			if strings.ToLower(strings.TrimSpace(doc.Content[i].Value)) == "steps" {
				return parseSteps(doc.Content[i+1])
			}
		}
		// 没有 steps: 但是单步骤简写 (有 name 或 image), 包成单元素 sequence 复用解析.
		hasStepish := false
		for i := 0; i < len(doc.Content); i += 2 {
			key := strings.ToLower(strings.TrimSpace(doc.Content[i].Value))
			switch key {
			case "name", "image", "commands", "settings", "kind", "build":
				hasStepish = true
			}
			if hasStepish {
				break
			}
		}
		if !hasStepish {
			return nil, fmt.Errorf("步骤片段未识别到任何步骤; 顶层应为 steps:、序列 (- name: ...)、或单步骤映射 (name: ... image: ...)")
		}
		seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{doc}}
		return parseSequenceSteps(seq)
	default:
		return nil, fmt.Errorf("步骤片段顶层必须是 mapping 或 sequence")
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
			Kind       string            `yaml:"kind"`
			Image      string            `yaml:"image"`
			Commands   []string          `yaml:"commands"`
			Secrets    []string          `yaml:"secrets"`
			Env        map[string]string `yaml:"env"`
			Settings   map[string]any    `yaml:"settings"`
			Volumes    []string          `yaml:"volumes"`
			Privileged bool              `yaml:"privileged"`
			When       yaml.Node         `yaml:"when"`
			// allow singular/plural spellings
			Certificate  yaml.Node  `yaml:"certificate"`
			Certificates yaml.Node  `yaml:"certificates"`
			Build        *BuildSpec `yaml:"build"`
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
		conditions, err := parseStepConditionsNode(&decoded.When)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 when 条件失败: %w", stepName, err)
		}

		image := strings.TrimSpace(decoded.Image)
		kindLower := strings.ToLower(strings.TrimSpace(decoded.Kind))
		buildSpec, err := finalizeBuildSpec(stepName, kindLower, decoded.Build, decoded.Commands, decoded.Settings, image)
		if err != nil {
			return nil, err
		}

		kind := StepKindCommands
		switch {
		case buildSpec != nil:
			kind = StepKindBuild
		case approvalSpec != nil:
			kind = StepKindApproval
		default:
			if image == "" {
				return nil, fmt.Errorf("步骤 %q 缺少镜像定义", stepName)
			}
			if len(decoded.Commands) == 0 && decoded.Settings == nil && len(decoded.Volumes) == 0 && !decoded.Privileged {
				return nil, fmt.Errorf("步骤 %q 未提供 commands", stepName)
			}
		}

		stepSettings := decoded.Settings
		if approvalSpec != nil || buildSpec != nil {
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
			Build:      buildSpec,
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
			Kind         string            `yaml:"kind"`
			Image        string            `yaml:"image"`
			Commands     []string          `yaml:"commands"`
			Secrets      []string          `yaml:"secrets"`
			Env          map[string]string `yaml:"env"`
			Settings     map[string]any    `yaml:"settings"`
			Volumes      []string          `yaml:"volumes"`
			Privileged   bool              `yaml:"privileged"`
			When         yaml.Node         `yaml:"when"`
			Certificate  yaml.Node         `yaml:"certificate"`
			Certificates yaml.Node         `yaml:"certificates"`
			Build        *BuildSpec        `yaml:"build"`
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

		conditions, err := parseStepConditionsNode(&decoded.When)
		if err != nil {
			return nil, fmt.Errorf("解析步骤 %q 的 when 条件失败: %w", name, err)
		}

		image := strings.TrimSpace(decoded.Image)
		kindLower := strings.ToLower(strings.TrimSpace(decoded.Kind))
		buildSpec, err := finalizeBuildSpec(name, kindLower, decoded.Build, decoded.Commands, decoded.Settings, image)
		if err != nil {
			return nil, err
		}

		kind := StepKindCommands
		switch {
		case buildSpec != nil:
			kind = StepKindBuild
		case approvalSpec != nil:
			kind = StepKindApproval
		default:
			if image == "" {
				return nil, fmt.Errorf("步骤 %q 缺少镜像定义", name)
			}
			if len(decoded.Commands) == 0 && decoded.Settings == nil && len(decoded.Volumes) == 0 && !decoded.Privileged {
				return nil, fmt.Errorf("步骤 %q 未提供 commands", name)
			}
		}

		stepSettings := decoded.Settings
		if approvalSpec != nil || buildSpec != nil {
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
			Build:      buildSpec,
			Conditions: conditions,
		})
	}

	return steps, nil
}

// parseStepConditionsNode 接受 Woodpecker 的两种 when 写法:
//   - 单个 mapping: when: { branch: main, event: push }     → 1 个 group
//   - mapping 列表: when: [{ branch: main }, { event: cron }] → N 个 group
//
// 每个 group 内部子条件全 AND, 多个 group 之间 OR (任一 group 命中即整个
// when 通过).
//
// 同时维护扁平的 StepConditions 字段 (各 group 的并集), 让简单"出现过即
// 通过"的旧逻辑也能继续工作 (例如基于扁平 Branches 的旧 task payload).
func parseStepConditionsNode(node *yaml.Node) (*StepConditions, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	var rawGroups []map[string]any
	switch node.Kind {
	case yaml.MappingNode:
		var single map[string]any
		if err := node.Decode(&single); err != nil {
			return nil, fmt.Errorf("when 解析失败: %w", err)
		}
		if len(single) > 0 {
			rawGroups = append(rawGroups, single)
		}
	case yaml.SequenceNode:
		for idx, item := range node.Content {
			if item.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("when 列表第 %d 项必须是 mapping", idx+1)
			}
			var entry map[string]any
			if err := item.Decode(&entry); err != nil {
				return nil, fmt.Errorf("when 列表第 %d 项解析失败: %w", idx+1, err)
			}
			if len(entry) > 0 {
				rawGroups = append(rawGroups, entry)
			}
		}
	default:
		return nil, fmt.Errorf("when 必须是 mapping 或 mapping 列表")
	}
	if len(rawGroups) == 0 {
		return nil, nil
	}

	conditions := &StepConditions{}
	for _, raw := range rawGroups {
		group, err := parseStepConditionGroup(raw)
		if err != nil {
			return nil, err
		}
		mergeFlatConditions(conditions, group)
		conditions.Groups = append(conditions.Groups, group)
	}
	return conditions, nil
}

func parseStepConditionGroup(raw map[string]any) (StepConditionGroup, error) {
	var group StepConditionGroup
	for key, value := range raw {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "branch", "branches":
			inc, exc, plain, err := normalizeIncludeExclude(value)
			if err != nil {
				return group, fmt.Errorf("when.branch: %w", err)
			}
			group.Branches = plain
			group.BranchInclude = inc
			group.BranchExclude = exc
		case "event", "events":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.event: %w", err)
			}
			group.Events = vals
		case "status", "statuses":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.status: %w", err)
			}
			group.Statuses = vals
		case "ref", "refs":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.ref: %w", err)
			}
			group.Refs = vals
		case "repo", "repos":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.repo: %w", err)
			}
			group.Repos = vals
		case "cron", "crons":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.cron: %w", err)
			}
			group.Crons = vals
		case "platform", "platforms":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.platform: %w", err)
			}
			group.Platforms = vals
		case "instance", "instances":
			vals, err := normalizeConditionValues(value)
			if err != nil {
				return group, fmt.Errorf("when.instance: %w", err)
			}
			group.Instances = vals
		case "evaluate":
			if s, ok := value.(string); ok {
				group.Evaluate = strings.TrimSpace(s)
			}
		case "matrix":
			if m, ok := value.(map[string]any); ok {
				out := make(map[string]string, len(m))
				for k, v := range m {
					out[k] = fmt.Sprintf("%v", v)
				}
				group.Matrix = out
			}
		case "path", "paths":
			inc, exc, plain, err := normalizeIncludeExclude(value)
			if err != nil {
				return group, fmt.Errorf("when.path: %w", err)
			}
			group.PathInclude = inc
			group.PathExclude = exc
			// path 字典还可能含 ignore_message / on_empty
			if m, ok := value.(map[string]any); ok {
				if msg, ok := m["ignore_message"].(string); ok {
					group.PathIgnoreMessage = strings.TrimSpace(msg)
				}
				if onEmpty, ok := m["on_empty"].(bool); ok {
					b := onEmpty
					group.PathOnEmpty = &b
				}
			}
			if len(plain) > 0 && len(inc) == 0 {
				// 普通字符串 / 数组形态走扁平 PathInclude 的语义
				group.PathInclude = plain
			}
		}
	}
	return group, nil
}

// mergeFlatConditions 把 group 字段并入扁平 StepConditions (去重+保序).
func mergeFlatConditions(dst *StepConditions, g StepConditionGroup) {
	dst.Branches = mergeStrings(dst.Branches, g.Branches)
	dst.Events = mergeStrings(dst.Events, g.Events)
	dst.Statuses = mergeStrings(dst.Statuses, g.Statuses)
	dst.Refs = mergeStrings(dst.Refs, g.Refs)
	dst.Repos = mergeStrings(dst.Repos, g.Repos)
	dst.Crons = mergeStrings(dst.Crons, g.Crons)
	dst.Platforms = mergeStrings(dst.Platforms, g.Platforms)
	dst.Instances = mergeStrings(dst.Instances, g.Instances)
	dst.BranchInclude = mergeStrings(dst.BranchInclude, g.BranchInclude)
	dst.BranchExclude = mergeStrings(dst.BranchExclude, g.BranchExclude)
	dst.PathInclude = mergeStrings(dst.PathInclude, g.PathInclude)
	dst.PathExclude = mergeStrings(dst.PathExclude, g.PathExclude)
	dst.Paths = mergeStrings(dst.Paths, g.PathInclude) // legacy 字段把 PathInclude 合并到 Paths
	if g.PathIgnoreMessage != "" {
		dst.PathIgnoreMessage = g.PathIgnoreMessage
	}
	if g.PathOnEmpty != nil {
		dst.PathOnEmpty = g.PathOnEmpty
	}
	if g.Evaluate != "" {
		dst.Evaluate = g.Evaluate
	}
	if len(g.Matrix) > 0 {
		if dst.Matrix == nil {
			dst.Matrix = map[string]string{}
		}
		for k, v := range g.Matrix {
			dst.Matrix[k] = v
		}
	}
}

func mergeStrings(dst, add []string) []string {
	if len(add) == 0 {
		return dst
	}
	seen := make(map[string]struct{}, len(dst)+len(add))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	out := dst
	for _, v := range add {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// normalizeIncludeExclude 处理 Woodpecker 的两种写法:
//   - branch: main             -> plain
//   - branch: [main, develop]  -> plain
//   - branch: { include: [...], exclude: [...] }
//
// 返回 (include, exclude, plain). 调用方根据自身语义合并.
func normalizeIncludeExclude(value any) ([]string, []string, []string, error) {
	if value == nil {
		return nil, nil, nil, nil
	}
	if m, ok := value.(map[string]any); ok {
		inc, err := normalizeConditionValues(m["include"])
		if err != nil {
			return nil, nil, nil, err
		}
		exc, err := normalizeConditionValues(m["exclude"])
		if err != nil {
			return nil, nil, nil, err
		}
		return inc, exc, nil, nil
	}
	plain, err := normalizeConditionValues(value)
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, nil, plain, nil
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
				return nil, fmt.Errorf("条件数组仅支持字符串值")
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
		return nil, fmt.Errorf("条件值必须为字符串或字符串数组")
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

// finalizeBuildSpec 校验并规范化一个 step 的 build 子块.
//
//   - kind 显式为 "build" 或 build 子块非 nil 时, 视作 build step.
//   - 与 commands / image / settings / approval 互斥: build step 由引擎指定
//     镜像并在容器内跑 buildctl-daemonless.sh, 用户不应再写其它执行方式.
//   - 默认 dockerfile=Dockerfile, context=., tags=[latest], platforms=[linux/amd64].
//   - repo 必填; registry/username/password 缺省时由 runtime 从 docker 凭证回填.
func finalizeBuildSpec(stepName, kind string, build *BuildSpec, commands []string, settings map[string]any, image string) (*BuildSpec, error) {
	requested := kind == "build" || build != nil
	if !requested {
		return nil, nil
	}
	if build == nil {
		return nil, fmt.Errorf("步骤 %q 声明 kind=build 但未提供 build: 子块", stepName)
	}
	if image != "" {
		return nil, fmt.Errorf("步骤 %q 是 build 步骤, 不应再设置 image (镜像由 build.buildkit_image 控制)", stepName)
	}
	if len(commands) > 0 {
		return nil, fmt.Errorf("步骤 %q 是 build 步骤, 不应再设置 commands", stepName)
	}
	if len(settings) > 0 {
		return nil, fmt.Errorf("步骤 %q 是 build 步骤, 不应再设置 settings (用 build: 子块)", stepName)
	}
	if strings.TrimSpace(build.Repo) == "" {
		return nil, fmt.Errorf("步骤 %q 的 build.repo 必填 (例: sixx/devsys)", stepName)
	}
	out := *build
	out.Repo = strings.TrimSpace(out.Repo)
	out.Registry = strings.TrimSpace(out.Registry)
	out.Username = strings.TrimSpace(out.Username)
	out.Password = strings.TrimSpace(out.Password)
	out.Dockerfile = strings.TrimSpace(out.Dockerfile)
	if out.Dockerfile == "" {
		out.Dockerfile = "Dockerfile"
	}
	out.Context = strings.TrimSpace(out.Context)
	if out.Context == "" {
		out.Context = "."
	}
	out.Target = strings.TrimSpace(out.Target)
	out.BuildkitImage = strings.TrimSpace(out.BuildkitImage)

	cleanedTags := make([]string, 0, len(out.Tags))
	for _, t := range out.Tags {
		t = strings.TrimSpace(t)
		if t != "" {
			cleanedTags = append(cleanedTags, t)
		}
	}
	if len(cleanedTags) == 0 {
		cleanedTags = []string{"latest"}
	}
	out.Tags = cleanedTags

	cleanedPlatforms := make([]string, 0, len(out.Platforms))
	for _, p := range out.Platforms {
		p = strings.TrimSpace(p)
		if p != "" {
			cleanedPlatforms = append(cleanedPlatforms, p)
		}
	}
	if len(cleanedPlatforms) == 0 {
		cleanedPlatforms = []string{"linux/amd64"}
	}
	out.Platforms = cleanedPlatforms

	if len(out.BuildArgs) > 0 {
		cleanedArgs := make(map[string]string, len(out.BuildArgs))
		for k, v := range out.BuildArgs {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			cleanedArgs[k] = v
		}
		if len(cleanedArgs) == 0 {
			cleanedArgs = nil
		}
		out.BuildArgs = cleanedArgs
	}
	return &out, nil
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
