CREATE TABLE IF NOT EXISTS delegate_task (
    id TEXT PRIMARY KEY,
    intent_json TEXT NOT NULL,
    fee INTEGER NOT NULL,
    delegator_public_key TEXT NOT NULL,
    scheduled_at INTEGER NOT NULL,
    status TEXT NOT NULL CHECK(status IN('pending', 'done', 'failed', 'cancelled')) DEFAULT 'pending',
    fail_reason TEXT
);

CREATE TABLE IF NOT EXISTS delegate_task_input (
    task_id TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    input_index INTEGER NOT NULL,
    forfeit_tx TEXT,
    PRIMARY KEY (task_id, input_hash, input_index),
    FOREIGN KEY (task_id) REFERENCES delegate_task(id) ON DELETE CASCADE
);