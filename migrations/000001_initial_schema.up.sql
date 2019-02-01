-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- A series is a group of modules that share the same base path and are assumed
-- to be major-version variants. series.name is the path prefix shared by these
-- variants.
CREATE TABLE series (
	created_at TIMESTAMP NOT NULL,
	name TEXT UNIQUE NOT NULL,
	PRIMARY KEY (name)
);

CREATE TABLE modules (
	created_at TIMESTAMP NOT NULL,
	name TEXT UNIQUE NOT NULL,
	series_name TEXT NOT NULL,
	PRIMARY KEY (name),
	FOREIGN KEY (series_name) REFERENCES series(name)
);

CREATE TABLE versions (
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL, -- should only change on takedown
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	license TEXT NOT NULL,
	synopsis TEXT,
	commit_time TIMESTAMP,
	deleted BOOL NOT NULL, 
	PRIMARY KEY (name, version),
	FOREIGN KEY (name) REFERENCES modules(name)
);

CREATE TABLE packages (
	path TEXT NOT NULL,
	synopsis TEXT NOT NULL,
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	PRIMARY KEY (name, version, path),
	UNIQUE(name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);

CREATE TABLE readmes (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	markdown TEXT NOT NULL,
	PRIMARY KEY (name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);

CREATE TABLE licenses (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	contents TEXT NOT NULL,
	PRIMARY KEY (name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);

CREATE TABLE dependencies (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	dependency_name TEXT NOT NULL,
	dependency_version TEXT NOT NULL,
	PRIMARY KEY (name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version),
	FOREIGN KEY (dependency_name, dependency_version) REFERENCES versions(name, version)
);

CREATE TYPE version_source AS ENUM (
	'frontend',
	'legal',
	'proxy-index'
);

-- version_log is used for pub/sub redundancy in case messages are lost.
CREATE TABLE version_log (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	source version_source NOT NULL,
	error TEXT, -- log if error occurs from proxy
	PRIMARY KEY (name, version)
);

CREATE TABLE documents (
	series_name TEXT NOT NULL,
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	name_tokens TSVECTOR,
	package_name_tokens TSVECTOR,
	module_synopsis_tokens TSVECTOR,
	package_synopsis_tokens TSVECTOR,
	readme_tokens TSVECTOR,
	PRIMARY KEY (name, version),
	FOREIGN KEY (series_name) REFERENCES series(name),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);
