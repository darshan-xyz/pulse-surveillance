-- Add detectionTypes column to cameras table
-- This will add a text array column to store detection types

-- First, let's see what columns currently exist
SELECT column_name, data_type, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'cameras' 
ORDER BY ordinal_position;

-- Add the detectionTypes column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'cameras' 
        AND column_name = 'detectionTypes'
    ) THEN
        ALTER TABLE cameras ADD COLUMN "detectionTypes" text[];
        RAISE NOTICE 'Added detectionTypes column to cameras table';
    ELSE
        RAISE NOTICE 'detectionTypes column already exists in cameras table';
    END IF;
END $$;

-- Verify the column was added
SELECT column_name, data_type, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'cameras' 
ORDER BY ordinal_position;




