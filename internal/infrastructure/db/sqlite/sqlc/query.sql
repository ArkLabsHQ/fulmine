-- Settings queries
-- name: UpsertSettings :exec
INSERT INTO settings (id, api_root, server_url, esplora_url, currency, event_server, full_node, ln_url, unit)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    api_root = excluded.api_root,
    server_url = excluded.server_url,
    esplora_url = excluded.esplora_url,
    currency = excluded.currency,
    event_server = excluded.event_server,
    full_node = excluded.full_node,
    ln_url = excluded.ln_url,
    unit = excluded.unit;

-- name: DeleteSettings :exec
DELETE FROM settings;

-- name: GetSettings :one
SELECT api_root, server_url, esplora_url, currency, event_server, full_node, ln_url, unit FROM settings WHERE id = 1;

-- VHTLC queries
-- name: InsertVHTLC :exec
INSERT INTO vhtlc (
    preimage_hash, sender, receiver, server, refund_locktime,
    unilateral_claim_delay_type, unilateral_claim_delay_value,
    unilateral_refund_delay_type, unilateral_refund_delay_value,
    unilateral_refund_without_receiver_delay_type, unilateral_refund_without_receiver_delay_value
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(preimage_hash) DO UPDATE SET
    sender = excluded.sender,
    receiver = excluded.receiver,
    server = excluded.server,
    refund_locktime = excluded.refund_locktime,
    unilateral_claim_delay_type = excluded.unilateral_claim_delay_type,
    unilateral_claim_delay_value = excluded.unilateral_claim_delay_value,
    unilateral_refund_delay_type = excluded.unilateral_refund_delay_type,
    unilateral_refund_delay_value = excluded.unilateral_refund_delay_value,
    unilateral_refund_without_receiver_delay_type = excluded.unilateral_refund_without_receiver_delay_type,
    unilateral_refund_without_receiver_delay_value = excluded.unilateral_refund_without_receiver_delay_value;

-- name: GetVHTLC :one
SELECT *
FROM vhtlc WHERE preimage_hash = ?;

-- name: ListVHTLC :many
SELECT *
FROM vhtlc;

-- name: DeleteVHTLC :exec
DELETE FROM vhtlc WHERE preimage_hash = ?;

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
