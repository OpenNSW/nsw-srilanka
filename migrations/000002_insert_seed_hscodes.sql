-- @UP
-- ============================================================================
-- Migration: 000002_insert_seed_hscodes.sql
-- Purpose: Seed baseline HS codes used for workflow selection.
-- ============================================================================

-- Seed data: HS code master entries
INSERT INTO hs_codes (id, hs_code, description, category)
VALUES
    (
        '90b06747-cfa7-486b-a084-eaa1fc95595e',
        '0902.10',
        'Green tea (not fermented) in immediate packings of content not exceeding ≤ 3kg',
        'Green Tea (Small)'
    ),
    (
        '3699f18c-832a-4026-ac31-3697c3a5235d',
        '0902.10.11',
        'Certified Ceylon Green tea, flavoured, ≤ 4g (Tea bags)',
        'Green Tea'
    ),
    (
        '1589b5b1-2db3-44ef-80c1-16151bb8d5b0',
        '0902.20',
        'Green tea (not fermented) in immediate packings > 3kg',
        'Green Tea (Bulk)'
    ),
    (
        '6aa146ba-dd72-4e5e-ae27-a1cb5d69caa5',
        '0902.30',
        'Black tea (fermented) in immediate packings ≤ 3kg',
        'Black Tea (Small)'
    ),
    (
        '2e173ef8-840b-4cc5-a667-03e1d80e04b9',
        '0902.30.21',
        'Certified Ceylon Black tea, flavoured, 4g–1kg',
        'Black Tea'
    ),
    (
        '851f0de7-0693-4cc1-9d92-19c39072bb53',
        '0902.40',
        'Black tea (fermented) in immediate packings > 3kg',
        'Black Tea (Bulk)'
    ),
    (
        '51e802c1-b57e-45ac-b563-1ae0fad06db5',
        '2101.20',
        'Extracts, essences, and concentrates of tea (Instant Tea)',
        'Value Added'
    ),
    (
        'cb34d1ac-c48f-4370-8260-a6585009ff7e',
        '2101.20.11',
        'Instant tea, certified Ceylon origin, ≤ 4g',
        'Instant Tea'
    ),
    (
        '36a58d44-8ff6-4bea-8c9b-3db84bb5a083',
        '0801.11.10',
        'Edible Copra',
        'Kernel'
    ),
    (
        '8a0783e4-82e6-488e-b96e-6140a8912f39',
        '0801.11.90',
        'Desiccated Coconut (DC)',
        'Kernel'
    ),
    (
        '4bdfb1f0-2b71-4ddc-8b99-f31c3d7660bc',
        '0801.12.00',
        'Fresh Coconut (in the inner shell)',
        'Fresh Fruit'
    ),
    (
        'b9e48207-2573-4c9b-89f6-06d4c22422be',
        '0801.19.30',
        'King Coconut (Thambili)',
        'Fresh Fruit'
    ),
    (
        '6b567998-4a57-4132-a595-577493aefb3f',
        '1106.30.10',
        'Coconut Flour',
        'Kernel'
    ),
    (
        '653c4c8f-8c39-4aee-86f5-7f3926d0d4c2',
        '1513.11.11',
        'Virgin Coconut Oil (VCO) - In Bulk',
        'Oils'
    ),
    (
        'bfa92119-64d3-41f4-b21c-fd0e2eb2966b',
        '1513.11.21',
        'Virgin Coconut Oil (VCO) - Not in Bulk',
        'Oils'
    ),
    (
        '5e0f2a51-8a1e-4d7d-a00b-4565e47535d2',
        '1513.19.10',
        'Coconut Oil (Refined/Not crude) - In Bulk',
        'Oils'
    ),
    (
        '4f4fac26-bf5c-42b0-9058-b17828dcba31',
        '2008.19.20',
        'Liquid Coconut Milk',
        'Edible Prep'
    ),
    (
        '1390c617-43d4-4eee-8fff-b9f10d038981',
        '2008.19.30',
        'Coconut Milk Powder',
        'Edible Prep'
    ),
    (
        'fd5a0de1-c547-4420-94b9-942a8349a463',
        '2106.90.97',
        'Coconut Water',
        'Beverages'
    ),
    (
        '7884654e-90e0-4b7c-a963-cf6d2b5d1c16',
        '1404.90.30',
        'Coconut Shell Pieces',
        'Non-Kernel'
    ),
    (
        '4ba1fd6b-f42f-438f-ab9f-0ee0054ee33c',
        '1404.90.50',
        'Coconut Husk Chips',
        'Non-Kernel'
    ) ON CONFLICT (id) DO NOTHING;
-- @DOWN
-- Migration: 000002_insert_seed_hscodes.sql
-- Description: Roll back HS code seed data.

DELETE FROM hs_codes 
WHERE id IN (
    '90b06747-cfa7-486b-a084-eaa1fc95595e',
    '3699f18c-832a-4026-ac31-3697c3a5235d',
    '1589b5b1-2db3-44ef-80c1-16151bb8d5b0',
    '6aa146ba-dd72-4e5e-ae27-a1cb5d69caa5',
    '2e173ef8-840b-4cc5-a667-03e1d80e04b9',
    '851f0de7-0693-4cc1-9d92-19c39072bb53',
    '51e802c1-b57e-45ac-b563-1ae0fad06db5',
    'cb34d1ac-c48f-4370-8260-a6585009ff7e',
    '36a58d44-8ff6-4bea-8c9b-3db84bb5a083',
    '8a0783e4-82e6-488e-b96e-6140a8912f39',
    '4bdfb1f0-2b71-4ddc-8b99-f31c3d7660bc',
    'b9e48207-2573-4c9b-89f6-06d4c22422be',
    '6b567998-4a57-4132-a595-577493aefb3f',
    '653c4c8f-8c39-4aee-86f5-7f3926d0d4c2',
    'bfa92119-64d3-41f4-b21c-fd0e2eb2966b',
    '5e0f2a51-8a1e-4d7d-a00b-4565e47535d2',
    '4f4fac26-bf5c-42b0-9058-b17828dcba31',
    '1390c617-43d4-4eee-8fff-b9f10d038981',
    'fd5a0de1-c547-4420-94b9-942a8349a463',
    '7884654e-90e0-4b7c-a963-cf6d2b5d1c16',
    '4ba1fd6b-f42f-438f-ab9f-0ee0054ee33c'
);