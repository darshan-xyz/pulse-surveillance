-- Add detectionTypes column to cameras table
ALTER TABLE cameras ADD COLUMN "detectionTypes" text[];

-- Verify the column was added
SELECT column_name, data_type, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'cameras' 
AND column_name = 'detectionTypes';




