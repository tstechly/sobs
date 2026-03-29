ATTACH TABLE _ UUID 'cd44f65d-0659-40f2-adbe-ac554456f279'
(
    `id` UInt64,
    `payload` String
)
ENGINE = MergeTree
ORDER BY id
SETTINGS storage_policy = 'encrypted_only', index_granularity = 8192
