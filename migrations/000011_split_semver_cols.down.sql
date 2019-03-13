ALTER TABLE versions DROP CONSTRAINT unique_semver;
ALTER TABLE versions DROP COLUMN major;
ALTER TABLE versions DROP COLUMN minor;
ALTER TABLE versions DROP COLUMN patch;
ALTER TABLE versions DROP COLUMN prerelease;
ALTER TABLE versions DROP COLUMN build;