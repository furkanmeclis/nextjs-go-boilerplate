import type { PublicKeyCredentialRequestOptionsJSON } from "@simplewebauthn/browser";

type PasskeyOptionsPayload = {
  publicKey?: PublicKeyCredentialRequestOptionsJSON;
} & Partial<PublicKeyCredentialRequestOptionsJSON>;

/** go-webauthn returns `{ publicKey: {...} }`; simplewebauthn expects the inner object. */
export function normalizePasskeyRequestOptions(
  raw: PasskeyOptionsPayload,
): PublicKeyCredentialRequestOptionsJSON {
  if (raw.publicKey) {
    return raw.publicKey;
  }
  if (typeof raw.challenge === "string") {
    return raw as PublicKeyCredentialRequestOptionsJSON;
  }
  throw new Error("Invalid passkey options payload");
}
