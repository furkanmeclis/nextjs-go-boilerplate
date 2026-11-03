package config

import (
	"strings"
	"testing"
	"time"
)

func validProdConfig() Config {
	var c Config
	c.App.Name = "app"
	c.App.Env = "production"
	c.HTTP.Addr = ":8080"
	c.DB.Host, c.DB.User, c.DB.Name = "db", "u", "n"
	c.Redis.Addr = "redis:6379"
	c.Queue.Concurrency = 1
	c.JWT.AccessTTL, c.JWT.RefreshTTL = time.Minute, time.Hour
	c.JWT.AccessSecret = strings.Repeat("a", 40)
	c.JWT.RefreshSecret = strings.Repeat("b", 40)
	c.Encryption.Key = strings.Repeat("k", 32)
	c.Auth.AdapterSecret = strings.Repeat("s", 40)
	c.Centrifugo.Enabled = true
	c.Centrifugo.APIKey = "real-api-key"
	c.Centrifugo.TokenHMAC = "real-hmac"
	c.DB.Password = "real-db-password"
	c.Redis.Password = "real-redis-password"
	c.Storage.Driver = "minio"
	c.Storage.MinIO.AccessKey, c.Storage.MinIO.SecretKey = "real-access", "real-secret"
	return c
}

func TestValidateRejectsDefaultSecretsInProduction(t *testing.T) {
	if err := validProdConfig().validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	mutations := map[string]func(*Config){
		"default encryption key": func(c *Config) { c.Encryption.Key = defaultEncryptionKey },
		"empty encryption key":   func(c *Config) { c.Encryption.Key = "" },
		"default adapter secret": func(c *Config) { c.Auth.AdapterSecret = defaultAdapterSecret },
		"default centrifugo":     func(c *Config) { c.Centrifugo.TokenHMAC = defaultCentrifugoTokenHMAC },
		"short jwt secret":       func(c *Config) { c.JWT.AccessSecret = "short" },
	}
	for name, mut := range mutations {
		c := validProdConfig()
		mut(&c)
		if err := c.validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
	dev := validProdConfig()
	dev.App.Env = "development"
	dev.Encryption.Key = defaultEncryptionKey
	if err := dev.validate(); err != nil {
		t.Fatalf("development must accept defaults: %v", err)
	}
}

func TestValidateSMTPTLS(t *testing.T) {
	for _, mode := range []string{"", "auto", "starttls", "implicit", "none"} {
		c := validProdConfig()
		c.SMTP.TLSMode = mode
		if err := c.validate(); err != nil {
			t.Errorf("TLSMode %q rejected: %v", mode, err)
		}
	}
	bad := validProdConfig()
	bad.SMTP.TLSMode = "bogus"
	if err := bad.validate(); err == nil {
		t.Error("invalid TLSMode accepted")
	}

	insecure := validProdConfig()
	insecure.SMTP.TLSInsecureSkipVerify = true
	if err := insecure.validate(); err == nil {
		t.Error("insecure skip-verify accepted in production")
	}
	insecure.App.Env = "development"
	insecure.Encryption.Key = defaultEncryptionKey
	if err := insecure.validate(); err != nil {
		t.Errorf("insecure skip-verify must be allowed in development: %v", err)
	}
}

func TestParseReviewAccounts(t *testing.T) {
	got := parseReviewAccounts(" Review@Example.com:246810 , bad, short@x.io:123, nocolon@x.io ,x:123456")
	if len(got) != 1 || got["review@example.com"] != "246810" {
		t.Fatalf("got %v", got)
	}
	if len(parseReviewAccounts("")) != 0 {
		t.Fatal("empty must disable")
	}
}
