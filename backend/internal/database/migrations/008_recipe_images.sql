-- Store an image URL for each recipe (downloaded from image search).
ALTER TABLE recipes ADD COLUMN IF NOT EXISTS image_url TEXT;
