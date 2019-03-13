ALTER TABLE versions ADD COLUMN major INTEGER NOT NULL;
ALTER TABLE versions ADD COLUMN minor INTEGER NOT NULL;
ALTER TABLE versions ADD COLUMN patch INTEGER NOT NULL;
ALTER TABLE versions ADD COLUMN prerelease TEXT;
ALTER TABLE versions ADD COLUMN build TEXT;
ALTER TABLE versions ADD CONSTRAINT unique_semver UNIQUE (major, minor, patch, prerelease, build);