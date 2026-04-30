package template

import (
	"regexp"
	"strings"
)

// 仅支持 ${VAR} 与 ${VAR:-default} 两种占位符, 没有引入完整模板引擎.
// 选择故意保守 — 模板内容是 YAML, 完整模板语法很容易破坏 YAML 结构.
//
//   ${VAR}                 -> vars["VAR"], 缺失时尝试 fallback (例如凭证库)
//                              仍未命中再走 :-default 或空串.
//   ${VAR:-default}        -> vars["VAR"], 缺失或为空时使用 default.
//   ${alias.field}         -> 先按完整名 (alias.field) 查 vars; 未命中再调 fallback,
//                              凭证回填器会按 alias 找凭证再取 field (见 cert_resolver.go).
//   ${alias.type.field}    -> Drone/Woodpecker 风格三段式, 行为同上, 中间段交给 fallback 校验.
var placeholderRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_-]*(?:\.[A-Za-z_][A-Za-z0-9_-]*)*)(?::-([^}]*))?\}`)

// Fallback 在 vars 没命中时被调用; 返回非空字符串视为命中.
// 调用方常见实现为 NewCertificateFallback (按 ${VAR} 名查凭证库).
type Fallback func(name string) string

// Render 把 yaml 中的 ${VAR}/${VAR:-default} 占位符按 vars 替换. 兼容旧调用,
// 不做凭证回填. 新代码请用 RenderWithFallback.
func Render(yaml string, vars map[string]string) string {
	return RenderWithFallback(yaml, vars, nil)
}

// RenderWithFallback 在 vars 缺失时再调一次 fallback. 替换发生在 spec.Parse
// 之前, 因此模板可在 image / commands / env value 等任意字符串字段使用占位符,
// 包括 commands 列表的元素 (替换的是整段 YAML 文本).
//
// 注意: vars 与 fallback 返回值的匹配都是大小写敏感.
func RenderWithFallback(yaml string, vars map[string]string, fallback Fallback) string {
	if yaml == "" {
		return yaml
	}
	return placeholderRegex.ReplaceAllStringFunc(yaml, func(match string) string {
		groups := placeholderRegex.FindStringSubmatch(match)
		if len(groups) == 0 {
			return match
		}
		name := groups[1]
		if value, ok := vars[name]; ok && value != "" {
			return value
		}
		if fallback != nil {
			if v := fallback(name); v != "" {
				return v
			}
		}
		// :-default 子表达式存在时使用默认值, 否则整体替换为空串
		if len(groups) > 2 {
			return groups[2]
		}
		return ""
	})
}

// MissingPlaceholders 返回 yaml 中没有被 vars 满足的占位符名称, 用于
// 触发流水线前给前端 / 日志一个明确的错误消息. 带 :-default 的占位符
// 不视为缺失.
func MissingPlaceholders(yaml string, vars map[string]string) []string {
	return MissingPlaceholdersWithFallback(yaml, vars, nil)
}

// MissingPlaceholdersWithFallback 与 MissingPlaceholders 同语义, 但额外把 fallback
// 命中也算作"已满足", 避免预览页把已能从凭证回填的变量误标红.
func MissingPlaceholdersWithFallback(yaml string, vars map[string]string, fallback Fallback) []string {
	if yaml == "" {
		return nil
	}
	matches := placeholderRegex.FindAllStringSubmatch(yaml, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, groups := range matches {
		if len(groups) < 2 {
			continue
		}
		name := strings.TrimSpace(groups[1])
		if name == "" {
			continue
		}
		if len(groups) > 2 && groups[2] != "" {
			continue
		}
		if value, ok := vars[name]; ok && value != "" {
			continue
		}
		if fallback != nil && fallback(name) != "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
