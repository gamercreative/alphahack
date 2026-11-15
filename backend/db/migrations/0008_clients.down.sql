-- Reverts 0008_clients. Client events are removed so the old CHECK can return.
DELETE FROM enquiry_events WHERE actor_type = 'client';
ALTER TABLE enquiry_events DROP CONSTRAINT enquiry_events_actor_type_check;
ALTER TABLE enquiry_events ADD CONSTRAINT enquiry_events_actor_type_check
    CHECK (actor_type IN ('staff','agent','system'));
DROP TABLE IF EXISTS client_sessions;
DROP TABLE IF EXISTS clients;
DELETE FROM staff_sessions WHERE staff_id = 'staff-client-desk';
DELETE FROM staff WHERE id = 'staff-client-desk'
  AND NOT EXISTS (SELECT 1 FROM enquiries WHERE requester_staff_id = 'staff-client-desk');
DELETE FROM branches WHERE id = 'branch-client-desk'
  AND NOT EXISTS (SELECT 1 FROM enquiries WHERE requester_branch_id = 'branch-client-desk');
