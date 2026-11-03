-- Email template for organization invitations (tenant core phase 3).
-- Vars: org_name, inviter_name, role_name, link, expires_days.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('tenant.invitation', 'email', 'tr', '{{org_name}} organizasyonuna davet edildiniz',
     'Merhaba, {{inviter_name}} sizi {{org_name}} organizasyonuna {{role_name}} olarak davet etti. Daveti kabul etmek için bağlantıyı açın: {{link}} (Bağlantı {{expires_days}} gün geçerlidir.)', TRUE),
    ('tenant.invitation', 'email', 'en', 'You are invited to {{org_name}}',
     'Hello, {{inviter_name}} invited you to join {{org_name}} as {{role_name}}. Open this link to accept: {{link}} (The link is valid for {{expires_days}} days.)', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
