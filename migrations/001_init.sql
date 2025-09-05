CREATE SCHEMA IF NOT EXISTS jpcorrect;

-- ENUM type for error_status
DO $$ BEGIN
	CREATE TYPE jpcorrect.mistake_status AS ENUM ('ai_detected', 'ai_miscorrected', 'human_corrected');
EXCEPTION
	WHEN duplicate_object THEN NULL;
END $$;

-- ENUM type for error_type
DO $$ BEGIN
	CREATE TYPE jpcorrect.mistake_type AS ENUM ('E1', 'E2', 'E3', 'E4', 'E5', 'E6', 'E7', 'E8', 'E9');
EXCEPTION
	WHEN duplicate_object THEN NULL;
END $$;

-- Tables
CREATE TABLE IF NOT EXISTS jpcorrect."user" (
	user_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name text NOT NULL
);

CREATE TABLE IF NOT EXISTS jpcorrect.practice (
	practice_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id integer,
	CONSTRAINT practice_user_id_fkey FOREIGN KEY (user_id) REFERENCES jpcorrect."user"(user_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.mistake (
	mistake_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	practice_id integer NOT NULL,
	user_id integer NOT NULL,
	start_time double precision DEFAULT 0 NOT NULL,
	end_time double precision DEFAULT 0 NOT NULL,
	mistake_status jpcorrect.mistake_status NOT NULL,
	mistake_type jpcorrect.mistake_type NOT NULL,
	CONSTRAINT mistake_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES jpcorrect.practice(practice_id),
	CONSTRAINT mistake_user_id_fkey FOREIGN KEY (user_id) REFERENCES jpcorrect."user"(user_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.note (
	note_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	practice_id integer NOT NULL,
	content text,
	CONSTRAINT note_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES jpcorrect.practice(practice_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.transcript (
	transcript_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	mistake_id integer NOT NULL,
	content text,
	furigana text,
	accent text,
	CONSTRAINT transcript_mistake_id_fkey FOREIGN KEY (mistake_id) REFERENCES jpcorrect.mistake(mistake_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.ai_correction (
	ai_correction_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	mistake_id integer NOT NULL,
	content text,
	CONSTRAINT ai_correction_mistake_id_fkey FOREIGN KEY (mistake_id) REFERENCES jpcorrect.mistake(mistake_id)
);
