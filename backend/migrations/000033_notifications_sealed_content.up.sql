-- Security emails keep their rendered subject and body encrypted here, only
-- until delivery succeeds. title and body hold a redacted stand-in.
ALTER TABLE notifications ADD COLUMN sealed_content TEXT NULL;

-- Rows written before this column hold codes and invitation links in plaintext.
-- Unsent ones cannot be sealed from SQL (the key lives in the app), so they are
-- cancelled: login codes expire within minutes and invitations can be re-sent.
UPDATE notifications
SET status = CASE
        WHEN status IN ('queued', 'processing', 'failed') THEN 'cancelled'
        ELSE status
    END,
    title = template_code,
    body = '[redacted]'
WHERE channel = 'email'
  AND template_code IN (
    'auth.password_reset',
    'auth.email_verification',
    'auth.login_code',
    'auth.oauth_link_code',
    'auth.account_deactivation_code',
    'tenant.invitation'
  );
