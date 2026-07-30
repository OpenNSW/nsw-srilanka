-- Migration: 000011_create_storage_object_metadata.sql
CREATE TABLE IF NOT EXISTS storage_object_metadata (
    key VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255),
    consignment_id VARCHAR(255),
    company_id VARCHAR(255),
    uploaded_by VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_storage_object_metadata_task_id ON storage_object_metadata (task_id);
CREATE INDEX IF NOT EXISTS idx_storage_object_metadata_consignment_id ON storage_object_metadata (consignment_id);

