-- VHTLC queries
-- name: InsertVHTLC :exec
INSERT INTO vhtlc (id, script) VALUES (?, ?);

-- name: GetVHTLC :one
SELECT * FROM vhtlc WHERE id = ?;

-- name: ListVHTLC :many
SELECT * FROM vhtlc;

-- name: ListVHTLCsByID :many
SELECT * FROM vhtlc
WHERE id IN (sqlc.slice('ids'));

-- SubscribedScript queries
-- name: InsertSubscribedScript :exec
INSERT INTO subscribed_script (script)
VALUES (?);

-- name: GetSubscribedScript :one
SELECT * FROM subscribed_script WHERE script = ?;

-- name: ListSubscribedScript :many
SELECT * FROM subscribed_script;

-- name: DeleteSubscribedScript :exec
DELETE FROM subscribed_script WHERE script = ?;

-- name: InsertDelegateTask :exec
INSERT INTO delegate_task (id, intent_txid, intent_message, intent_proof, fee, delegator_public_key, scheduled_at, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertDelegateTaskInput :exec
INSERT INTO delegate_task_input (task_id, outpoint, forfeit_tx)
VALUES (?, ?, ?)
ON CONFLICT(task_id, outpoint) DO UPDATE SET
    forfeit_tx = excluded.forfeit_tx;

-- name: GetDelegateTask :many
SELECT
    dt.id,
    dt.intent_txid,
    dt.intent_message,
    dt.intent_proof,
    dt.fee,
    dt.delegator_public_key,
    dt.scheduled_at,
    dt.status,
    dt.fail_reason,
    dt.commitment_txid,
    dti.outpoint,
    dti.forfeit_tx
FROM delegate_task dt
LEFT JOIN delegate_task_input dti ON dt.id = dti.task_id
WHERE dt.id = ?;

-- name: GetDelegateTaskInputs :many
SELECT outpoint FROM delegate_task_input WHERE task_id = ?;

-- name: ListDelegateTaskPending :many
SELECT id, scheduled_at FROM delegate_task WHERE status = 0;

-- name: GetPendingTaskByIntentTxID :one
SELECT id, scheduled_at FROM delegate_task WHERE status = 0 AND intent_txid = ?;

-- name: CancelDelegateTasks :exec
UPDATE delegate_task
SET status = 3
WHERE status = 0 AND id IN (sqlc.slice(ids));

-- name: SuccessDelegateTasks :exec
UPDATE delegate_task
SET status = 1, commitment_txid = ?
WHERE status = 0 AND id IN (sqlc.slice(ids));

-- name: FailDelegateTasks :exec
UPDATE delegate_task
SET status = 2, fail_reason = ?
WHERE status = 0 AND id IN (sqlc.slice(ids));

-- name: GetPendingTaskIDsByInputs :many
SELECT DISTINCT dt.id FROM delegate_task dt
		INNER JOIN delegate_task_input dti ON dt.id = dti.task_id
		WHERE dt.status = 0
		AND dti.outpoint IN (sqlc.slice(outpoints));

-- name: ListDelegateTasks :many
SELECT
    dt.id,
    dt.intent_txid,
    dt.intent_message,
    dt.intent_proof,
    dt.fee,
    dt.delegator_public_key,
    dt.scheduled_at,
    dt.status,
    dt.fail_reason,
    dt.commitment_txid,
    dti.outpoint,
    dti.forfeit_tx
FROM delegate_task dt
LEFT JOIN delegate_task_input dti ON dt.id = dti.task_id
WHERE dt.status = ?
ORDER BY dt.scheduled_at DESC
LIMIT ? OFFSET ?;