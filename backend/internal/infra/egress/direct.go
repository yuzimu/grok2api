package egress

import (
	"net/http"
	"net/url"
	"strings"
)

// DirectXAIHost 标识默认直连的x.ai域名；Console租约保留配置代理，不匹配相似域名。
func DirectXAIHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "x.ai" || strings.HasSuffix(host, ".x.ai")
}

// EnvironmentProxy 对 x.ai 绕过环境代理，其余域名保留原有代理规则。
func EnvironmentProxy(request *http.Request) (*url.URL, error) {
	if DirectXAIHost(request.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(request)
}
