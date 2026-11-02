package license

import "strings"

// EnvDevelopment is the APP_ENV value that grants a synthetic license without
// a key, mirroring the rest of the platform's development fallbacks.
const EnvDevelopment = "development"

// Input is the configuration Resolve needs. It is deliberately a small value
// of primitives so this package stays free of the config package and of any
// I/O: the caller reads the environment and embeds the keys.
type Input struct {
	// Env is APP_ENV. "development" grants a synthetic license.
	Env string
	// AppID is APP_NAME; a token's app claim must equal it.
	AppID string
	// Token is the raw LICENSE_KEY (may be empty).
	Token string
	// Keys is the verifier's key set, normally license.EmbeddedKeys().
	Keys KeySet
}

// Resolve computes the current license State from configuration.
//
//   - In development it returns a synthetic valid license; no key is needed.
//   - An empty token outside development is reported as ReasonNoLicense
//     (Valid=false). Phase 1 only logs this; whether it eventually gates
//     traffic is governed by LICENSE_ENFORCE in a later phase.
//   - Otherwise the token is verified against Keys.
func Resolve(in Input) State {
	if strings.EqualFold(strings.TrimSpace(in.Env), EnvDevelopment) {
		return State{
			Valid:       true,
			Reason:      ReasonDevelopment,
			Development: true,
			AppID:       in.AppID,
		}
	}
	if strings.TrimSpace(in.Token) == "" {
		return State{Valid: false, Reason: ReasonNoLicense, AppID: in.AppID}
	}
	return NewVerifier(in.AppID, in.Keys).Verify(in.Token)
}
