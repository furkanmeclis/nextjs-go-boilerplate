package config

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The schema lives outside the Go module (go:embed cannot reach it), so the
// tests read it from the repository checkout.
var (
	schemaPath     = filepath.Join("..", "..", "..", "deploy", "env", "schema")
	devExamplePath = filepath.Join("..", "..", "..", ".env.example")
)

// Schema secrets the backend guard does not cover, and why.
var unguardedSchemaSecrets = map[string]string{
	"AUTH_SECRET":       "read by the Next.js frontend only",
	"VAPID_PUBLIC_KEY":  "public by design",
	"VAPID_PRIVATE_KEY": "no development default; empty turns Web Push off",
}

func schemaSecrets(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(schemaPath)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() { _ = f.Close() }()
	line := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*\|\s*(secret|vapid):`)
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := line.FindStringSubmatch(sc.Text()); m != nil {
			out[m[1]] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no secret: keys parsed from the schema")
	}
	return out
}

func readDevExample(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(devExamplePath)
	if err != nil {
		t.Fatalf("open .env.example: %v", err)
	}
	defer func() { _ = f.Close() }()
	line := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := line.FindStringSubmatch(sc.Text()); m != nil {
			out[m[1]] = strings.Trim(m[2], `"'`)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// A secret added to deploy/env/schema must be guarded here (or exempted with a
// reason), and the guard must not cover keys the schema no longer manages.
func TestManagedSecretsMatchSchema(t *testing.T) {
	schema := schemaSecrets(t)
	guarded := map[string]bool{}
	for _, s := range managedSecrets {
		if guarded[s.Key] {
			t.Errorf("%s is listed twice in managedSecrets", s.Key)
		}
		guarded[s.Key] = true
	}
	for key, why := range unguardedSchemaSecrets {
		if !schema[key] {
			t.Errorf("unguardedSchemaSecrets has %s (%s) but the schema no longer manages it", key, why)
		}
		if guarded[key] {
			t.Errorf("%s is both guarded and exempted", key)
		}
	}
	for _, key := range sortedKeys(schema) {
		if !guarded[key] && unguardedSchemaSecrets[key] == "" {
			t.Errorf("schema secret %s has no entry in managedSecrets (guard.go); add one or exempt it with a reason", key)
		}
	}
	for _, key := range sortedKeys(guarded) {
		if !schema[key] {
			t.Errorf("managedSecrets guards %s, which is not a secret in %s", key, schemaPath)
		}
	}
}

// Every development value a secret has in .env.example is public, so the guard
// must refuse it (an empty one is refused as "required").
func TestDevExampleSecretsAreRefused(t *testing.T) {
	dev := readDevExample(t)
	for _, s := range managedSecrets {
		v, ok := dev[s.Key]
		if !ok {
			t.Errorf("%s is missing from .env.example", s.Key)
			continue
		}
		if v != "" && !contains(s.Defaults, v) {
			t.Errorf("%s: .env.example value is not in its Defaults, so production would accept it", s.Key)
		}
	}
}

// strongEnv sets every guarded secret to a unique strong value plus a
// production APP_ENV, so Load succeeds; tests then break one thing at a time.
func strongEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{}
	t.Setenv("APP_ENV", EnvProduction)
	t.Setenv("STORAGE_DRIVER", "minio")
	t.Setenv("CENTRIFUGO_ENABLED", "true")
	t.Setenv("SEARCH_ENABLED", "true")
	for _, k := range []string{"MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"} {
		t.Setenv(k, "") // aliases fall back to S3_*
	}
	for _, s := range managedSecrets {
		v := "strong-" + strings.ToLower(s.Key) + "-0123456789abcdef0123456789abcdef"
		values[s.Key] = v
		t.Setenv(s.Key, v)
	}
	return values
}

// Each table row must read the env key it names: otherwise the guard would
// check one variable while the operator sets another.
func TestManagedSecretsReadTheirEnvKey(t *testing.T) {
	values := strongEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with strong secrets: %v", err)
	}
	for _, s := range managedSecrets {
		if got := s.Value(cfg); got != values[s.Key] {
			t.Errorf("%s: Value() returned %q, want the value of env %s", s.Key, got, s.Key)
		}
	}
}

func TestSecretGuardRefusesDefaultsOutsideDevelopment(t *testing.T) {
	for _, s := range managedSecrets {
		cases := map[string]string{"empty": ""}
		for _, d := range s.Defaults {
			cases["default "+d] = d
		}
		for name, v := range cases {
			t.Run(s.Key+"/"+name, func(t *testing.T) {
				strongEnv(t)
				t.Setenv(s.Key, v) // empty falls back to the Load default: refused too
				_, err := Load()
				if err == nil || !strings.Contains(err.Error(), s.Key) {
					t.Fatalf("expected an error naming %s, got %v", s.Key, err)
				}
			})
		}
	}
}

func TestSecretGuardReportsEveryProblemAtOnce(t *testing.T) {
	strongEnv(t)
	t.Setenv("JWT_REFRESH_SECRET", defaultJWTRefreshSecret)
	t.Setenv("APP_ENCRYPTION_KEY", defaultEncryptionKey)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_REFRESH_SECRET") || !strings.Contains(err.Error(), "APP_ENCRYPTION_KEY") {
		t.Fatalf("want both keys in one error, got %v", err)
	}
}

func TestSecretGuardSkipsUnusedFeatures(t *testing.T) {
	strongEnv(t)
	t.Setenv("CENTRIFUGO_ENABLED", "false")
	t.Setenv("CENTRIFUGO_API_KEY", defaultCentrifugoAPIKey)
	t.Setenv("CENTRIFUGO_TOKEN_HMAC_SECRET", defaultCentrifugoTokenHMAC)
	t.Setenv("SEARCH_ENABLED", "false")
	t.Setenv("MEILI_MASTER_KEY", defaultMeiliMasterKey)
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("S3_ACCESS_KEY", defaultS3AccessKey)
	t.Setenv("S3_SECRET_KEY", defaultS3SecretKey)
	if _, err := Load(); err != nil {
		t.Fatalf("disabled features must not need their secrets: %v", err)
	}
}

func TestSecretGuardChecksTheActiveObjectStore(t *testing.T) {
	strongEnv(t)
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("S3_SECRET_KEY", defaultS3SecretKey)
	t.Setenv("MINIO_SECRET_KEY", "strong-minio-only-secret-0123456789abcdef")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "S3_SECRET_KEY") {
		t.Fatalf("driver s3 must check the S3 credentials, got %v", err)
	}
}

func TestAppEnv(t *testing.T) {
	t.Run("staging is guarded like production", func(t *testing.T) {
		strongEnv(t)
		t.Setenv("APP_ENV", EnvStaging)
		t.Setenv("AUTH_ADAPTER_SECRET", defaultAdapterSecret)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AUTH_ADAPTER_SECRET") {
			t.Fatalf("staging must refuse defaults, got %v", err)
		}
	})
	t.Run("development and test accept every default", func(t *testing.T) {
		for _, env := range []string{EnvDevelopment, EnvTest} {
			for _, s := range managedSecrets {
				t.Setenv(s.Key, "")
			}
			t.Setenv("APP_ENV", env)
			if _, err := Load(); err != nil {
				t.Fatalf("APP_ENV=%s with defaults: %v", env, err)
			}
		}
	})
	t.Run("unknown values fail with a hint", func(t *testing.T) {
		for env, hint := range map[string]string{"prod": EnvProduction, "dev": EnvDevelopment, "qa": ""} {
			t.Setenv("APP_ENV", env)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
				t.Fatalf("APP_ENV=%s: want an APP_ENV error, got %v", env, err)
			}
			if hint != "" && !strings.Contains(err.Error(), `did you mean "`+hint+`"`) {
				t.Fatalf("APP_ENV=%s: want hint %q, got %v", env, hint, err)
			}
		}
	})
}
