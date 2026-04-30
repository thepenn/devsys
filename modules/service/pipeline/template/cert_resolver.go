package template

import (
	"context"
	"strings"

	"github.com/thepenn/devsys/model"
	systemsvc "github.com/thepenn/devsys/service/system"
)

// NewCertificateFallback 返回一个 Fallback, 把 ${VAR} / ${alias.field} / ${alias.type.field}
// 形式的占位符解析到 Certificate 仓库:
//
//	${alias}                  -> primaryCertValue(cert)
//	                                git    -> password (可作 token Bearer)
//	                                docker -> repo (registry URL)
//	${alias.field}            -> certFieldValue(cert, field)
//	                                docker: username|password|repo|registry(=repo)
//	                                git:    username|password|token(=password)
//	${alias.type.field}       -> 同上, 中间段必须匹配 cert.Type (大小写不敏感, 不匹配视作未命中)
//
// 闭包内带请求级缓存, 同一 Render / MissingPlaceholders 调用里同名变量
// 仅查一次 DB; 命中或失败都缓存.
//
// systemSvc 为 nil 时返回 nil fallback, 渲染器会回到原始行为 (无凭证回填).
func NewCertificateFallback(ctx context.Context, systemSvc *systemsvc.Service) Fallback {
	if systemSvc == nil {
		return nil
	}
	certCache := map[string]*model.Certificate{}
	resultCache := map[string]string{}
	return func(name string) string {
		if name == "" {
			return ""
		}
		if v, ok := resultCache[name]; ok {
			return v
		}
		segments := strings.Split(name, ".")
		alias := strings.TrimSpace(segments[0])
		if alias == "" {
			resultCache[name] = ""
			return ""
		}
		cert, ok := certCache[strings.ToLower(alias)]
		if !ok {
			loaded, err := systemSvc.GetCertificateWithSecretsByName(ctx, alias)
			if err != nil {
				loaded = nil
			}
			cert = loaded
			certCache[strings.ToLower(alias)] = cert
		}
		if cert == nil {
			resultCache[name] = ""
			return ""
		}
		var value string
		switch len(segments) {
		case 1:
			value = primaryCertValue(cert)
		case 2:
			value = certFieldValue(cert, segments[1])
		case 3:
			expectedType := strings.ToLower(strings.TrimSpace(segments[1]))
			actualType := strings.ToLower(strings.TrimSpace(cert.Type))
			if expectedType != "" && expectedType != actualType {
				value = ""
			} else {
				value = certFieldValue(cert, segments[2])
			}
		default:
			value = ""
		}
		resultCache[name] = value
		return value
	}
}

// primaryCertValue 把 Certificate 映射成单个字符串, 决定 ${VAR} 替换的值.
// 仅暴露常用类型的"主"字段; 复合字段访问请用 ${alias.field} 形式.
func primaryCertValue(cert *model.Certificate) string {
	if cert == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(cert.Type)) {
	case "git":
		if g, err := cert.AsGitCertificate(); err == nil && g != nil {
			return g.Password
		}
	case "docker":
		if d, err := cert.AsDockerCertificate(); err == nil && d != nil {
			return d.Repo
		}
	}
	return ""
}

// certFieldValue 暴露常用凭证字段, 字段名按惯例提供别名:
//
//	docker.repo == docker.registry      git.token == git.password
//
// 未识别字段返回 "" 而不是报错, 让上层走 :-default 或被标记为 missing.
func certFieldValue(cert *model.Certificate, field string) string {
	if cert == nil {
		return ""
	}
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(cert.Type)) {
	case "docker":
		d, err := cert.AsDockerCertificate()
		if err != nil || d == nil {
			return ""
		}
		switch field {
		case "username":
			return d.Username
		case "password":
			return d.Password
		case "repo", "registry":
			return d.Repo
		}
	case "git":
		g, err := cert.AsGitCertificate()
		if err != nil || g == nil {
			return ""
		}
		switch field {
		case "username":
			return g.Username
		case "password", "token":
			return g.Password
		}
	}
	return ""
}
