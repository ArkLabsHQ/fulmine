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

-- name: UpsertDelegateTask :exec
INSERT INTO delegate_task (
    id, intent_json, forfeit_tx, inputs_json, fee, delegator_public_key, scheduled_at, status, fail_reason
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    intent_json = excluded.intent_json,
    forfeit_tx = excluded.forfeit_tx,
    inputs_json = excluded.inputs_json,
    fee = excluded.fee,
    delegator_public_key = excluded.delegator_public_key,
    scheduled_at = excluded.scheduled_at,
    status = excluded.status,
    fail_reason = excluded.fail_reason;

-- name: GetDelegateTask :one
SELECT * FROM delegate_task WHERE id = ?;

-- name: ListDelegateTaskPending :many
SELECT id, scheduled_at FROM delegate_task WHERE status = 'pending';

-- name: GetPendingTaskByInput :many
SELECT * FROM delegate_task 
WHERE status = 'pending' 
  AND json_extract(inputs_json, '$.hash') = ?
  AND CAST(json_extract(inputs_json, '$.index') AS INTEGER) = ?;
