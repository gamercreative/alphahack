-- Reverts 0004_enquiries (children before parents).
DROP TABLE IF EXISTS enquiry_agent_tool_calls;
DROP TABLE IF EXISTS enquiry_agent_runs;
DROP TABLE IF EXISTS enquiry_events;
DROP TABLE IF EXISTS outbox_messages;
DROP TABLE IF EXISTS enquiry_targets;
DROP TABLE IF EXISTS enquiries;
DROP TABLE IF EXISTS staff_sessions;
DROP TABLE IF EXISTS staff;
ALTER TABLE branches DROP COLUMN IF EXISTS participating, DROP COLUMN IF EXISTS lng, DROP COLUMN IF EXISTS lat;
