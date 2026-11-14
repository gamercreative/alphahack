SELECT e.id, e.requester_branch_id, e.requester_staff_id, e.raw_request, e.medication_id, e.quantity, e.status, e.outcome, e.outcome_branch_id, e.handled_by, e.clarification_question, e.clarification_answer, e.wake_trigger, e.next_wake_at, e.created_at, e.updated_at, e.closed_at
FROM enquiries e
WHERE e.requester_branch_id = $1
  AND ($2::text = '' OR e.status = $2)
ORDER BY e.created_at DESC, e.id
LIMIT $3 OFFSET $4
