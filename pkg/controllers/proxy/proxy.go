package proxy

import "github.com/gofiber/fiber/v2"

type ProxyTypes string

const (
	REDIRECT ProxyTypes = "redirect"
	PROXY    ProxyTypes = "proxy"
)

type ProxyRule struct {
	SourceDomain string
	TargetDomain string
	SourcePath   string
	TargetPath   string
	Method       ProxyTypes
}

type ProxyController struct {
	ProxyRules []*ProxyRule
}

func (r *ProxyRule) Matches(ctx *fiber.Ctx) bool {
	return string(ctx.Request().Host()) == r.SourceDomain && ctx.Path() == r.SourcePath
}
func (r *ProxyRule) Handle(ctx *fiber.Ctx) error {
	if r.Method == REDIRECT {
		return ctx.Redirect(r.TargetDomain + r.TargetPath)
	} else {
		return nil
	}
}

func New(proxyRuleList []*ProxyRule) *ProxyController {
	return &ProxyController{
		ProxyRules: proxyRuleList,
	}
}

func (c *ProxyController) HandleRequest(ctx *fiber.Ctx) error {
	for _, rule := range c.ProxyRules {
		if rule.Matches(ctx) {
			return rule.Handle(ctx)
		}
	}
	return ctx.Next()
}
