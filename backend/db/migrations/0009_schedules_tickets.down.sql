-- Reverts 0009_schedules_tickets (the client-desk enquiries stay).
DROP TABLE IF EXISTS client_tickets;
DROP TABLE IF EXISTS medicine_schedules;
