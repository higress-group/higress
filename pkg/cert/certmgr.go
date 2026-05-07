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

package cert

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez"
	"go.uber.org/zap"
	istiomodel "istio.io/istio/pilot/pkg/model"
	"k8s.io/client-go/kubernetes"
)

const (
	EventCertObtained = "cert_obtained"
)

var (
	cfg *certmagic.Config
)

type CertMgr struct {
	cfg           *certmagic.Config
	client        kubernetes.Interface
	namespace     string
	mux           sync.RWMutex
	storage       certmagic.Storage
	cache         *certmagic.Cache
	myACME        *certmagic.ACMEIssuer
	ingressSolver acmez.Solver
	configMgr     *ConfigMgr
	secretMgr     *SecretMgr
	XDSUpdater    istiomodel.XDSUpdater
}

func InitCertMgr(opts *Option, clientSet kubernetes.Interface, config *Config, XDSUpdater istiomodel.XDSUpdater, configMgr *ConfigMgr) (*CertMgr, error) {
	CertLog.Infof("certmgr init config: %+v", config)
	// Init certmagic config
	// First make a pointer to a Cache as we need to reference the same Cache in
	// GetConfigForCert below.
	var cache *certmagic.Cache
	var storage certmagic.Storage
	storage, _ = NewConfigmapStorage(opts.Namespace, clientSet)
	renewalWindowRatio := float64(config.RenewBeforeDays) / float64(RenewMaxDays)
	logger := zap.L()
	magicConfig := certmagic.Config{
		RenewalWindowRatio: renewalWindowRatio,
		Storage:            storage,
		Logger:             logger,
	}
	cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			// Here we use New to get a valid Config associated with the same cache.
			// The provided Config is used as a template and will be completed with
			// any defaults that are set in the Default config.
			return cfg, nil
		},
		Logger: logger,
	})
	// init certmagic
	cfg = certmagic.New(cache, magicConfig)

	// Init certmagic acme
	issuer := config.GetIssuer(IssuerTypeLetsencrypt)
	if issuer == nil {
		// should never happen here
		return nil, fmt.Errorf("there is no Letsencrypt Issuer found in config")
	}

	myACME := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
		//CA:                      certmagic.LetsEncryptStagingCA,
		CA:                      certmagic.LetsEncryptProductionCA,
		Email:                   issuer.Email,
		Agreed:                  true,
		DisableHTTPChallenge:    false,
		DisableTLSALPNChallenge: true,
	})
	// inject http01 solver
	ingressSolver, _ := NewIngressSolver(opts.Namespace, clientSet, myACME)
	myACME.Http01Solver = ingressSolver
	// init issuers
	cfg.Issuers = []certmagic.Issuer{myACME}

	secretMgr, _ := NewSecretMgr(opts.Namespace, clientSet)

	certMgr := &CertMgr{
		cfg:           cfg,
		client:        clientSet,
		namespace:     opts.Namespace,
		myACME:        myACME,
		ingressSolver: ingressSolver,
		configMgr:     configMgr,
		secretMgr:     secretMgr,
		cache:         cache,
		XDSUpdater:    XDSUpdater,
	}
	certMgr.cfg.OnEvent = certMgr.OnEvent
	return certMgr, nil
}
func (s *CertMgr) Reconcile(ctx context.Context, oldConfig *Config, newConfig *Config) error {
	CertLog.Infof("cermgr reconcile old config:%+v to new config:%+v", oldConfig, newConfig)
	// sync email
	if oldConfig != nil && newConfig != nil {
		oldIssuer := oldConfig.GetIssuer(IssuerTypeLetsencrypt)
		newIssuer := newConfig.GetIssuer(IssuerTypeLetsencrypt)
		if oldIssuer.Email != newIssuer.Email {
			// TODO before sync email, maybe need to clean up cache and account
		}
	}

	// sync domains
	newDomains := make([]string, 0)
	newDomainsMap := make(map[string]string, 0)
	removeDomains := make([]string, 0)

	if newConfig != nil {
		for _, config := range newConfig.CredentialConfig {
			if config.TLSIssuer == IssuerTypeLetsencrypt {
				for _, newDomain := range config.Domains {
					newDomains = append(newDomains, newDomain)
					newDomainsMap[newDomain] = newDomain
				}

			}
		}
	}

	if oldConfig != nil {
		for _, config := range oldConfig.CredentialConfig {
			if config.TLSIssuer == IssuerTypeLetsencrypt {
				for _, oldDomain := range config.Domains {
					if _, ok := newDomainsMap[oldDomain]; !ok {
						removeDomains = append(removeDomains, oldDomain)
					}
				}

			}
		}
	}

	if newConfig.AutomaticHttps == true {
		newIssuer := newConfig.GetIssuer(IssuerTypeLetsencrypt)
		// clean up  unused domains
		s.cleanSync(context.Background(), removeDomains)
		// sync email
		s.myACME.Email = newIssuer.Email
		// sync RenewalWindowRatio
		renewalWindowRatio := float64(newConfig.RenewBeforeDays) / float64(RenewMaxDays)
		s.cfg.RenewalWindowRatio = renewalWindowRatio
		// start cache
		s.cache.Start()
		// sync domains
		s.configMgr.SetConfig(newConfig)
		CertLog.Infof("certMgr start to manageSync domains: %+v", newDomains)
		s.manageSync(context.Background(), newDomains)
		CertLog.Infof("certMgr manageSync domains done")
	} else {
		// stop cache  maintainAssets
		s.cache.Stop()
		s.configMgr.SetConfig(newConfig)
	}

	if oldConfig != nil && newConfig != nil {
		if oldConfig.FallbackForInvalidSecret != newConfig.FallbackForInvalidSecret || !reflect.DeepEqual(oldConfig.CredentialConfig, newConfig.CredentialConfig) {
			CertLog.Infof("ingress need to full push")
			s.XDSUpdater.ConfigUpdate(&istiomodel.PushRequest{
				Full:   true,
				Reason: istiomodel.NewReasonStats("higress-https-updated"),
			})
		}
	}

	return nil
}

func (s *CertMgr) manageSync(ctx context.Context, domainNames []string) error {
	CertLog.Infof("cert manage sync domains:%v", domainNames)
	return s.cfg.ManageSync(ctx, domainNames)
}

func (s *CertMgr) cleanSync(ctx context.Context, domainNames []string) error {
	//TODO implement clean up domains
	CertLog.Infof("cert clean sync domains:%v", domainNames)
	return nil
}

func (s *CertMgr) OnEvent(ctx context.Context, event string, data map[string]any) error {
	CertLog.Infof("certmgr receive event:% data:%+v", event, data)
	/**
	event: cert_obtained
	cfg.emit(ctx, "cert_obtained", map[string]any{
		"renewal":          true,
		"remaining":        timeLeft,
		"identifier":       name,
		"issuer":           issuerKey,
		"storage_path":     StorageKeys.CertsSitePrefix(issuerKey, certKey),
		"private_key_path": StorageKeys.SitePrivateKey(issuerKey, certKey),
		"certificate_path": StorageKeys.SiteCert(issuerKey, certKey),
		"metadata_path":    StorageKeys.SiteMeta(issuerKey, certKey),
	})
	*/
	if event == EventCertObtained {
		// obtain certificate and update secret
		domain := data["identifier"].(string)
		isRenew := data["renewal"].(bool)
		privateKeyPath := data["private_key_path"].(string)
		certificatePath := data["certificate_path"].(string)
		privateKey, err := s.cfg.Storage.Load(context.Background(), privateKeyPath)
		certificate, err := s.cfg.Storage.Load(context.Background(), certificatePath)
		certChain, err := parseCertsFromPEMBundle(certificate)
		if err != nil {
			return err
		}
		notAfterTime := notAfter(certChain[0])
		notBeforeTime := notBefore(certChain[0])
		secretName := s.configMgr.GetConfig().GetSecretNameByDomain(IssuerTypeLetsencrypt, domain)
		if len(secretName) == 0 {
			CertLog.Errorf("can not find secret name for domain % in config", domain)
			return nil
		}
		err2 := s.secretMgr.Update(domain, secretName, privateKey, certificate, notBeforeTime, notAfterTime, isRenew)
		if err2 != nil {
			CertLog.Errorf("update secretName %s for domain %s error: %v", secretName, domain, err2)
		}
		return err
	}
	return nil
}
