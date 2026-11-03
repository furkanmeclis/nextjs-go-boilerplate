DELETE FROM notification_templates
WHERE code = 'tenant.invitation' AND channel = 'email' AND language IN ('tr', 'en');
