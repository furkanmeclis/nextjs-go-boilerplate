-- Data-loss note: subjects and bodies redacted by the up migration cannot be
-- restored, and undelivered sealed rows are cancelled because the old code would
-- send their redacted stand-in.
UPDATE notifications
SET status = 'cancelled'
WHERE sealed_content IS NOT NULL
  AND status IN ('queued', 'processing', 'failed');

ALTER TABLE notifications DROP COLUMN sealed_content;
