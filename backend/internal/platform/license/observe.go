package license

import "log/slog"

// Observe resolves the license from configuration and logs it once, at
// startup. This is phase-1 "observe mode": the license is verified and
// reported but never used to gate traffic, whatever LICENSE_ENFORCE is set to
// — enforcement arrives in a later phase. The resolved State is returned for
// callers that want to hold onto it.
//
// Loading the embedded public keys is best-effort: a parse failure is logged
// and treated as an empty key set (every token then fails to verify), never
// fatal, so a key problem can never take a deployment down in observe mode.
func Observe(log *slog.Logger, env, appID, token string, enforceConfigured bool) State {
	keys, err := EmbeddedKeys()
	if err != nil {
		log.Error("license_keys_load_failed", "error", err)
	}
	st := Resolve(Input{Env: env, AppID: appID, Token: token, Keys: keys})

	// enforce_configured is surfaced for visibility only; phase 1 does not act
	// on it. It lets operators confirm the switch before phase-2 gating lands.
	if st.Valid {
		log.Info("license_observed", "state", st, "enforce_configured", enforceConfigured)
	} else {
		log.Warn("license_unlicensed", "state", st, "enforce_configured", enforceConfigured)
	}
	return st
}
