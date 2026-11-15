// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package annotations

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	networking "istio.io/api/networking/v1alpha3"
)

const (
	appRoot               = "app-root"
	temporalRedirect      = "temporal-redirect"
	permanentRedirect     = "permanent-redirect"
	permanentRedirectCode = "permanent-redirect-code"
	sslRedirect           = "ssl-redirect"
	forceSSLRedirect      = "force-ssl-redirect"

	defaultPermanentRedirectCode = 301
	defaultTemporalRedirectCode  = 302
)

var (
	_ Parser       = &redirect{}
	_ RouteHandler = &redirect{}
)

type RedirectConfig struct {
	AppRoot string

	URL string

	Code int

	httpsRedirect bool
}

type redirect struct{}

func (r redirect) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needRedirectConfig(annotations) {
		return nil
	}

	// MCP server traffic is fully intercepted by the mcp-server plugin before routing,
	// and tool calls reuse the matched route's cluster to reach the backend. A redirect
	// route has no cluster, so every tool call would end in a local redirect instead of
	// reaching the backend. Redirect annotations are meaningless on MCP server routes.
	if enabled, _ := annotations.ParseBoolASAP(enableMcpServer); enabled {
		return nil
	}

	redirectConfig := &RedirectConfig{
		Code: defaultPermanentRedirectCode,
	}
	config.Redirect = redirectConfig

	redirectConfig.AppRoot, _ = annotations.ParseStringASAP(appRoot)

	httpsRedirect, _ := annotations.ParseBoolASAP(sslRedirect)
	forceHTTPSRedirect, _ := annotations.ParseBoolASAP(forceSSLRedirect)
	if httpsRedirect || forceHTTPSRedirect {
		redirectConfig.httpsRedirect = true
	}

	// temporal redirect is firstly applied.
	tr, err := annotations.ParseStringASAP(temporalRedirect)
	if err != nil && !IsMissingAnnotations(err) {
		return nil
	}
	if tr != "" && isValidURL(tr) == nil {
		redirectConfig.URL = tr
		redirectConfig.Code = defaultTemporalRedirectCode
		return nil
	}

	// permanent redirect
	// url
	pr, err := annotations.ParseStringASAP(permanentRedirect)
	if err != nil && !IsMissingAnnotations(err) {
		return nil
	}
	if pr != "" && isValidURL(pr) == nil {
		redirectConfig.URL = pr
	}
	// code
	if prc, err := annotations.ParseIntASAP(permanentRedirectCode); err == nil {
		if prc < http.StatusMultipleChoices || prc > http.StatusPermanentRedirect {
			prc = defaultPermanentRedirectCode
		}
		redirectConfig.Code = prc
	}

	return nil
}

func (r redirect) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	redirectConfig := config.Redirect
	if redirectConfig == nil {
		return
	}

	var redirectPolicy *networking.HTTPRedirect
	if redirectConfig.URL != "" {
		parseURL, err := url.Parse(redirectConfig.URL)
		if err != nil {
			return
		}
		redirectPolicy = &networking.HTTPRedirect{
			Scheme:       parseURL.Scheme,
			Authority:    parseURL.Host,
			Uri:          parseURL.Path,
			RedirectCode: uint32(redirectConfig.Code),
		}
	} else if redirectConfig.httpsRedirect {
		redirectPolicy = &networking.HTTPRedirect{
			Scheme: "https",
			// 308 is the default code for ssl redirect
			RedirectCode: 308,
		}
	}

	route.Redirect = redirectPolicy
}

func needRedirectConfig(annotations Annotations) bool {
	return annotations.HasASAP(temporalRedirect) ||
		annotations.HasASAP(permanentRedirect) ||
		annotations.HasASAP(sslRedirect) ||
		annotations.HasASAP(forceSSLRedirect) ||
		annotations.HasASAP(appRoot)
}

func isValidURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(u.Scheme, "http") {
		return fmt.Errorf("only http and https are valid protocols (%v)", u.Scheme)
	}

	return nil
}
