CREATE TABLE IF NOT EXISTS delegate_task (
    id TEXT PRIMARY KEY,
    intent_json TEXT NOT NULL,
    forfeit_tx TEXT NOT NULL,
    inputs_json TEXT NOT NULL,
    fee INTEGER NOT NULL,
    delegator_public_key TEXT NOT NULL,
    scheduled_at INTEGER NOT NULL,
    status TEXT NOT NULL,
    fail_reason TEXT NOT NULL
);
