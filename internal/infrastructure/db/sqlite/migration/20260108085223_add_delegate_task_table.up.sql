CREATE TABLE IF NOT EXISTS delegate_task (
    id TEXT PRIMARY KEY,
    intent_txid TEXT NOT NULL,
    intent_message TEXT NOT NULL,
    intent_proof TEXT NOT NULL,
    fee INTEGER NOT NULL,
    delegator_public_key TEXT NOT NULL,
    scheduled_at INTEGER NOT NULL,
    status INTEGER NOT NULL CHECK(status IN(0,1,2,3)) DEFAULT 0,
    fail_reason TEXT,
    commitment_txid TEXT
);

CREATE TABLE IF NOT EXISTS delegate_task_input (
    task_id TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    input_index INTEGER NOT NULL,
    forfeit_tx TEXT,
    PRIMARY KEY (task_id, input_hash, input_index),
    FOREIGN KEY (task_id) REFERENCES delegate_task(id) ON DELETE CASCADE
);