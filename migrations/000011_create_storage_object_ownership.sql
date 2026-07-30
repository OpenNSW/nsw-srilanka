-- Migration: 000011_create_storage_object_ownership.sql
CREATE TABLE IF NOT EXISTS storage_object_ownership (
    key VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255),
    consignment_id VARCHAR(255),
    uploaded_by VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
