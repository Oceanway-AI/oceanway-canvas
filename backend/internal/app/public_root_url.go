package app

import (
	"errors"
	"net/url"
	"strings"
)

func validatePublicRootURL(value string) error {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("服务器公网地址必须是有效的 HTTP(S) 根地址")
	}
	return nil
}
