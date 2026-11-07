ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_outbox_unique;

ALTER TABLE notifications
  ADD CONSTRAINT notifications_outbox_recipient_unique UNIQUE (outbox_id, user_id);