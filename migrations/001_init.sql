-- Schema: jpcorrect
CREATE SCHEMA IF NOT EXISTS jpcorrect;

-- Sequences
CREATE SEQUENCE IF NOT EXISTS jpcorrect.practice_practice_id_seq START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 NO CYCLE;
CREATE SEQUENCE IF NOT EXISTS jpcorrect.error_tag_error_tag_id_seq START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 NO CYCLE;
CREATE SEQUENCE IF NOT EXISTS jpcorrect.xml_detail_xml_detail_id_seq START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 NO CYCLE;
CREATE SEQUENCE IF NOT EXISTS jpcorrect.ai_correction_ai_correction_id_seq START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 NO CYCLE;
CREATE SEQUENCE IF NOT EXISTS jpcorrect.note_note_id_seq START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 NO CYCLE;

-- Tables
CREATE TABLE IF NOT EXISTS jpcorrect.practice (
	practice_id integer DEFAULT nextval('jpcorrect.practice_practice_id_seq') PRIMARY KEY,
	start_time double precision NOT NULL,
	end_time double precision NOT NULL
);

CREATE TABLE IF NOT EXISTS jpcorrect.error_tag (
	error_tag_id integer DEFAULT nextval('jpcorrect.error_tag_error_tag_id_seq') PRIMARY KEY,
	practice_id integer NOT NULL,
	error_person_id integer NOT NULL,
	error_type character varying(3) NOT NULL,
	ai_flag boolean DEFAULT false,
	ai_corrected boolean DEFAULT false,
	human_corrected boolean DEFAULT false,
	CONSTRAINT error_tag_error_type_check CHECK (error_type ~ '^E[1-9]$'),
	CONSTRAINT error_tag_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES jpcorrect.practice(practice_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.ai_correction (
	ai_correction_id integer DEFAULT nextval('jpcorrect.ai_correction_ai_correction_id_seq') PRIMARY KEY,
	error_tag_id integer NOT NULL,
	correction_content text,
	CONSTRAINT ai_correction_error_tag_id_fkey FOREIGN KEY (error_tag_id) REFERENCES jpcorrect.error_tag(error_tag_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.note (
	note_id integer DEFAULT nextval('jpcorrect.note_note_id_seq') PRIMARY KEY,
	practice_id integer NOT NULL,
	user_note text,
	CONSTRAINT note_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES jpcorrect.practice(practice_id)
);

CREATE TABLE IF NOT EXISTS jpcorrect.xml_detail (
	xml_detail_id integer DEFAULT nextval('jpcorrect.xml_detail_xml_detail_id_seq') PRIMARY KEY,
	error_tag_id integer NOT NULL,
	text_content text,
	furigana text,
	pitch text,
	CONSTRAINT xml_detail_error_tag_id_fkey FOREIGN KEY (error_tag_id) REFERENCES jpcorrect.error_tag(error_tag_id)
);

-- Indexes
CREATE UNIQUE INDEX IF NOT EXISTS practice_pkey ON jpcorrect.practice (practice_id);
CREATE UNIQUE INDEX IF NOT EXISTS error_tag_pkey ON jpcorrect.error_tag (error_tag_id);
CREATE UNIQUE INDEX IF NOT EXISTS ai_correction_pkey ON jpcorrect.ai_correction (ai_correction_id);
CREATE UNIQUE INDEX IF NOT EXISTS note_pkey ON jpcorrect.note (note_id);
CREATE UNIQUE INDEX IF NOT EXISTS xml_detail_pkey ON jpcorrect.xml_detail (xml_detail_id);
