package test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
)

type keySet struct {
	Name       string
	PrivateKey any
	PublicKey  any
}

type jwts struct {
	JWTs []struct {
		Algorithm string `json:"alg"`
		Token     string `json:"token"`
		Type      string `json:"type"`
	} `json:"jwts"`
}

func genPrivateKey() (keySets map[string]keySet) {
	keySets = map[string]keySet{}
	rsaPri, _ := rsa.GenerateKey(rand.Reader, 2048)
	keySets["rsa"] = keySet{Name: "rsa", PrivateKey: rsaPri, PublicKey: &rsaPri.PublicKey}

	// ed25519pri, ed25519pub, _ := ed25519.GenerateKey(rand.Reader)
	// keySets["ed25519"] = keySet{Name: "ed25519", PrivateKey: ed25519pri, PublicKey: ed25519pub}

	p256Pri, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keySets["p256"] = keySet{Name: "p256", PrivateKey: p256Pri, PublicKey: &p256Pri.PublicKey}

	// p384Pri, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	// keySets = append(keySets, keySet{Name: "p384", PrivateKey: p384Pri, PublicKey: &p384Pri.PublicKey})

	// p521Pri, _ := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	// keySets = append(keySets, keySet{Name: "p521", PrivateKey: p521Pri, PublicKey: &p521Pri.PublicKey})
	return
}

func genJWKs(keySets map[string]keySet) (keys jose.JSONWebKeySet) {
	for k := range keySets {
		k := jose.JSONWebKey{
			Key:   keySets[k].PublicKey,
			KeyID: keySets[k].Name,
		}
		keys.Keys = append(keys.Keys, k)
	}
	return
}

func genJWTs(keySets map[string]keySet) (jwts jwts) {
	claims := map[string]jwt.Claims{
		"normal": {
			Issuer:    "higress-test",
			Subject:   "higress-test",
			Audience:  []string{"foo", "bar"},
			Expiry:    jwt.NewNumericDate(time.Date(2034, 1, 1, 0, 0, 0, 0, time.UTC)),
			NotBefore: jwt.NewNumericDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		"expired": {
			Issuer:    "higress-test",
			Subject:   "higress-test",
			Audience:  []string{"foo", "bar"},
			Expiry:    jwt.NewNumericDate(time.Date(2024, 1, 1, 0, 0, 0, 1, time.UTC)),
			NotBefore: jwt.NewNumericDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
	}

	sigrsa, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       keySets["rsa"].PrivateKey,
	}, (&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "rsa"))
	if err != nil {
		panic(err)
	}

	sigp256, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.ES256,
		Key:       keySets["p256"].PrivateKey,
	}, (&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "p256"))
	if err != nil {
		panic(err)
	}

	sigs := map[string]jose.Signer{
		"RS256": sigrsa,
		"ES256": sigp256,
	}

	for k1, v1 := range sigs {
		for k2, v2 := range claims {
			raw, _ := jwt.Signed(v1).Claims(v2).CompactSerialize()
			jwts.JWTs = append(jwts.JWTs, struct {
				Algorithm string "json:\"alg\""
				Token     string "json:\"token\""
				Type      string "json:\"type\""
			}{
				Algorithm: k1,
				Token:     raw,
				Type:      k2,
			})
		}
	}
	return
}

func TestMain(m *testing.M) {
	keySets := genPrivateKey()
	keys := genJWKs(keySets)
	jwts := genJWTs(keySets)

	jwks, err := json.Marshal(keys)
	if err != nil {
		panic(err)
	}
	f, _ := os.Create("keys.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	f.WriteString(string(jwks))

	jwtsm, err := json.Marshal(&jwts)
	if err != nil {
		panic(err)
	}
	f, _ = os.Create("jwts.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	f.WriteString(string(jwtsm))
	m.Run()
}
