-- Settings queries
-- name: UpsertSettings :exec
INSERT INTO settings (id, api_root, server_url, esplora_url, currency, event_server, full_node, unit, ln_url, ln_datadir, ln_type)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    api_root = excluded.api_root,
    server_url = excluded.server_url,
    esplora_url = excluded.esplora_url,
    currency = excluded.currency,
    event_server = excluded.event_server,
    full_node = excluded.full_node,
    unit = excluded.unit,
    ln_url = excluded.ln_url,
    ln_datadir = excluded.ln_datadir,
    ln_type = excluded.ln_type;

-- name: DeleteSettings :exec
DELETE FROM settings;

-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- VHTLC queries
-- name: InsertVHTLC :exec
INSERT INTO vhtlc (
    id, preimage_hash, sender, receiver, server, refund_locktime,
    unilateral_claim_delay_type, unilateral_claim_delay_value,
    unilateral_refund_delay_type, unilateral_refund_delay_value,
    unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetVHTLC :one
SELECT * FROM vhtlc WHERE id = ?;

-- name: ListVHTLC :many
SELECT * FROM vhtlc;

-- VtxoRollover queries
-- name: UpsertVtxoRollover :exec
INSERT INTO vtxo_rollover (address, taproot_tree, destination_address) VALUES (?, ?, ?)
ON CONFLICT(address) DO UPDATE SET
    taproot_tree = excluded.taproot_tree,
    destination_address = excluded.destination_address;

-- name: GetVtxoRollover :one
SELECT * FROM vtxo_rollover WHERE address = ?;

-- name: ListVtxoRollover :many
SELECT * FROM vtxo_rollover;

-- name: DeleteVtxoRollover :exec
DELETE FROM vtxo_rollover WHERE address = ?;

-- Swap queries
-- name: CreateSwap :exec
INSERT INTO swap (
  id, amount, timestamp, to_currency, from_currency, swap_type, status, invoice, funding_tx_id, redeem_tx_id, vhtlc_id
) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? );

-- name: GetSwap :one
SELECT  sqlc.embed(swap),
        sqlc.embed(vhtlc)
FROM swap
  LEFT JOIN vhtlc ON swap.vhtlc_id = vhtlc.id
WHERE swap.id = ?;

-- name: ListSwaps :many
SELECT  sqlc.embed(swap), sqlc.embed(vhtlc)
FROM swap
  LEFT JOIN vhtlc ON swap.vhtlc_id = vhtlc.id;

-- name: UpdateSwap :exec
UPDATE swap 
SET status = ?,
redeem_tx_id = ?
WHERE id = ?;

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