// Package i18n provides lightweight HTTP error message localisation.
//
// It uses the Accept-Language header to determine the target language and
// returns pre-defined translations for known error codes. If no translation
// is found the original (English) message is returned unmodified.
package i18n

import (
	"net/http"
	"strings"
)

// Lang represents a supported language.
type Lang string

const (
	LangEN   Lang = "en"
	LangZhCN Lang = "zh-CN"
)

// catalog maps lang → code → translated message.
var catalog = map[Lang]map[string]string{
	LangZhCN: {
		// generic
		"BAD_REQUEST":   "请求格式无效",
		"BAD_ID":        "无效的 ID 参数",
		"NOT_FOUND":     "资源未找到",
		"STORE_ERROR":   "存储错误",
		"INVALID_STATE": "当前状态不允许此操作",

		// project
		"MISSING_NAME":      "名称不能为空",
		"PROJECT_NOT_FOUND": "项目未找到",

		// resource
		"MISSING_KIND": "类型不能为空",
		"MISSING_URI":  "URI 不能为空",

		// agents
		"MISSING_LAUNCH_COMMAND": "启动命令不能为空",
		"MISSING_DRIVER_ID":      "驱动 ID 不能为空",
		"DRIVER_NOT_FOUND":       "驱动未找到",
		"PROFILE_NOT_FOUND":      "配置未找到",
		"DUPLICATE_DRIVER":       "驱动 ID 已存在",
		"DUPLICATE_PROFILE":      "配置 ID 已存在",
		"CAP_OVERFLOW":           "配置能力超出驱动最大能力",
		"DRIVER_IN_USE":          "驱动正被一个或多个配置引用",
		"INVALID_SKILLS":         "配置引用了无效或不存在的技能",

		// skills
		"SKILL_NOT_FOUND":  "技能未找到",
		"SKILL_EXISTS":     "技能已存在",
		"MISSING_REPO_URL": "仓库 URL 不能为空",

		// dag / template
		"DAG_GEN_UNAVAILABLE": "DAG 生成器未配置（需要 LLM）",
		"MISSING_ACTIONS":     "至少需要一个动作",
		"TEMPLATE_NOT_FOUND":  "模板未找到",

		// chat
		"MISSING_SESSION_ID": "会话 ID 不能为空",

		// cron
		"MISSING_SCHEDULE": "定时表达式不能为空",

		// system
		"MISSING_ENABLED_OR_PROVIDER": "请指定 enabled 或 provider 参数",

		// probe
		"PROBE_UNAVAILABLE": "执行探针服务未配置",

		// admin
		"MISSING_EVENT": "事件不能为空",
	},
}

// DetectLang extracts the preferred language from the Accept-Language header.
// Returns LangZhCN for any Chinese variant, LangEN otherwise.
func DetectLang(r *http.Request) Lang {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return LangEN
	}
	lower := strings.ToLower(accept)
	// Simple parsing: check for zh prefix in any quality-tagged segment
	for _, part := range strings.Split(lower, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if strings.HasPrefix(tag, "zh") {
			return LangZhCN
		}
	}
	return LangEN
}

// Translate returns the localised version of an error message for the given
// code and language. If no translation exists, the original message is returned.
func Translate(lang Lang, code, fallback string) string {
	if lang == LangEN {
		return fallback
	}
	if msgs, ok := catalog[lang]; ok {
		if msg, ok := msgs[code]; ok {
			return msg
		}
	}
	return fallback
}
