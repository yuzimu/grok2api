package provider

import "strings"

const ContentPolicyCode = "content_policy_violation"
const ContentPolicyMessage = "提示词或参考素材未通过内容安全审核，请修改后重新生成。"

// IsContentPolicyRejection 只识别上游明确的安全拒绝标记，不将普通权限错误视为审核拒绝。
func IsContentPolicyRejection(text string) bool {
	value := strings.ToLower(text)
	return strings.Contains(value, "content_policy_violation") ||
		strings.Contains(value, "content_policy_rejected") ||
		strings.Contains(value, "content violates usage guidelines") ||
		strings.Contains(value, "safety_check_type_")
}
